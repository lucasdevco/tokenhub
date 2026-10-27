package capture

import (
	"bytes"
	"context"
	"errors"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type socketSource struct {
	read  func([]byte) (int, error)
	write func([]byte) (int, error)
	close func() error
}

func (s socketSource) Read(p []byte) (int, error) {
	if s.read != nil {
		return s.read(p)
	}
	return 0, io.EOF
}
func (s socketSource) Write(p []byte) (int, error) {
	if s.write != nil {
		return s.write(p)
	}
	return len(p), nil
}
func (s socketSource) Close() error {
	if s.close != nil {
		return s.close()
	}
	return nil
}

// Pause both disk consumers before their first event, so queue saturation and
// the close/write handoff can be exercised without depending on disk speed.
func pausedSocketCapture(t *testing.T, ctx context.Context, source io.ReadWriteCloser) (*Recorder, *Transaction, func()) {
	t.Helper()
	r, err := NewRecorder(t.TempDir(), database.Path(t.TempDir()), false)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Begin(ctx, Metadata{RequestStartedAt: time.Now()})
	s.metaMu.Lock()
	release := sync.OnceFunc(s.metaMu.Unlock)
	t.Cleanup(release)
	s.CaptureRequest(httptest.NewRequest("GET", "http://upstream.test/", nil), true)
	response := &http.Response{Proto: "HTTP/1.1", Status: "101 Switching Protocols", StatusCode: 101, Header: http.Header{}, Body: source, ContentLength: -1}
	if err := s.CaptureResponse(response); err != nil {
		t.Fatal(err)
	}
	return r, s, release
}

func awaitSocket(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("socket operation did not finish")
	}
}

func TestSocketCloseWaitsForInflightCapture(t *testing.T) {
	for _, direction := range []string{"request", "response"} {
		t.Run(direction, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			payload := bytes.Repeat([]byte("x"), (streamChunks+1)*chunkSize)
			reader := bytes.NewReader(payload)
			accepted := make(chan struct{}, streamChunks+1)
			closed := make(chan struct{})
			source := socketSource{
				read:  func(p []byte) (int, error) { n, err := reader.Read(p); accepted <- struct{}{}; return n, err },
				write: func(p []byte) (int, error) { accepted <- struct{}{}; return len(p), nil },
				close: func() error { close(closed); return nil },
			}
			r, s, release := pausedSocketCapture(t, ctx, source)
			operation := s.socket.Write
			if direction == "response" {
				operation = s.socket.Read
			}
			block := make([]byte, chunkSize)
			copy(block, payload)
			for i := 0; i < streamChunks; i++ {
				if n, err := operation(block); n != len(block) || err != nil {
					t.Fatalf("operation: %d %v", n, err)
				}
				<-accepted
			}
			written := make(chan struct{})
			go func() { defer close(written); operation(block) }()
			<-accepted // The upstream accepted one more block, but the queue is full.
			closeDone := make(chan struct{})
			go func() { s.socket.Close(); close(closeDone) }()
			awaitSocket(t, closed)
			finished := make(chan struct{})
			go func() { s.End(nil); close(finished) }()
			select {
			case <-closeDone:
				t.Fatal("close discarded in-flight capture")
			case <-finished:
				t.Fatal("outcome finalized before in-flight capture")
			case <-time.After(20 * time.Millisecond):
			}
			release()
			awaitSocket(t, written)
			awaitSocket(t, closeDone)
			awaitSocket(t, finished)
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, event := range readEvents(t, r.dir) {
				if event.Event == direction+"_completed" {
					found = true
					data, err := os.ReadFile(filepath.Join(r.dir, event.Filename))
					if err != nil {
						t.Fatal(err)
					}
					_, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
					if !event.Completed || !bytes.Equal(body, payload) {
						t.Fatalf("captured %d/%d: %+v", len(body), len(payload), event)
					}
				}
				if event.Event == "proxy_completed" && !event.Completed {
					t.Fatalf("normal close became a network error: %+v", event)
				}
			}
			if !found {
				t.Fatal("capture completion missing")
			}
		})
	}
}

