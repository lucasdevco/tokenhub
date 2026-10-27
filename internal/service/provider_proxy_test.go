package service

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderProxyToggleAppliesImmediatelyAndPersists(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct")
	}))
	defer backend.Close()
	upstreamProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxied")
	}))
	defer upstreamProxy.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	filename := filepath.Join(t.TempDir(), "config.yaml")
	source := fmt.Sprintf("server: {host: 127.0.0.1, port: %d}\nhttp_client: {proxy_url: %q}\nactive_provider: api\nproviders:\n  api: {base_url: %q}\n  other: {base_url: %q}\n", port, upstreamProxy.URL, backend.URL, backend.URL)
	if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	request := func(want string) {
		t.Helper()
		resp, err := http.Get(manager.Status().Address + "/v1/models")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || string(body) != want {
			t.Fatalf("response = %q, error = %v, want %q", body, err, want)
		}
	}
	request("proxied") // An omitted flag retains the existing global behavior.
	catalog, err := manager.ReadProviders()
	if err != nil {
		t.Fatal(err)
	}
	view := catalog.Services[0]
	if view.ID != "api" || !view.ProxyEnabled {
		t.Fatalf("unexpected provider: %+v", view)
	}
	view.ProxyEnabled = false
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "save", Service: view, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Services[0].ProxyEnabled || manager.Status().Pending {
		t.Fatal("proxy bypass was not applied")
	}
	request("direct")
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "select", Provider: "other", Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	request("proxied")
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "select", Provider: "api", Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Restart(); err != nil {
		t.Fatal(err)
	}
	request("direct")
	view.ProxyEnabled = true
	_, err = manager.ChangeProvider(ProviderChange{Action: "save", Service: view, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	request("proxied")
}
