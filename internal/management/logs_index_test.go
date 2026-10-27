package management

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lucasdevco/tokenhub/internal/database"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
)

func TestOpenReusesIndexWithoutReadingNewHistory(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	fixture(t, dir, "indexed", now, true)
	s, err := NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	// First open does not ingest files synchronously.
	if _, ok := s.transaction("indexed"); ok {
		t.Fatal("opening the index scanned history")
	}
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	fixture(t, dir, "not-indexed", now.Add(time.Second), true)
	s, err = NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok := s.transaction("indexed"); !ok {
		t.Fatal("persisted history lost")
	}
	if _, ok := s.transaction("not-indexed"); ok {
		t.Fatal("reopen rescanned directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(350 * time.Millisecond)
	if _, ok := s.transaction("not-indexed"); ok {
		t.Fatal("completed import restarted automatically")
	}
	cancel()
	<-done
}

func TestStartupIgnoresUnindexedHistory(t *testing.T) {
	dir := t.TempDir()
	fixture(t, dir, "history", time.Now(), true)
	s, err := NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	time.Sleep(350 * time.Millisecond)
	if _, ok := s.transaction("history"); ok {
		t.Fatal("startup imported historical files")
	}

}

func TestDurableNotificationsSurviveClosedViewer(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	recorder, err := capture.NewRecorder(dir, database.Path(dir), false)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the capture mailbox protocol after a viewer has already closed.
	e := fixture(t, dir, "offline", time.Now(), true)
	id, err := recorderIndexNotify(dir, "offline_metadata.jsonl")
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	if err := recorder.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err = NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.processPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if item, ok := s.transaction(e.Metadata.ID); !ok || item.State != "completed" {
		t.Fatal("durable notification lost")
	}
}

func recorderIndexNotify(dir, name string) (int64, error) {
	s, err := NewStore(dir, database.Path(dir))
	if err != nil {
		return 0, err
	}
	defer s.Close()
	id, err := s.db.Q.BeginCaptureWrite(context.Background(), queries.BeginCaptureWriteParams{Source: s.dir, Name: name})
	if err != nil {
		return 0, err
	}
	return id, s.db.Q.EndCaptureWrite(context.Background(), id)
}

func TestCursorPagesStayStableWhenNewRequestsArrive(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < 5; i++ {
		fixture(t, dir, fmt.Sprintf("id-%d", i), now, true)
	}
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items []Transaction
		Next  string `json:"next_cursor"`
		Total int
	}
	w := request(t, NewLogHandler(s), "/api/requests?limit=2")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Next == "" {
		t.Fatal(w.Body.String())
	}
	seen := map[string]bool{}
	for _, item := range page.Items {
		seen[item.Key] = true
	}
	fixture(t, dir, "new", now.Add(time.Second), true)
	s.refresh([]string{"new_metadata.jsonl"})
	for page.Next != "" {
		w = request(t, NewLogHandler(s), "/api/requests?limit=2&cursor="+page.Next)
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.Key] || item.Key == "new" {
				t.Fatal("cursor duplicated/skipped ordering", item.Key)
			}
			seen[item.Key] = true
		}
	}
	if len(seen) != 5 {
		t.Fatal("cursor skipped history", seen)
	}
}

func TestAggregatesFollowUpsertsAndDeletion(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	fixture(t, dir, "one", now, false)
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(total, unfinished int) {
		t.Helper()
		w := request(t, NewLogHandler(s), "/api/requests")
		var page struct {
			All, Unfinished int
			Histogram       requestHistogram
		}
		json.Unmarshal(w.Body.Bytes(), &page)
		if page.All != total || page.Unfinished != unfinished || page.Histogram.Total != total {
			t.Fatal(w.Body.String())
		}
	}
	check(1, 1)
	fixture(t, dir, "one", now, true)
	s.refresh([]string{"one_metadata.jsonl"})
	check(1, 0)
	s.refresh([]string{"one_metadata.jsonl"})
	check(1, 0)
	os.Remove(filepath.Join(dir, "one_metadata.jsonl"))
	s.refresh([]string{"one_metadata.jsonl"})
	check(0, 0)
}

func TestIndexDatabaseCannotBeDownloaded(t *testing.T) {
	s, err := NewStore(t.TempDir(), database.Path(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"tokenhub.db", "tokenhub.db-wal", "tokenhub.db-shm"} {
		if w := request(t, NewLogHandler(s), "/api/file?name="+name); w.Code != 404 {
			t.Fatal("index exposed", name)
		}
	}
}

func BenchmarkOpenAndFirstPage(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			dir := b.TempDir()
			s, err := NewStore(dir, database.Path(dir))
			if err != nil {
				b.Fatal(err)
			}
			tx, err := s.db.DB.Begin()
			if err != nil {
				b.Fatal(err)
			}
			q := s.db.Q.WithTx(tx)
			data := []byte(`{"metadata":{},"state":"completed"}`)
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("%012d", i)
				if err := q.PutRecord(context.Background(), queries.PutRecordParams{Source: s.dir, MetadataFile: name, Key: name, Started: int64(i), State: "completed", Summary: data}); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			s.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s, err := NewStore(dir, database.Path(dir))
				if err != nil {
					b.Fatal(err)
				}
				rows, err := s.db.Q.ListRecords(context.Background(), queries.ListRecordsParams{Source: s.dir, BeforeStarted: math.MaxInt64, BeforeKey: "\uffff", PageSize: 100})
				if err != nil || len(rows) != 100 {
					b.Fatal(err, len(rows))
				}
				s.Close()
			}
		})
	}
}

func TestReadOnlyHistoryPreservesCaptureFiles(t *testing.T) {
	// New databases may create SQLite sidecars, but imports never rewrite captures.
	dir := t.TempDir()
	e := fixture(t, dir, "original", time.Now(), true)
	before, _ := os.ReadFile(filepath.Join(dir, e.Filename))
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = request(t, NewLogHandler(s), "/api/requests?q=original")
	after, _ := os.ReadFile(filepath.Join(dir, e.Filename))
	if strings.Compare(string(before), string(after)) != 0 {
		t.Fatal("capture modified")
	}
}
