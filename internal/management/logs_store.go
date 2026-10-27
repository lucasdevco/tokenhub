package management

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucasdevco/tokenhub/internal/database"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
)

const indexBatch = 128

type File struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Problem  string    `json:"problem,omitempty"`
}
type cachedMetadata struct {
	file    File
	events  []Event
	problem string
}

func (c cachedMetadata) complete() bool {
	for _, e := range c.events {
		if e.Event == "proxy_completed" {
			return true
		}
	}
	return false
}

type Store struct {
	dir        string
	db         *database.Database
	scanMu     sync.Mutex
	importMu   sync.Mutex
	generation atomic.Int64
	mu         sync.RWMutex
	revision   uint64
	problem    string
	indexing   bool
	wake       chan struct{}
}

// NewStore only opens the index. It never enumerates the capture directory.
func NewStore(directory, dbPath string) (*Store, error) {
	dir, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("log path is not a directory")
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, db: db, wake: make(chan struct{})}, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Run(ctx context.Context) {
	defer s.Close()
	updates := time.NewTicker(250 * time.Millisecond)
	defer updates.Stop()
	progress := time.NewTicker(time.Second)
	defer progress.Stop()
	activeAfter := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-updates.C:
			s.report(s.processPending(ctx))
		case <-progress.C:
			names, err := s.db.Q.ActiveFiles(ctx, queries.ActiveFilesParams{Source: s.dir, AfterName: activeAfter, PageSize: indexBatch})
			if err != nil {
				s.report(err)
				continue
			}
			for _, name := range names {
				s.report(s.refreshOne(ctx, name, true, 0))
				activeAfter = name
			}
			if len(names) < indexBatch {
				activeAfter = ""
			}
		}
	}
}

func (s *Store) report(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.problem != err.Error() {
		s.problem = err.Error()
		s.signalLocked()
	}
}
func (s *Store) signalLocked() { s.revision++; close(s.wake); s.wake = make(chan struct{}) }

func (s *Store) processPending(ctx context.Context) error {
	pending, err := s.db.Q.PendingFiles(ctx, queries.PendingFilesParams{Source: s.dir, PageSize: indexBatch})
	if err != nil {
		return err
	}
	for _, job := range pending {
		if err := s.refreshOne(ctx, job.Name, true, job.ID); err != nil {
			return err
		}
	}
	return nil
}

// Scan is an explicit rebuild/reconciliation entry point, not a polling task.
func (s *Store) Scan() error                       { return s.scan(context.Background(), true) }
func (s *Store) Reindex(ctx context.Context) error { return s.scan(ctx, true) }

func (s *Store) scan(ctx context.Context, reconcile bool) error {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	s.mu.Lock()
	s.indexing = true
	s.signalLocked()
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.indexing = false; s.signalLocked(); s.mu.Unlock() }()
	generation := time.Now().UnixNano()
	s.generation.Store(generation)
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		names, readErr := dir.Readdirnames(indexBatch) // Unsorted and bounded, unlike os.ReadDir.
		for _, name := range names {
			if allowedFile(name) {
				if err := s.refreshOne(ctx, name, false, 0); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		// Yield between batches so current traffic takes priority over first import.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
	if reconcile {
		after := ""
		for {
			stale, err := s.db.Q.StaleFiles(ctx, queries.StaleFilesParams{Source: s.dir, Generation: generation, AfterName: after, PageSize: indexBatch})
			if err != nil {
				return err
			}
			for _, file := range stale {
				if err := s.refreshOne(ctx, file.Name, true, 0); err != nil {
					return err
				}
				after = file.Name
			}
			if len(stale) < indexBatch {
				break
			}
		}
	}
	return nil
}

func (s *Store) refresh(names []string) {
	for _, name := range names {
		s.report(s.refreshOne(context.Background(), name, true, 0))
	}
}

func (s *Store) fileInfo(name string) (File, error) {
	f, err := s.open(name)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return File{}, err
	}
	return File{Name: name, Size: info.Size(), Modified: info.ModTime()}, nil
}

