package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readEvents(t *testing.T, dir string) []Event {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_metadata.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("metadata: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestRecorderLargeBodyStoredOnceAndMetrics(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRecorder(dir, database.Path(dir), false)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Begin(context.Background(), Metadata{RequestStartedAt: time.Now()})
	payload := `{"model":"large","input":"` + strings.Repeat("x", 5<<20) + `"}`
	request := httptest.NewRequest("POST", "http://upstream.test/v1/responses", strings.NewReader(payload))
	s.CaptureRequest(request, false)
	got, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil || string(got) != payload {
		t.Fatal("request changed", err)
	}
	response := &http.Response{Proto: "HTTP/1.1", Status: "200 OK", StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"model":"test","usage":{"output_tokens":4}}`)), ContentLength: -1}
	if err := s.CaptureResponse(response); err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	s.End(nil)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, dir)
	if len(events) != 5 {
		t.Fatalf("events=%d; no body copies expected", len(events))
	}
	for _, e := range events {
		if e.Event == "request_completed" {
			data, _ := os.ReadFile(filepath.Join(dir, e.Filename))
			_, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
			if !e.Completed || string(body) != payload {
				t.Fatal("large capture incomplete", e)
			}
		}
		if e.Event == "response_completed" && (e.Metadata.OutputTokens == nil || *e.Metadata.OutputTokens != 4) {
			t.Fatal("metrics missing", e)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*_metadata.jsonl"))
	info, _ := os.Stat(files[0])
	if info.Size() > 16<<10 {
		t.Fatalf("metadata grew with body: %d", info.Size())
	}
}

func TestStreamBackpressureDrainsAllBytesAndReleasesBudget(t *testing.T) {
	budget := make(chan struct{}, 2)
	s := newStream(context.Background(), budget)
	data := bytes.Repeat([]byte("x"), chunkSize*6)
	done := make(chan struct{})
	go func() { s.write(data, time.Now()); s.finish(nil); close(done) }()
	select {
	case <-done:
		t.Fatal("producer ignored budget")
	case <-time.After(20 * time.Millisecond):
	}
	got, err := io.ReadAll(s)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("backpressure lost bytes", err)
	}
	<-done
	s.dispose()
	if len(budget) != 0 || len(s.slots) != 0 || s.waitNS.Load() == 0 {
		t.Fatal("budget leaked or wait not measured")
	}
}

func TestStreamCancellationKeepsQueuedPrefixAndUnblocksProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budget := make(chan struct{}, 1)
	s := newStream(ctx, budget)
	s.write([]byte("accepted"), time.Now())
	done := make(chan struct{})
	go func() { s.write([]byte("blocked"), time.Now()); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked producer")
	}
	got, err := io.ReadAll(s)
	s.dispose()
	if string(got) != "accepted" || !errors.Is(err, context.Canceled) || len(budget) != 0 {
		t.Fatalf("%q %v budget=%d", got, err, len(budget))
	}
}

func TestStreamConsumerFailureReleasesBlockedWriters(t *testing.T) {
	budget := make(chan struct{}, 1)
	s := newStream(context.Background(), budget)
	s.write([]byte("first"), time.Now())
	done := make(chan struct{})
	go func() { s.write([]byte("second"), time.Now()); close(done) }()
	s.dispose()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failed consumer blocked forwarding")
	}
	if len(budget) != 0 || len(s.slots) != 0 {
		t.Fatal("failed capture leaked budget")
	}
}

func TestMetricsUseReceivedTimesAfterSlowConsumer(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	s := newStream(context.Background(), make(chan struct{}, 8))
	s.write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"), start)
	s.write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"), start.Add(100*time.Millisecond))
	s.write([]byte("data: {\"usage\":{\"completion_tokens\":10},\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"), start.Add(1100*time.Millisecond))
	s.finish(nil)
	metrics := newCaptureMetrics(Metadata{RequestStartedAt: start}, "response", func() time.Time { return s.at })
	io.Copy(metrics, s)
	s.dispose()
	m := metrics.finish(true)
	if m.TimeToFirstTokenMS == nil || *m.TimeToFirstTokenMS != 100 || m.TokensPerSecond == nil || *m.TokensPerSecond != 10 {
		t.Fatalf("used processing time: %+v", m)
	}
}

func TestTransactionEndIsAsyncAndShutdownWaitsForMetadata(t *testing.T) {
	r, err := NewRecorder(t.TempDir(), database.Path(t.TempDir()), false)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Begin(context.Background(), Metadata{RequestStartedAt: time.Now()})
	s.metaMu.Lock() // Hold finalization behind the transaction's disk-write lock.
	request := httptest.NewRequest("POST", "http://upstream.test/", strings.NewReader("body"))
	s.CaptureRequest(request, false)
	io.Copy(io.Discard, request.Body)
	request.Body.Close()
	s.End(nil)
	s.End(errors.New("duplicate end must not change the outcome"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shutdown skipped pending metadata", err)
	}
	s.metaMu.Unlock()
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, r.dir)
	if len(events) != 3 || !events[1].Completed {
		t.Fatal("accepted body not drained", events)
	}
	if events[2].Event != "proxy_completed" || !events[2].Completed {
		t.Fatal("End must publish the first outcome exactly once", events)
	}
}

func TestUnsupportedEncodingPreservesOriginalCapture(t *testing.T) {
	r, err := NewRecorder(t.TempDir(), database.Path(t.TempDir()), false)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Begin(context.Background(), Metadata{RequestStartedAt: time.Now()})
	payload := []byte{0, 255, 1, 2, 3}
	req := httptest.NewRequest("POST", "http://upstream.test/", bytes.NewReader(payload))
	req.Header.Set("Content-Encoding", "custom, gzip")
	s.CaptureRequest(req, false)
	io.Copy(io.Discard, req.Body)
	req.Body.Close()
	s.End(nil)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, e := range readEvents(t, r.dir) {
		if e.Event == "request_completed" {
			data, _ := os.ReadFile(filepath.Join(r.dir, e.Filename))
			headers, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
			if !e.Completed || !bytes.Equal(body, payload) || !bytes.Contains(headers, []byte("Content-Encoding: custom, gzip")) {
				t.Fatalf("opaque capture changed: %+v %q", e, data)
			}
			return
		}
	}
	t.Fatal("completion missing")
}
