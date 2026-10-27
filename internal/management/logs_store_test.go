package management

import (
	"context"
	"encoding/json"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
)

func TestRefreshUpdatesOnlyAffectedTransactionAndReconcilesDeletion(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	fixture(t, dir, "old", now.Add(-time.Hour), true)
	fixture(t, dir, "active", now, false)
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := s.transaction("old")
	put(t, dir, "old_metadata.jsonl", []byte("invalid\n")) // External change without notification.
	put(t, dir, "active_request.bin", []byte("growing"))
	s.refresh([]string{"active_request.bin"})
	unchanged, _ := s.transaction("old")
	active, _ := s.transaction("active")
	if old.Metadata.ID != unchanged.Metadata.ID || unchanged.State != "completed" || active.RequestBytes != 7 {
		t.Fatal("refresh rebuilt unrelated history")
	}
	if !s.finished("old_response.bin") {
		t.Fatal("completed files remained active")
	}
	if err := os.Remove(filepath.Join(dir, "old_metadata.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.transaction("old"); ok || s.finished("old_response.bin") {
		t.Fatal("removed transaction retained in lookup")
	}
	if _, ok := s.transaction("active"); !ok {
		t.Fatal("position index broken by deletion")
	}
}

func TestRecorderNotificationsDiscoverAndCompleteNewTransaction(t *testing.T) {
	dir := t.TempDir()
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	r, err := capture.NewRecorder(dir, database.Path(dir), false)
	if err != nil {
		t.Fatal(err)
	}
	tx := r.Begin(ctx, capture.Metadata{RequestStartedAt: time.Now(), Method: "POST"})
	req := httptest.NewRequest("POST", "http://upstream.test/", strings.NewReader(`{"model":"selected"}`))
	tx.CaptureRequest(req, false)
	io.Copy(io.Discard, req.Body)
	req.Body.Close()
	response := &http.Response{StatusCode: 200, Status: "200 OK", Proto: "HTTP/1.1", ContentLength: 2, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}
	tx.CaptureResponse(response)
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	tx.End(nil)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		w := request(t, NewLogHandler(s), "/api/requests")
		var page struct{ Items []Transaction }
		_ = json.Unmarshal(w.Body.Bytes(), &page)
		ready := len(page.Items) == 1 && page.Items[0].State == "completed" && page.Items[0].CaptureState == "completed"
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("notification did not update index before reconciliation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCaptureFailureDoesNotOverwriteSuccessfulProxyOutcome(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	e := fixture(t, dir, "failure", now, false)
	e.Event = "request_completed"
	e.CaptureError = "disk full"
	data := eventLine(t, e)
	e.Event = "response_completed"
	e.StreamType = "response"
	e.Filename = "failure_response.bin"
	e.Metadata.ResponseStatusCode = 200
	data = append(data, eventLine(t, e)...)
	e.Event = "proxy_completed"
	e.StreamType = "proxy"
	e.Filename = ""
	e.Completed = true
	e.CaptureError = ""
	data = append(data, eventLine(t, e)...)
	put(t, dir, "failure_metadata.jsonl", data)
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := s.transaction("failure")
	if tx.State != "completed" || tx.CaptureState != "failed" || tx.Metadata.ResponseStatusCode != 200 {
		t.Fatalf("outcomes conflated: %+v", tx)
	}
}
