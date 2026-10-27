package management

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestProvidersSessionCRUDAndVersionProtection(t *testing.T) {
	manager, err := service.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(manager, Options{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	call := func(method string, body any, authorized bool) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(body)
		request, _ := http.NewRequest(method, server.URL+"/api/providers", bytes.NewReader(data))
		request.Header.Set("Origin", server.URL)
		request.Header.Set("Content-Type", "application/json")
		if authorized {
			request.Header.Set("X-Tokenhub-Session", server.session)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		result, _ := io.ReadAll(response.Body)
		return response.StatusCode, result
	}
	if status, data := call("GET", nil, false); status != 403 || bytes.Contains(data, []byte("services")) {
		t.Fatal("catalog leaked without session")
	}
	status, data := call("GET", nil, true)
	if status != 200 {
		t.Fatalf("%d %s", status, data)
	}
	var catalog service.ProviderCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	change := service.ProviderChange{Action: "save", New: true, Activate: true, Version: catalog.Version, Service: service.ProviderView{ID: "api", Name: "服务", Address: "https://example.com", Header: "Authorization", Template: "Bearer {}", Accounts: []service.ProviderAccount{{ID: "personal", Name: "个人", Key: "private-secret"}}, Active: "personal"}}
	if status, _ := call("POST", change, false); status != 403 {
		t.Fatal("unauthorized mutation accepted")
	}
	status, data = call("POST", change, true)
	if status != 200 {
		t.Fatalf("%d %s", status, data)
	}
	if !bytes.Contains(data, []byte("private-secret")) {
		t.Fatal("saved account not returned")
	}
	if status, _ := call("POST", change, true); status != 409 {
		t.Fatalf("stale save status %d", status)
	}
}
