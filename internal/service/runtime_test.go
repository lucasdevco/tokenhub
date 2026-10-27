package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/lucasdevco/tokenhub/internal/capture"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartRejectsOccupiedListener(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	cfg, err := ParseConfig([]byte(fmt.Sprintf("server:\n  host: 127.0.0.1\n  port: %d\nhttp_client:\n  proxy_from_environment: false\n", occupied.Addr().(*net.TCPAddr).Port)), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if run, err := Start(context.Background(), cfg); err == nil {
		run.Stop()
		t.Fatal("occupied proxy listener accepted")
	}
}

func TestAtomicSaveRejectsBadPorts(t *testing.T) {
	manager, err := NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(manager.Filename)
	if err := manager.Save([]byte("server:\n  port: 70000\n")); err == nil {
		t.Fatal("invalid port accepted")
	}
	after, _ := os.ReadFile(manager.Filename)
	if string(after) != string(before) {
		t.Fatal("valid file overwritten")
	}
}

func TestStopClosesUpgradedConnections(t *testing.T) {
	ended := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		defer close(ended)
		_, _ = fmt.Fprint(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_, _ = io.Copy(io.Discard, conn)
	}))
	defer backend.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cfg, err := ParseConfig([]byte(fmt.Sprintf("server:\n  host: 127.0.0.1\n  port: %d\nhttp_client:\n  proxy_from_environment: false\nactive_provider: socket\nproviders:\n  socket:\n    base_url: %s/\n", port, backend.URL)), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer run.Stop()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", address)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 101 {
		t.Fatalf("upgrade: %d", response.StatusCode)
	}
	if err := run.Stop(); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("upgraded connection remained open")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("stop did not close upgraded connection")
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("upstream upgraded connection remained open")
	}
	run.mu.Lock()
	remaining := len(run.connections)
	run.mu.Unlock()
	if remaining != 0 {
		t.Fatal("closed connections retained")
	}
}

func TestProxyURLIsConnectable(t *testing.T) {
	for _, example := range []struct{ host, want string }{{"127.0.0.1", "http://127.0.0.1:5601"}, {"0.0.0.0", "http://127.0.0.1:5601"}, {"::", "http://[::1]:5601"}, {"::1", "http://[::1]:5601"}, {"localhost", "http://localhost:5601"}} {
		if got := proxyURL(example.host, 5601); got != example.want {
			t.Fatalf("%s: got %s want %s", example.host, got, example.want)
		}
	}
}

func TestStopDrainsRequestResponseAndProxyOutcome(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer backend.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	dir := t.TempDir()
	config, err := ParseConfig([]byte(fmt.Sprintf("server:\n  host: 127.0.0.1\n  port: %d\nlogging:\n  enabled: true\n  log_dir: %s\nhttp_client:\n  proxy_from_environment: false\nactive_provider: test\nproviders:\n  test:\n    base_url: %s\n", port, dir, backend.URL)), filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := Start(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer run.Stop()
	response, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/", port), "application/octet-stream", bytes.NewReader(bytes.Repeat([]byte("x"), 2<<20)))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err := run.Stop(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*_metadata.jsonl"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"request_completed", "response_completed", "proxy_completed"} {
		if !bytes.Contains(data, []byte(`"event":"`+event+`"`)) {
			t.Fatalf("stop returned before %s persisted", event)
		}
	}
}

func TestStopClosesRecorderWithoutProviders(t *testing.T) {
	cfg := &Config{}
	cfg.Logging.Enabled = true
	cfg.Logging.LogDir = t.TempDir()
	cfg.DatabasePath = database.Path(t.TempDir())
	run, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Stop(); err != nil {
		t.Fatal(err)
	}
	tx := run.recorder.Begin(context.Background(), capture.Metadata{RequestStartedAt: time.Now()})
	tx.CaptureRequest(httptest.NewRequest("GET", "http://unused.test/", nil), false)
	tx.End(nil)
	if err := run.recorder.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(cfg.Logging.LogDir, "*_metadata.jsonl"))
	if err != nil || len(files) != 0 {
		t.Fatal("stopped recorder still accepts writes", files, err)
	}
}