func flag(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
func (s *Store) refreshOne(ctx context.Context, name string, force bool, ack int64) error {
	if !allowedFile(name) {
		return nil
	}
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.db.Q.WithTx(tx)
	generation := s.generation.Load()
	if !strings.HasSuffix(name, "_metadata.jsonl") {
		old, getErr := q.GetFile(ctx, queries.GetFileParams{Name: name, Source: s.dir})
		if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
			return getErr
		}
		file, statErr := s.fileInfo(name)
		if statErr != nil {
			if !os.IsNotExist(statErr) {
				return statErr
			}
			if err = q.DeleteFile(ctx, queries.DeleteFileParams{Name: name, Source: s.dir}); err != nil {
				return err
			}
		} else {
			if getErr == nil && old.Size == file.Size && old.Modified == file.Modified.UnixNano() {
				if err = q.TouchFile(ctx, queries.TouchFileParams{Source: s.dir, Seen: generation, Name: name}); err != nil {
					return err
				}
				return commitIndex(ctx, tx, q, ack)
			}
			if err = q.PutFile(ctx, queries.PutFileParams{Source: s.dir, Name: name, Owner: old.Owner, Size: file.Size, Modified: file.Modified.UnixNano(), Problem: file.Problem, Complete: old.Complete, Seen: generation}); err != nil {
				return err
			}
		}
		if old.Owner != "" {
			name = old.Owner
			force = true
		} else {
			if err = commitIndex(ctx, tx, q, ack); err == nil {
				s.changed()
			}
			return err
		}
	}
	file, err := s.fileInfo(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err = q.DeleteRecord(ctx, queries.DeleteRecordParams{MetadataFile: name, Source: s.dir}); err != nil {
			return err
		}
		if err = q.DeleteFile(ctx, queries.DeleteFileParams{Name: name, Source: s.dir}); err != nil {
			return err
		}
		if err = q.OrphanFiles(ctx, queries.OrphanFilesParams{Owner: name, Source: s.dir}); err != nil {
			return err
		}
		if err = commitIndex(ctx, tx, q, ack); err == nil {
			s.changed()
		}
		return err
	}
	old, oldErr := q.GetMetadata(ctx, queries.GetMetadataParams{MetadataFile: name, Source: s.dir})
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return oldErr
	}
	if !force && oldErr == nil && old.Size == file.Size && old.Modified == file.Modified.UnixNano() {
		if err = q.TouchRecord(ctx, queries.TouchRecordParams{Source: s.dir, Seen: generation, MetadataFile: name}); err != nil {
			return err
		}
		if err = q.TouchFile(ctx, queries.TouchFileParams{Source: s.dir, Seen: generation, Name: name}); err != nil {
			return err
		}
		return commitIndex(ctx, tx, q, ack)
	}
	cached := s.readMetadata(file)
	key := strings.TrimSuffix(name, "_metadata.jsonl")
	if len(cached.events) > 0 && cached.events[0].Metadata.ID != "" {
		key = cached.events[0].Metadata.ID
	}
	t := Transaction{Key: key, Started: file.Modified, State: "unfinished", MetadataFiles: []string{name}, Events: cached.events}
	if cached.problem != "" {
		t.Problems = append(t.Problems, cached.problem)
	}
	files := map[string]File{name: file}
	complete := make(map[string]bool)
	for _, event := range cached.events {
		if !allowedFile(event.Filename) {
			continue
		}
		complete[event.Filename] = complete[event.Filename] || strings.HasSuffix(event.Event, "_completed")
		if _, ok := files[event.Filename]; !ok {
			f, statErr := s.fileInfo(event.Filename)
			if statErr == nil {
				files[f.Name] = f
			} else if !os.IsNotExist(statErr) {
				return statErr
			}
		}
	}
	t.summarize(files)
	t.Events = nil
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	// Historical unfinished files may be abandoned; only durable live writes
	// enroll a transaction in periodic progress checks.
	changed := oldErr != nil || !bytes.Equal(data, old.Summary) || old.Size != file.Size || old.Modified != file.Modified.UnixNano()
	search := strings.ToLower(strings.Join([]string{t.Metadata.Method, t.Metadata.SourceURL, t.Metadata.DestinationURL, t.Metadata.ID, t.Metadata.Provider, t.Metadata.Model, t.Metadata.UserAgent, strconv.Itoa(t.Metadata.ResponseStatusCode), name}, " "))
	if err = q.PutRecord(ctx, queries.PutRecordParams{Source: s.dir, MetadataFile: name, Key: key, Started: t.Started.UnixNano(), State: t.State, Method: t.Metadata.Method, CaptureError: flag(t.CaptureState == "failed" || t.CaptureState == "incomplete"), Failed: flag(t.State == "error" || t.Metadata.ResponseStatusCode >= 400 || t.CaptureState == "failed" || t.CaptureState == "incomplete"), SearchText: search, Summary: data, Size: file.Size, Modified: file.Modified.UnixNano(), Watch: flag(!cached.complete() && (ack != 0 || old.Watch != 0)), Seen: generation}); err != nil {
		return err
	}
	for filename, f := range files {
		if filename == name {
			f.Problem = cached.problem
		}
		if err = q.PutFile(ctx, queries.PutFileParams{Source: s.dir, Name: filename, Owner: name, Size: f.Size, Modified: f.Modified.UnixNano(), Problem: f.Problem, Complete: flag(complete[filename]), Seen: generation}); err != nil {
			return err
		}
	}
	if err = commitIndex(ctx, tx, q, ack); err == nil && changed {
		s.changed()
	}
	return err
}

