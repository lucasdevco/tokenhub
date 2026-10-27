package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
)

func captureFixture(t *testing.T, backend string, enabled bool) (*ProxyServer, *httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	var recorder *capture.Recorder
	var err error
	if enabled {
		recorder, err = capture.NewRecorder(dir, database.Path(dir), false)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := NewProxyServer()
	if err := useTestProvider(p, "test", backend, recorder); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(p)
	t.Cleanup(func() { server.Close(); drainCaptures(t, p) })
	return p, server, dir
}

func drainCaptures(t *testing.T, p *ProxyServer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.ShutdownCaptures(ctx); err != nil {
		t.Fatal(err)
	}
}

func captureEvents(t *testing.T, dir string) []capture.Event {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_metadata.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("metadata files: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var events []capture.Event
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var event capture.Event
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestSmallSSEArrivesBeforeUpstreamCompletes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			release := make(chan struct{})
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: first\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			defer backend.Close()
			defer close(release)
			_, server, _ := captureFixture(t, backend.URL, enabled)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			line, err := bufio.NewReader(resp.Body).ReadString('\n')
			if err != nil || line != "data: first\n" {
				t.Fatalf("SSE buffered: %q %v", line, err)
			}
		})
	}
}

func TestProxyPreservesRedirectAndRemovesHopHeaders(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			seen := make(chan string, 2)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- r.Header.Get("X-Hop-Only")
				if r.URL.Path != "/original" {
					t.Error("proxy followed redirect")
				}
				w.Header().Set("Location", "/target")
				w.Header().Set("Connection", "X-Response-Hop")
				w.Header().Set("X-Response-Hop", "secret")
				w.WriteHeader(302)
			}))
			defer backend.Close()
			_, server, _ := captureFixture(t, backend.URL, enabled)
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			req, _ := http.NewRequest("POST", server.URL+"/original", strings.NewReader("body"))
			req.Header.Set("Connection", "X-Hop-Only")
			req.Header.Set("X-Hop-Only", "private")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 302 || resp.Header.Get("Location") != "/target" || resp.Header.Get("X-Response-Hop") != "" || <-seen != "" {
				t.Fatalf("proxy changed protocol semantics: %v", resp)
			}
		})
	}
}

func TestTLSCompressionPassesThroughAndCapturesDecodedBodies(t *testing.T) {
	payload := `{"model":"test","input":"hello"}`
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	io.WriteString(gz, payload)
	gz.Close()
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Encoding") != "gzip" || !bytes.Equal(body, compressed.Bytes()) {
			t.Error("wire request changed")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		w.Write(compressed.Bytes())
	}))
	defer backend.Close()
	p, server, dir := captureFixture(t, backend.URL, true)
	p.transport = backend.Client().Transport
	req, _ := http.NewRequest("POST", server.URL+"/v1/responses", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("User-Agent", "review-client")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" || !bytes.Equal(data, compressed.Bytes()) {
		t.Fatal("wire response changed")
	}
	drainCaptures(t, p)
	events := captureEvents(t, dir)
	for _, e := range events {
		if !strings.HasSuffix(e.Event, "_completed") || e.StreamType == "proxy" {
			continue
		}
		if !e.Completed || e.Metadata.SourceURL != server.URL+"/v1/responses" || e.Metadata.UserAgent != "review-client" {
			t.Fatalf("bad metadata: %+v", e)
		}
		data, _ := os.ReadFile(filepath.Join(dir, e.Filename))
		header, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
		if string(body) != payload || bytes.Contains(header, []byte("Content-Encoding:")) || bytes.Contains(header, []byte("Content-Length:")) {
			t.Fatalf("decoded capture inconsistent: %s", data)
		}
	}
}

func TestUpstreamFailureAndInterruptedBodyAreRecorded(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		t.Run(fmt.Sprint(truncated), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "100")
				io.WriteString(w, "partial")
			}))
			defer backend.Close()
			if !truncated {
				backend.Close()
			}
			p, server, dir := captureFixture(t, backend.URL, true)
			resp, err := http.Get(server.URL)
			if err == nil {
				_, err = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			if truncated && err == nil {
				t.Fatal("truncated response appeared successful")
			}
			drainCaptures(t, p)
			found := false
			for _, e := range captureEvents(t, dir) {
				if e.Event != "response_completed" {
					continue
				}
				found = true
				if e.Completed || e.Error == "" || e.CaptureError != "" {
					t.Fatalf("wrong failure classification: %+v", e)
				}
				if !truncated && (e.Metadata.ResponseStatusCode != 502 || e.Metadata.UpstreamResponseAt != nil) {
					t.Fatal("502 claims upstream response", e)
				}
			}
			if !found {
				t.Fatal("completion missing")
			}
		})
	}
}

func TestClientDisconnectPreservesPartialCapture(t *testing.T) {
	canceled := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: partial\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer backend.Close()
	p, server, dir := captureFixture(t, backend.URL, true)
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: local\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := make([]byte, len("data: partial\n\n"))
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream not canceled")
	}
	server.Close()
	drainCaptures(t, p)
	for _, e := range captureEvents(t, dir) {
		if e.Event == "response_completed" {
			data, _ := os.ReadFile(filepath.Join(dir, e.Filename))
			if e.Completed || !bytes.HasSuffix(data, prefix) {
				t.Fatalf("partial capture lost: %+v %q", e, data)
			}
			return
		}
	}
	t.Fatal("completion missing")
}

func TestCaptureDiskFailureDoesNotBreakProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This checks storage failure isolation, not HTTP/1 full-duplex echoing.
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(data)
	}))
	defer backend.Close()
	p, server, dir := captureFixture(t, backend.URL, true)
	if err := os.Rename(dir, dir+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("blocked directory"), 0600); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("payload", 100000)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(server.URL, "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(got) != body {
		t.Fatal("storage failure broke forwarding", err)
	}
	drainCaptures(t, p)
}
