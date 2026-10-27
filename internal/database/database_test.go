package database

import (
	"context"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
	"testing"
)

func TestConnectionPragmas(t *testing.T) {
	path := Path(t.TempDir())
	for range 2 {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for pragma, want := range map[string]string{
			"journal_mode": "wal",
			"busy_timeout": "5000",
			"synchronous":  "1",
			"foreign_keys": "1",
			"cache_size":   "-8192",
		} {
			var got string
			if err := db.DB.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
				t.Error(err)
			} else if got != want {
				t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAcknowledgingOldWriteKeepsNewNotification(t *testing.T) {
	db, err := Open(Path(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	old, err := db.Q.BeginCaptureWrite(ctx, queries.BeginCaptureWriteParams{Source: "test", Name: "record_metadata.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Q.EndCaptureWrite(ctx, old); err != nil {
		t.Fatal(err)
	}
	newer, err := db.Q.BeginCaptureWrite(ctx, queries.BeginCaptureWriteParams{Source: "test", Name: "record_metadata.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Q.EndCaptureWrite(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err = db.Q.AckCaptureWrite(ctx, old); err != nil {
		t.Fatal(err)
	}
	jobs, err := db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: "test", PageSize: 128})
	if err != nil || len(jobs) != 1 || jobs[0].ID != newer {
		t.Fatal("new notification lost", jobs, err)
	}
}

func TestIncompleteWriteIsNotVisibleUntilRecovery(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = db.Q.BeginCaptureWrite(ctx, queries.BeginCaptureWriteParams{Source: "test", Name: "record_metadata.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: "test", PageSize: 128})
	if err != nil || len(jobs) != 0 {
		t.Fatal("read in-flight capture", jobs, err)
	}
	db.Close()
	db, err = Open(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Q.RecoverCaptureWrites(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	jobs, err = db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: "test", PageSize: 128})
	if err != nil || len(jobs) != 1 {
		t.Fatal("interrupted write lost", jobs, err)
	}
}