func commitIndex(ctx context.Context, tx *sql.Tx, q *queries.Queries, ack int64) error {
	if ack != 0 {
		if err := q.AckCaptureWrite(ctx, ack); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) changed() { s.mu.Lock(); s.signalLocked(); s.mu.Unlock() }

func (s *Store) transaction(key string) (Transaction, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	record, err := s.db.Q.GetRecord(ctx, queries.GetRecordParams{Key: key, Source: s.dir})
	if err != nil {
		return Transaction{}, false
	}
	var t Transaction
	if json.Unmarshal(record.Summary, &t) != nil {
		return Transaction{}, false
	}
	file, err := s.fileInfo(record.MetadataFile)
	if err == nil {
		t.Events = s.readMetadata(file).events
	}
	return t, true
}
func (s *Store) finished(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	file, err := s.db.Q.GetFile(ctx, queries.GetFileParams{Name: name, Source: s.dir})
	return err == nil && file.Complete != 0
}
func allowedFile(name string) bool {
	return name != "" && filepath.Base(name) == name && !strings.ContainsAny(name, `/\:`) &&
		(strings.HasSuffix(name, ".bin") || strings.HasSuffix(name, "_metadata.jsonl"))
}

// Open only regular files immediately inside the selected log directory. In
// particular, never follow a symlink planted in a log directory.
func (s *Store) open(name string) (*os.File, error) {
	if !allowedFile(name) {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(s.dir, name)
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		f.Close()
		return nil, os.ErrPermission
	}
	return f, nil
}

func (s *Store) readMetadata(file File) cachedMetadata {
	result := cachedMetadata{file: file}
	f, err := s.open(file.Name)
	if err != nil {
		result.problem = err.Error()
		return result
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil {
		result.file.Size = info.Size()
		result.file.Modified = info.ModTime()
	}
	const maxMetadata = 4 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxMetadata+1))
	if err != nil {
		result.problem = err.Error()
		return result
	}
	if len(data) > maxMetadata {
		result.problem = "Metadata exceeds 4 MiB; download the original to inspect it"
		return result
	}
	lines := bytes.Split(data, []byte{'\n'})
	for _, line := range lines[:len(lines)-1] {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			result.problem = "Invalid metadata line; original file retained"
			continue
		}
		result.events = append(result.events, event)
	}
	if len(lines[len(lines)-1]) > 0 {
		result.problem = "Waiting for a complete metadata line (or interrupted write)"
	}
	return result
}
