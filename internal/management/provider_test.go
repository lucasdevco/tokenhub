package management

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestProviderEndpointRequiresSessionAndReturnsUpdatedConfiguration(t *testing.T) {
	manager, err := service.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Save([]byte("server: {}\nproviders:\n  local: {base_url: 'http://127.0.0.1:1234'}\n")); err != nil {
		t.Fatal(err)
	}
	server, err := New(manager, Options{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	original, _ := os.ReadFile(manager.Filename)
	for _, authorized := range []bool{false, true} {
		body, _ := json.Marshal(map[string]string{"provider": "local", "version": version(original)})
		request, _ := http.NewRequest("POST", server.URL+"/api/provider", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", server.URL)
		if authorized {
			request.Header.Set("X-Tokenhub-Session", server.session)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if !authorized {
			if response.StatusCode != 403 {
				t.Fatal("unauthenticated switch accepted", response.StatusCode)
			}
			current, _ := os.ReadFile(manager.Filename)
			if !bytes.Equal(current, original) {
				t.Fatal("unauthenticated switch changed config")
			}
			continue
		}
		if response.StatusCode != 200 {
			t.Fatalf("%d: %s", response.StatusCode, data)
		}
		var result struct{ YAML, Version, Filename string }
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Version == version(original) || result.Filename != manager.Filename || manager.Status().ActiveProvider != "local" {
			t.Fatalf("incomplete switch response: %s", data)
		}
		if _, err := service.ParseConfig([]byte(result.YAML), manager.Filename); err != nil {
			t.Fatal(err)
		}
	}
}
