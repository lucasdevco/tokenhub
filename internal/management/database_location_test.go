package management

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	"github.com/lucasdevco/tokenhub/internal/database"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
)

func TestApplicationDatabaseIgnoresLegacyData(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(logs, ".tokenhub-index.sqlite")
	legacy := []byte("legacy database must not be opened or migrated")
	put(t, logs, filepath.Base(legacyPath), legacy)
	fixture(t, logs, "legacy", time.Now(), true)
	dbPath := database.Path(root)
	store, err := NewStore(logs, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { store.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	recorder, err := capture.NewRecorder(logs, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Shutdown(context.Background())
	tx := recorder.Begin(ctx, capture.Metadata{RequestStartedAt: time.Now(), Method: "GET"})
	tx.CaptureRequest(httptest.NewRequest("GET", "http://upstream.test/", nil), false)
	response := &http.Response{Proto: "HTTP/1.1", Status: "200 OK", StatusCode: 200,
		Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), ContentLength: 2}
	if err := tx.CaptureResponse(response); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	tx.End(nil)
	if err := recorder.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		w := request(t, NewLogHandler(store), "/api/requests")
		var page struct {
			All   int
			Items []Transaction
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.All > 1 {
			t.Fatalf("historical files were imported: %s", w.Body.String())
		}
		if len(page.Items) == 1 && page.Items[0].State == "completed" {
			if page.Items[0].Key == "legacy" {
				t.Fatal("legacy record was imported")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("new capture was not indexed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := store.transaction("legacy"); ok {
		t.Fatal("legacy history was imported")
	}
	if info, err := os.Stat(dbPath); err != nil || !info.Mode().IsRegular() {
		t.Fatal("application database missing", err)
	}
	if _, err := os.Stat(database.Path(logs)); !os.IsNotExist(err) {
		t.Fatal("database created inside log directory", err)
	}
	if data, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(data, legacy) {
		t.Fatal("legacy database was changed", err)
	}
}

func TestSharedApplicationDatabaseIsolatesLogDirectories(t *testing.T) {
	root := t.TempDir()
	dbPath := database.Path(root)
	firstDir, secondDir := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, dir := range []string{firstDir, secondDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		fixture(t, dir, "same", time.Now(), true)
	}
	fixture(t, secondDir, "extra", time.Now(), true)
	put(t, firstDir, "same_request.bin", []byte("first"))
	put(t, secondDir, "same_request.bin", []byte("second payload"))
	first, err := NewStore(firstDir, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewStore(secondDir, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	// Explicit scans seed independent fixtures; normal startup never scans.
	if err := first.Scan(); err != nil {
		t.Fatal(err)
	}
	if err := second.Scan(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		store        *Store
		count        int
		requestBytes int64
	}{
		{first, 1, int64(len("first"))}, {second, 2, int64(len("second payload"))},
	} {
		w := request(t, NewLogHandler(tc.store), "/api/requests")
		var page struct {
			All       int
			Total     int
			Items     []Transaction
			Histogram requestHistogram
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.All != tc.count || page.Total != tc.count || len(page.Items) != tc.count || page.Histogram.Total != tc.count {
			t.Fatalf("directory counts mixed: %s", w.Body.String())
		}
		item, ok := tc.store.transaction("same")
		if !ok || item.RequestBytes != tc.requestBytes {
			t.Fatalf("directory details mixed: %+v", item)
		}
		w = request(t, NewLogHandler(tc.store), "/api/files")
		var files struct {
			Total int
			Items []File
		}
		if err := json.Unmarshal(w.Body.Bytes(), &files); err != nil {
			t.Fatal(err)
		}
		if files.Total != 3*tc.count || len(files.Items) != 3*tc.count {
			t.Fatalf("directory files mixed: %s", w.Body.String())
		}
	}
	ctx := context.Background()
	id, err := first.db.Q.BeginCaptureWrite(ctx, queries.BeginCaptureWriteParams{Source: first.dir, Name: "same_metadata.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.db.Q.EndCaptureWrite(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := second.processPending(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err := first.db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: first.dir, PageSize: indexBatch})
	if err != nil || len(jobs) != 1 {
		t.Fatal("another directory consumed the notification", jobs, err)
	}
	if err := first.processPending(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err = first.db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: first.dir, PageSize: indexBatch})
	if err != nil || len(jobs) != 0 {
		t.Fatal("own notification was not processed", jobs, err)
	}
}
