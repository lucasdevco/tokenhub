package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderPreservesNativePathsAndUpstreamErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		fmt.Fprintf(w, `{"path":%q}`, r.URL.RequestURI())
	}))
	defer upstream.Close()
	proxy := NewProxyServer()
	if err := proxy.AddProvider("moark", upstream.URL+"/api/", nil, ProviderOptions{}); err != nil {
		t.Fatal(err)
	}
	// Registration alone never implicitly activates a provider.
	empty := httptest.NewRecorder()
	proxy.ServeHTTP(empty, httptest.NewRequest("GET", "/v1/models", nil))
	if empty.Code != 503 || !strings.Contains(empty.Body.String(), "provider_not_selected") {
		t.Fatal(empty.Code, empty.Body.String())
	}
	if err := proxy.SelectProvider("moark"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/files/a%2Fb?x=1&x=2", "/v1/a//b/../c"} {
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest("POST", path, strings.NewReader("{}")))
		want := fmt.Sprintf(`{"path":%q}`, "/api"+path)
		if response.Code != 404 || response.Body.String() != want {
			t.Fatalf("%s: %d %s, want %s", path, response.Code, response.Body.String(), want)
		}
	}
	if err := proxy.SelectProvider("missing"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if proxy.active.Load().name != "moark" {
		t.Fatal("failed selection changed provider")
	}
	if err := proxy.SelectProvider(""); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 503 {
		t.Fatal(response.Code)
	}
}

func TestProviderSwitchLeavesActiveStreamRunning(t *testing.T) {
	release := make(chan struct{})
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Repeat("a", 8192))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "old-complete")
	}))
	defer old.Close()
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "new-provider") }))
	defer next.Close()
	proxy := NewProxyServer()
	for name, address := range map[string]string{"old": old.URL, "next": next.URL} {
		if err := proxy.AddProvider(name, address, nil, ProviderOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := proxy.SelectProvider("old"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	defer close(release)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server.URL + "/v1/responses")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	prefix := make([]byte, 8192)
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prefix, bytes.Repeat([]byte("a"), 8192)) {
		t.Fatal("stream changed")
	}
	if err := proxy.SelectProvider("next"); err != nil {
		t.Fatal(err)
	}
	newer, err := client.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer newer.Body.Close()
	data, err := io.ReadAll(newer.Body)
	if err != nil || string(data) != "new-provider" {
		t.Fatalf("new request: %s %v", data, err)
	}
	release <- struct{}{}
	data, err = io.ReadAll(response.Body)
	if err != nil || string(data) != "old-complete" {
		t.Fatalf("old stream: %s %v", data, err)
	}
}

func TestProviderRejectsInvalidBaseURLs(t *testing.T) {
	for _, address := range []string{"", "/relative", "ftp://example.com", "http://", "https://user:secret@example.com", "https://example.com?key=secret", "https://example.com#fragment"} {
		if err := NewProxyServer().AddProvider("test", address, nil, ProviderOptions{}); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
}