func TestSocketNetworkErrorsReachProxyOutcome(t *testing.T) {
	for _, direction := range []string{"request", "response"} {
		t.Run(direction, func(t *testing.T) {
			reset := errors.New("upstream connection reset")
			source := socketSource{
				read:  func(p []byte) (int, error) { return copy(p, "part"), reset },
				write: func(p []byte) (int, error) { return len(p), reset },
			}
			r, s, release := pausedSocketCapture(t, context.Background(), source)
			release()
			operation := s.socket.Write
			if direction == "response" {
				operation = s.socket.Read
			}
			if _, err := operation([]byte("part")); !errors.Is(err, reset) {
				t.Fatal("network error changed", err)
			}
			s.End(nil) // The upgraded ReverseProxy returns without ErrorHandler.
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, event := range readEvents(t, r.dir) {
				if event.Event == "proxy_completed" {
					found = true
					if event.Completed || event.Error != reset.Error() {
						t.Fatalf("network failure reported as success: %+v", event)
					}
				}
				if event.Event == direction+"_completed" {
					data, _ := os.ReadFile(filepath.Join(r.dir, event.Filename))
					if event.Completed || event.Error != reset.Error() || !bytes.HasSuffix(data, []byte("part")) {
						t.Fatalf("partial failure lost: %+v %q", event, data)
					}
				}
			}
			if !found {
				t.Fatal("proxy outcome missing")
			}
		})
	}
}

func TestSocketAbortReleasesCloseUnderBackpressure(t *testing.T) {
	for _, abort := range []string{"cancel", "shutdown"} {
		t.Run(abort, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			accepted := make(chan struct{}, streamChunks+1)
			closed := make(chan struct{})
			source := socketSource{write: func(p []byte) (int, error) { accepted <- struct{}{}; return len(p), nil }, close: func() error { close(closed); return nil }}
			r, s, release := pausedSocketCapture(t, ctx, source)
			block := bytes.Repeat([]byte("x"), chunkSize)
			for i := 0; i < streamChunks; i++ {
				s.socket.Write(block)
				<-accepted
			}
			written := make(chan struct{})
			go func() { s.socket.Write(block); close(written) }()
			<-accepted
			closeDone := make(chan struct{})
			go func() { s.socket.Close(); close(closeDone) }()
			awaitSocket(t, closed)
			wantCause := context.Canceled
			if abort == "cancel" {
				cancel()
			} else {
				deadline, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer stop()
				if err := r.Shutdown(deadline); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("shutdown did not respect deadline", err)
				}
				wantCause = context.DeadlineExceeded
			}
			awaitSocket(t, written)
			awaitSocket(t, closeDone)
			s.End(ctx.Err())
			release()
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, event := range readEvents(t, r.dir) {
				if event.Event == "request_completed" {
					data, _ := os.ReadFile(filepath.Join(r.dir, event.Filename))
					_, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
					if event.Completed || !strings.Contains(event.Error, wantCause.Error()) || len(body) != streamChunks*chunkSize {
						t.Fatalf("cancellation lost prefix or claimed success: %+v bytes=%d", event, len(body))
					}
					return
				}
			}
			t.Fatal("request completion missing")

		})
	}
}

func TestSocketCloseUnblocksNetworkWithoutInventingFailure(t *testing.T) {
	started := make(chan struct{}, 2)
	closed := make(chan struct{})
	blocked := func([]byte) (int, error) { started <- struct{}{}; <-closed; return 0, net.ErrClosed }
	r, s, release := pausedSocketCapture(t, context.Background(), socketSource{read: blocked, write: blocked, close: func() error { close(closed); return nil }})
	release()
	readDone, writeDone := make(chan struct{}), make(chan struct{})
	go func() { s.socket.Read(make([]byte, 1)); close(readDone) }()
	go func() { s.socket.Write([]byte("x")); close(writeDone) }()
	<-started
	<-started
	finishDone := make(chan struct{})
	go func() { s.End(nil); close(finishDone) }()
	awaitSocket(t, readDone)
	awaitSocket(t, writeDone)
	awaitSocket(t, finishDone)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, event := range readEvents(t, r.dir) {
		if strings.HasSuffix(event.Event, "_completed") && !event.Completed {
			t.Fatalf("local close reported as failure: %+v", event)
		}
	}
}
