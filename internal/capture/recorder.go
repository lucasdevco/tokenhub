// Package capture records complete traffic with bounded buffering and explicit
// backpressure. It has no dependency on the proxy or the log viewer.
package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lucasdevco/tokenhub/internal/database"
)

type Recorder struct {
	dir      string
	console  bool
	mu       sync.Mutex
	active   map[*Transaction]struct{}
	idle     chan struct{}
	stopping bool
	index    *database.Database
}

func NewRecorder(dir, dbPath string, console bool) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	index, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err = index.Q.RecoverCaptureWrites(context.Background(), absolute); err != nil {
		index.Close()
		return nil, err
	}
	return &Recorder{dir: absolute, console: console, active: make(map[*Transaction]struct{}), index: index}, nil
}

// Transaction captures one proxied request and its response.
// After Begin, call CaptureRequest once, then CaptureResponse or Fail, and
// finally End, including on error paths. These lifecycle calls must be serialized;
// network I/O on the wrapped bodies may run concurrently.
// Request and response have separate consumers and share metadata serialization.
type Transaction struct {
	recorder          *Recorder
	ctx               context.Context
	metadata          Metadata
	metaMu            sync.Mutex
	request, response *stream
	upgrade           bool
	socket            *socket
	workers           sync.WaitGroup
	once              sync.Once
	streamMu          sync.Mutex
	streams           []*stream
	abortErr          error
	outcomeErr        error
	disabled          bool
}

// Begin registers a transaction so Shutdown can wait for its capture work.
// The caller must call End even if forwarding fails before CaptureRequest.
func (r *Recorder) Begin(ctx context.Context, metadata Metadata) *Transaction {
	metadata.ID = uuid.NewString()
	tx := &Transaction{recorder: r, ctx: ctx, metadata: metadata}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		tx.disabled = true
		return tx
	}
	if len(r.active) == 0 {
		r.idle = make(chan struct{})
	}
	r.active[tx] = struct{}{}
	return tx
}

func (tx *Transaction) start(metadata Metadata, timestamp time.Time, line string, headers http.Header, direction, encoding string) *stream {
	input := newStream(tx.ctx, captureBudget)
	tx.streamMu.Lock()
	if tx.disabled {
		tx.streamMu.Unlock()
		input.dispose()
		return input
	}
	tx.streams = append(tx.streams, input)
	if tx.abortErr != nil {
		input.finish(tx.abortErr)
	}
	tx.streamMu.Unlock()
	headers = headers.Clone()
	tx.workers.Add(1)
	go func() {
		defer tx.workers.Done()
		defer input.dispose()
		tx.consume(input, metadata, timestamp, line, headers, direction, encoding)
	}()
	return input
}

func (tx *Transaction) setError(err error) {
	if err == nil || err == io.EOF {
		return
	}
	tx.streamMu.Lock()
	defer tx.streamMu.Unlock()
	if tx.outcomeErr == nil {
		tx.outcomeErr = err
	}
}

func (tx *Transaction) abort(err error) {
	tx.streamMu.Lock()
	defer tx.streamMu.Unlock()
	tx.abortErr = err
	for _, input := range tx.streams {
		input.finish(err)
	}
}

// CaptureRequest records the outgoing headers and replaces r.Body with a capture
// wrapper. Call it after rewriting the request and before sending it upstream.
// upgrade keeps request capture open for a possible bidirectional connection.
func (tx *Transaction) CaptureRequest(r *http.Request, upgrade bool) {
	tx.upgrade = upgrade
	tx.metadata.DestinationURL = r.URL.String()
	tx.metadata.RequestContentEncoding = r.Header.Get("Content-Encoding")
	overhead := float64(time.Since(tx.metadata.RequestStartedAt)) / float64(time.Millisecond)
	tx.metadata.RoutingOverheadMS = &overhead
	encoding := tx.metadata.RequestContentEncoding
	if upgrade || r.Body == nil || r.Body == http.NoBody {
		encoding = ""
	}
	tx.request = tx.start(tx.metadata, tx.metadata.RequestStartedAt, fmt.Sprintf("%s %s %s", r.Method, r.URL, r.Proto), r.Header, "request", encoding)
	if r.Body == nil || r.Body == http.NoBody {
		if !upgrade {
			tx.request.finish(nil)
		}
		return
	}
	r.Body = &body{source: r.Body, stream: tx.request, length: r.ContentLength, keepOpen: upgrade, observe: tx.setError}
}

