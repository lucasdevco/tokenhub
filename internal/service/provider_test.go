package service

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerProviderSelectionPersistenceAndConflicts(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "first:"+r.URL.RequestURI()) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "second:"+r.URL.RequestURI()) }))
	defer second.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	dir := t.TempDir()
	manager, err := NewManager(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf("# preserved\nserver: {host: 127.0.0.1, port: %d}\nhttp_client: {proxy_from_environment: false}\nproviders:\n  first:\n    base_url: %s\n    authorization: {backend_key: literal-secret}\n  second:\n    base_url: %s/api\ncustom: keep\n", port, first.URL, second.URL)
	if err := manager.Save([]byte(source)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	initialRun := manager.run
	version := func() string {
		t.Helper()
		data, err := os.ReadFile(manager.Filename)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	request := func(wantStatus int, wantBody string) {
		t.Helper()
		response, err := http.Get(manager.Status().Address + "/v1/messages?x=1")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != wantStatus || !strings.Contains(string(body), wantBody) {
			t.Fatalf("%d %s %v", response.StatusCode, body, err)
		}
	}
	request(503, "provider_not_selected")
	stale := version()
	if err := manager.SelectProvider("first", stale); err != nil {
		t.Fatal(err)
	}
	request(200, "first:/v1/messages?x=1")
	if manager.run != initialRun || manager.Status().Pending || manager.Status().ActiveProvider != "first" {
		t.Fatal("selection restarted the listener or left config pending")
	}
	if err := manager.SelectProvider("second", stale); err == nil {
		t.Fatal("stale version accepted")
	}
	before := version()
	if err := manager.SelectProvider("missing", before); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if version() != before {
		t.Fatal("failed selection overwrote config")
	}
	if err := manager.SelectProvider("second", version()); err != nil {
		t.Fatal(err)
	}
	request(200, "second:/api/v1/messages?x=1")
	stored, err := os.ReadFile(manager.Filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "# preserved") || !strings.Contains(string(stored), "literal-secret") || !strings.Contains(string(stored), "custom: keep") {
		t.Fatalf("source not preserved: %s", stored)
	}
	if err := manager.Save(append(stored, []byte("# pending change\n")...)); err != nil {
		t.Fatal(err)
	}
	if err := manager.SelectProvider("first", version()); err == nil {
		t.Fatal("pending configuration was silently applied")
	}
	request(200, "second:")
	if err := manager.Restart(); err != nil {
		t.Fatal(err)
	}
	request(200, "second:")
	// Legacy environment files cannot affect pending state or provider selection.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=ignored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if manager.Status().Pending {
		t.Fatal("env file change marked configuration pending")
	}
	if err := manager.SelectProvider("first", version()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := manager.SelectProvider("first", version()); err != nil {
		t.Fatal(err)
	}
	if manager.Status().ActiveProvider != "first" {
		t.Fatal("stopped selection not reflected in status")
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	request(200, "first:")
	if err := manager.SelectProvider("", version()); err != nil {
		t.Fatal(err)
	}
	request(503, "provider_not_selected")
}
