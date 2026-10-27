package management

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
)

func indexedStore(t *testing.T, dir string) (*Store, error) {
	t.Helper()
	s, err := NewStore(dir, database.Path(dir))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Scan(); err != nil {
		return nil, err
	}
	return s, nil
}

func put(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func eventLine(t *testing.T, e Event) []byte {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}
func fixture(t *testing.T, dir, id string, when time.Time, complete bool) Event {
	t.Helper()
	e := Event{Event: "request_started", StreamType: "request", Metadata: capture.Metadata{ID: id, Method: "GET", SourceURL: "https://example.test/" + id, RequestStartedAt: when}, Timestamp: when, StartedAt: when, Filename: id + "_request.bin"}
	data := eventLine(t, e)
	put(t, dir, e.Filename, []byte("GET / HTTP/1.1\r\n\r\n"))
	if complete {
		done := e
		done.Event = "request_completed"
		done.Completed = true
		data = append(data, eventLine(t, done)...)
		done.Event = "response_completed"
		done.StreamType = "response"
		done.Filename = id + "_response.bin"
		done.Metadata.ResponseStatusCode = 200
		data = append(data, eventLine(t, done)...)
		put(t, dir, done.Filename, []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: hi\n\n"))
		done.Event = "proxy_completed"
		done.StreamType = "proxy"
		done.Filename = ""
		data = append(data, eventLine(t, done)...)
	}
	put(t, dir, id+"_metadata.jsonl", data)
	return e
}
func request(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1"+path, nil))
	return w
}

func TestHistoryReadOnlyPaginationAndFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	fixture(t, dir, "older", now.Add(-time.Hour), true)
	fixture(t, dir, "newer", now, false)
	put(t, dir, "orphan_response.bin", []byte{0, 255, 1})
	put(t, dir, "broken_metadata.jsonl", []byte("invalid\n"))
	before := make(map[string][]byte)
	dates := make(map[string]time.Time)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		before[e.Name()], _ = os.ReadFile(filepath.Join(dir, e.Name()))
		info, _ := e.Info()
		dates[e.Name()] = info.ModTime()
	}
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	h := NewLogHandler(s)
	w := request(t, h, "/api/requests?limit=1")
	var result struct {
		Items []Transaction
		Total int
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || len(result.Items) != 1 {
		t.Fatalf("unexpected page: %s", w.Body.String())
	}
	w = request(t, h, "/api/requests?q=older")
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 1 || result.Items[0].State != "completed" || result.Items[0].Metadata.ResponseStatusCode != 200 {
		t.Fatalf("bad history: %s", w.Body.String())
	}
	w = request(t, h, "/api/requests?state=unfinished")
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 2 {
		t.Fatalf("unfinished: %s", w.Body.String())
	}
	w = request(t, h, "/api/file?name=orphan_response.bin&download=1")
	if !bytes.Equal(w.Body.Bytes(), []byte{0, 255, 1}) {
		t.Fatal("binary download changed")
	}
	for i := 0; i < 3; i++ {
		if err = s.Scan(); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := os.ReadDir(dir)

	for _, e := range after {
		if !allowedFile(e.Name()) {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		info, _ := e.Info()
		if !bytes.Equal(data, before[e.Name()]) || !info.ModTime().Equal(dates[e.Name()]) {
			t.Fatalf("viewer modified %s", e.Name())
		}
	}
}

func TestPartialMetadataAndGrowth(t *testing.T) {
	dir := t.TempDir()
	e := fixture(t, dir, "stream", time.Now().UTC(), false)
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "stream_metadata.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	e.Event = "response_started"
	e.StreamType = "response"
	e.Filename = "stream_response.bin"
	e.Metadata.ResponseStatusCode = 200
	line := eventLine(t, e)
	_, _ = f.Write(line[:len(line)/2])
	if err = s.Scan(); err != nil {
		t.Fatal(err)
	}
	item, _ := s.transaction("stream")
	if len(item.Events) != 1 {
		t.Fatal("partial line was ingested")
	}
	_, _ = f.Write(line[len(line)/2:])
	put(t, dir, e.Filename, []byte("first"))
	if err = s.Scan(); err != nil {
		t.Fatal(err)
	}
	item, _ = s.transaction("stream")
	if len(item.Events) != 2 || item.ResponseBytes != 5 {
		t.Fatal("completed line/body not visible")
	}
	revision := s.revision
	put(t, dir, e.Filename, []byte("first + more"))
	_ = s.Scan()
	item, _ = s.transaction("stream")
	if s.revision <= revision || item.ResponseBytes != 12 {
		t.Fatal("body growth not visible")
	}
}

func TestAccessBoundariesAndRawRanges(t *testing.T) {
	dir := t.TempDir()
	content := bytes.Repeat([]byte{0, 255, 10, 13, 42}, 20000)
	put(t, dir, "raw_response.bin", content)
	s, _ := indexedStore(t, dir)
	h := NewLogHandler(s)
	for _, path := range []string{"/api/file?name=../secret.bin", "/api/file?name=C:%5Csecret.bin", "/api/file?name=raw_response.bin:secret", "/api/file?name=.env"} {
		if w := request(t, h, path); w.Code != 404 {
			t.Fatalf("path accessible: %s = %d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/requests?limit=0", "/api/requests?offset=-1", "/api/file?name=raw_response.bin&offset=-1", "/api/tail?name=raw_response.bin&offset=bad"} {
		if w := request(t, h, path); w.Code != 400 {
			t.Fatalf("bad input accepted: %s = %d", path, w.Code)
		}
	}
	w := request(t, h, "/api/file?name=raw_response.bin&offset=65536")
	var chunk struct {
		Data       string
		Next, Size int64
	}
	_ = json.Unmarshal(w.Body.Bytes(), &chunk)
	data, _ := base64.StdEncoding.DecodeString(chunk.Data)
	if !bytes.Equal(data, content[65536:]) || chunk.Next != int64(len(content)) {
		t.Fatal("range read lost bytes")
	}
	for _, r := range []*http.Request{httptest.NewRequest("GET", "http://evil.test/api/files", nil), httptest.NewRequest("POST", "http://127.0.0.1/api/files", nil)} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 && w.Code != 405 {
			t.Fatal("unsafe request accepted")
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1/api/files", nil)
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross origin request accepted")
	}

}

type sseMessage struct {
	kind, id string
	data     []byte
}

func nextMessage(t *testing.T, reader *bufio.Reader) sseMessage {
	t.Helper()
	var m sseMessage
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" && m.kind != "" {
			return m
		}
		if strings.HasPrefix(line, "event: ") {
			m.kind = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "id: ") {
			m.id = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			m.data = []byte(strings.TrimPrefix(line, "data: "))
		}
	}
}

func TestTailReplaysResumesAndFollowsUntilCompletion(t *testing.T) {
	dir := t.TempDir()
	e := fixture(t, dir, "live", time.Now().UTC(), false)
	name := e.Filename
	put(t, dir, name, []byte{0, 255, 1, 2})
	s, _ := indexedStore(t, dir)
	server := httptest.NewServer(NewLogHandler(s))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	open := func(offset string) *http.Response {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/tail?name="+url.QueryEscape(name), nil)
		if offset != "" {
			req.Header.Set("Last-Event-ID", offset)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := open("")
	reader := bufio.NewReader(resp.Body)
	first := nextMessage(t, reader)
	if first.kind != "chunk" || first.id != "4" {
		t.Fatalf("bad replay: %+v", first)
	}
	_ = resp.Body.Close()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte{3, 4, 255})
	_ = f.Close()
	resp = open(first.id)
	defer resp.Body.Close()
	reader = bufio.NewReader(resp.Body)
	second := nextMessage(t, reader)
	var chunk struct {
		Data   string
		Offset int64
	}
	_ = json.Unmarshal(second.data, &chunk)
	data, _ := base64.StdEncoding.DecodeString(chunk.Data)
	if second.id != "7" || chunk.Offset != 4 || !bytes.Equal(data, []byte{3, 4, 255}) {
		t.Fatal("resume lost or duplicated bytes")
	}
	// Append while the same HTTP stream is open; then publish completion.
	f, _ = os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND, 0600)
	_, _ = f.Write([]byte("last"))
	_ = f.Close()
	e.Event = "request_completed"
	e.Completed = true
	f, _ = os.OpenFile(filepath.Join(dir, "live_metadata.jsonl"), os.O_WRONLY|os.O_APPEND, 0600)
	_, _ = f.Write(eventLine(t, e))
	_ = f.Close()
	_ = s.Scan()
	third := nextMessage(t, reader)
	if third.kind != "chunk" || third.id != "11" {
		t.Fatalf("missed last append: %+v", third)
	}
	end := nextMessage(t, reader)
	if end.kind != "end" || end.id != "11" {
		t.Fatalf("bad end: %+v", end)
	}
}

func TestChangesAndEmbeddedAssets(t *testing.T) {
	dir := t.TempDir()
	s, _ := indexedStore(t, dir)
	server := httptest.NewServer(NewLogHandler(s))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	first := nextMessage(t, reader)
	fixture(t, dir, "new", time.Now().UTC(), false)
	_ = s.Scan()
	second := nextMessage(t, reader)
	if first.kind != "change" || second.kind != "change" || bytes.Equal(first.data, second.data) {
		t.Fatal("change not delivered")
	}

	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	index, _ := io.ReadAll(response.Body)
	response.Body.Close()
	paths := []string{"/", "/logs/"}
	for _, match := range regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindAllStringSubmatch(string(index), -1) {
		paths = append(paths, match[1])
	}
	if len(paths) < 4 {
		t.Fatal("React entrypoint does not reference built JS and CSS assets")
	}
	for _, path := range paths {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal(fmt.Sprint(path, response.StatusCode))
		}
		if strings.HasSuffix(path, ".js") && !strings.Contains(response.Header.Get("Content-Type"), "javascript") {
			t.Fatalf("invalid JS MIME type: %s", response.Header.Get("Content-Type"))
		}
		if response.Header.Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}

}

func TestRequestHistogramCountsBeyondPageAndHonorsFilters(t *testing.T) {
	now := time.Now().UTC()
	items := []Transaction{
		{Started: now.Add(-5 * time.Minute), State: "completed", Metadata: capture.Metadata{Method: "GET", SourceURL: "https://example.test/match"}},
		{Started: now.Add(-10 * time.Minute), State: "completed", Metadata: capture.Metadata{Method: "GET", SourceURL: "https://example.test/match"}},
		{Started: now.Add(-15 * time.Minute), State: "error"},
		{Started: now.Add(-8 * 24 * time.Hour), State: "completed"},
		{Started: now.Add(time.Hour), State: "completed"},
	}
	s, err := indexedStore(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		item.Key = fmt.Sprint(i)
		data, _ := json.Marshal(item)
		err := s.db.Q.PutRecord(context.Background(), queries.PutRecordParams{Source: s.dir, MetadataFile: item.Key + "_metadata.jsonl", Key: item.Key, Started: item.Started.UnixNano(), State: item.State, Method: item.Metadata.Method, SearchText: item.Metadata.SourceURL, Summary: data})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query    string
		count    int
		bins     int
		interval int
	}{
		{"limit=1", 3, 48, 1800},
		{"limit=1&offset=1&q=match&state=completed&hours=1", 2, 30, 120},
		{"hours=168&state=error", 1, 56, 10800},
		{"q=absent", 0, 48, 1800},
	} {
		w := request(t, NewLogHandler(s), "/api/requests?"+tc.query)
		var result struct{ Histogram requestHistogram }
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("invalid response: %s", w.Body.String())
		}
		h := result.Histogram
		sum := 0
		for _, b := range h.Buckets {
			sum += b.Count
		}
		if sum != tc.count || h.Total != sum || len(h.Buckets) != tc.bins || h.IntervalSeconds != tc.interval {
			t.Fatalf("%s: unexpected histogram: %+v", tc.query, h)
		}
	}
	if w := request(t, NewLogHandler(s), "/api/requests?hours=bad"); w.Code != http.StatusBadRequest {
		t.Fatal("invalid histogram range accepted")
	}
	h, _ := newRequestHistogram("1", time.Date(2026, 10, 14, 12, 1, 0, 0, time.UTC))
	if !h.Buckets[0].Started.Equal(time.Date(2026, 10, 14, 11, 2, 0, 0, time.UTC)) || !h.Buckets[29].Started.Equal(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("incorrect aligned bucket boundaries")
	}
}

func TestHistoryInferenceMetrics(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().UTC()
	e := fixture(t, dir, "inference", start, false)
	body := e
	body.Event = "request_completed"
	body.Metadata.Model = "requested-model"
	body.Timestamp = start.Add(time.Millisecond)
	response := e
	response.StreamType = "response"
	response.Event = "response_completed"
	response.Timestamp = start.Add(2 * time.Millisecond)
	response.Metadata.ResponseStatusCode = 200
	zero, output := int64(0), int64(12)
	response.Metadata.InputTokens = &zero
	response.Metadata.OutputTokens = &output
	response.Metadata.FinishReason = "stop"
	data := append(eventLine(t, e), eventLine(t, body)...)
	data = append(data, eventLine(t, response)...)
	put(t, dir, "inference_metadata.jsonl", data)
	s, err := indexedStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, NewLogHandler(s), "/api/requests?q=requested-model")
	var result struct {
		Items []Transaction
		Total int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("model search failed: %s", w.Body.String())
	}
	m := result.Items[0].Metadata
	if m.Model != "requested-model" || m.InputTokens == nil || *m.InputTokens != 0 || m.OutputTokens == nil || *m.OutputTokens != 12 || m.FinishReason != "stop" {
		t.Fatalf("metrics lost while merging lifecycle events: %+v", m)
	}
}