// CaptureResponse records upstream headers and replaces r.Body with a capture
// wrapper. Call it after CaptureRequest and before reading the response body.
// A switching-protocols response must have an io.ReadWriteCloser body.
func (tx *Transaction) CaptureResponse(r *http.Response) error {
	now := time.Now()
	tx.metadata.UpstreamResponseAt = &now
	tx.metadata.UpstreamHeaderDurationMS = now.Sub(tx.metadata.RequestStartedAt).Milliseconds()
	tx.metadata.ResponseStatus, tx.metadata.ResponseStatusCode = r.Status, r.StatusCode
	tx.metadata.ResponseContentEncoding = r.Header.Get("Content-Encoding")
	encoding := tx.metadata.ResponseContentEncoding
	if r.StatusCode == http.StatusSwitchingProtocols || r.Body == http.NoBody || r.ContentLength == 0 || (r.Request != nil && r.Request.Method == "HEAD") {
		encoding = ""
	}
	tx.response = tx.start(tx.metadata, now, r.Proto+" "+r.Status, r.Header, "response", encoding)
	if r.StatusCode == http.StatusSwitchingProtocols {
		source, ok := r.Body.(io.ReadWriteCloser)
		if !ok {
			return errors.New("upgraded response is not bidirectional")
		}
		tx.socket = &socket{source: source, request: tx.request, response: tx.response, observe: tx.setError}
		r.Body = tx.socket
	} else {
		if tx.upgrade {
			tx.request.finish(nil)
		}
		length := r.ContentLength
		if r.Body == http.NoBody || (r.Request != nil && r.Request.Method == "HEAD") {
			length = 0
		}
		r.Body = &body{source: r.Body, stream: tx.response, length: length, observe: tx.setError}
	}
	return nil
}

// Fail records the exact proxy-generated body without claiming upstream headers
// arrived. It is separate from a capture-side storage/decompression failure.
func (tx *Transaction) Fail(message string, cause error) {
	tx.setError(cause)
	if tx.request != nil {
		tx.request.finish(cause)
	}
	if tx.response != nil {
		tx.response.finish(cause)
		return
	}
	tx.metadata.ResponseStatusCode = 502
	tx.metadata.ResponseStatus = "502 Bad Gateway"
	tx.response = tx.start(tx.metadata, time.Now(), "HTTP/1.1 502 Bad Gateway", http.Header{"Content-Type": {"text/plain; charset=utf-8"}}, "response", "")
	tx.response.write([]byte(message+"\n"), time.Now())
	tx.response.finish(cause)
}

// End marks forwarding as ended and starts background finalization. It is
// idempotent; do not call CaptureRequest, CaptureResponse, or Fail afterward.
// For upgraded connections it first
// closes the socket and waits for in-flight network I/O to finish capturing.
// Returning does not mean files have been written: Recorder.Shutdown waits for
// all capture workers and final transaction events. Storage failures are recorded
// separately from the forwarding outcome.
func (tx *Transaction) End(cause error) {
	tx.once.Do(func() {
		if tx.disabled {
			return
		}
		tx.setError(cause)
		completedAt := time.Now()
		if tx.socket != nil {
			// ReverseProxy can return while its tunnel-closing goroutine is still
			// running. Keep the request context alive until in-flight bytes have
			// been captured and their network errors have been observed.
			_ = tx.socket.Close()
		}
		if tx.request != nil {
			tx.request.finish(io.ErrUnexpectedEOF)
		}
		if tx.response != nil {
			tx.response.finish(io.ErrUnexpectedEOF)
		}
		metadata := tx.metadata
		go func() {
			defer func() {
				r := tx.recorder
				r.mu.Lock()
				defer r.mu.Unlock()
				delete(r.active, tx)
				if len(r.active) == 0 {
					close(r.idle)
					if r.stopping {
						_ = r.index.Close()
					}
				}
			}()
			tx.workers.Wait()
			tx.streamMu.Lock()
			err := tx.outcomeErr
			tx.streamMu.Unlock()
			event := Event{Event: "proxy_completed", StreamType: "proxy", Metadata: metadata, Timestamp: completedAt, StartedAt: metadata.RequestStartedAt, CompletedAt: &completedAt, Completed: err == nil}
			if err != nil {
				event.Error = err.Error()
			}
			if err := tx.appendEvent(event); err != nil {
				log.Printf("[capture] %s outcome: %v", metadata.ID, err)
			}
		}()
	})
}

// Shutdown stops accepting captures and waits for all begun transactions to End
// and finish writing. If ctx expires, it aborts their streams and returns ctx.Err();
// background cleanup may continue after that return.
func (r *Recorder) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.stopping = true
	idle := r.idle
	if len(r.active) == 0 {
		r.mu.Unlock()
		return r.index.Close()
	}
	r.mu.Unlock()
	select {
	case <-idle:
		return r.index.Close()
	case <-ctx.Done():
		r.mu.Lock()
		for tx := range r.active {
			tx.abort(ctx.Err())
		}
		r.mu.Unlock()
		return ctx.Err()
	}
}
