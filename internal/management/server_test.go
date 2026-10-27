package management

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestManagementLifecycleConfigurationAndSecurity(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	directory := t.TempDir()
	filename := filepath.Join(directory, "config.yaml")
	original := []byte(strings.Replace(service.DefaultConfig, "port: 5601", "port: "+jsonNumber(port), 1) + "custom_extension:\n  keep: yes\n")
	if err := os.WriteFile(filename, original, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := service.NewManager(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	server, err := New(manager, Options{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	request := func(method, path string, body any, origin, token string) (int, []byte) {
		t.Helper()
		var content io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			content = bytes.NewReader(data)
		}
		req, _ := http.NewRequest(method, server.URL+path, content)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if token != "" {
			req.Header.Set("X-Tokenhub-Session", token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return response.StatusCode, data
	}
	// Foreign origins and forged sessions cannot operate the service.
	for _, credentials := range [][2]string{{"https://evil.example", server.session}, {server.URL, "wrong"}, {"", server.session}} {
		code, _ := request("POST", "/api/service/start", map[string]any{}, credentials[0], credentials[1])
		if code != 403 {
			t.Fatalf("unauthorized mutation accepted: %d", code)
		}
	}
	code, _ := request("POST", "/api/service/start", map[string]any{}, server.URL, server.session)
	if code != 400 || manager.Status().State != "error" {
		t.Fatal("occupied listener did not produce startup error")
	}
	listener.Close()
	code, data := request("POST", "/api/service/start", map[string]any{}, server.URL, server.session)
	if code != 200 || manager.Status().State != "running" {
		t.Fatalf("start: %d %s", code, data)
	}

	for _, path := range []string{"/api/environment", "/api/environment/profiles"} {
		for _, method := range []string{"GET", "POST"} {
			if code, _ := request(method, path, map[string]any{}, server.URL, server.session); code != http.StatusNotFound {
				t.Fatalf("removed endpoint %s %s returned %d", method, path, code)
			}
		}
	}
	changed := strings.Replace(string(original), "log_dir: \"logs\"", "log_dir: \"next-logs\"", 1)
	code, data = request("POST", "/api/config", configInput{YAML: changed, Version: version(original)}, server.URL, server.session)
	if code != 200 || !manager.Status().Pending {
		t.Fatalf("save: %d %s", code, data)
	}
	persisted, _ := os.ReadFile(filename)
	if !bytes.Contains(persisted, []byte("custom_extension:")) {
		t.Fatal("unknown field lost")
	}
	code, _ = request("GET", "/api/files", nil, "", "")
	if code != 200 || server.logDir != filepath.Join(directory, "logs") {
		t.Fatal("viewer switched directories before restart")
	}
	code, _ = request("POST", "/api/config", configInput{YAML: string(original), Version: version(original)}, server.URL, server.session)
	if code != 409 {
		t.Fatal("stale write accepted")
	}
	code, _ = request("POST", "/api/config", configInput{YAML: "server: [invalid", Version: version(persisted)}, server.URL, server.session)
	if code != 400 {
		t.Fatal("invalid YAML accepted")
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(persisted, after) {
		t.Fatal("invalid save replaced configuration")
	}
	code, data = request("POST", "/api/service/restart", map[string]any{}, server.URL, server.session)
	if code != 200 || manager.Status().Pending {
		t.Fatalf("restart: %d %s", code, data)
	}
	code, _ = request("POST", "/api/service/stop", map[string]any{}, server.URL, server.session)
	if code != 200 || manager.Status().State != "stopped" {
		t.Fatal("stop failed")
	}
	code, _ = request("GET", "/api/files", nil, "", "")
	if code != 200 || server.logDir != filepath.Join(directory, "next-logs") {
		t.Fatal("viewer did not retain the last active directory after stopping")
	}
	listener, err = net.Listen("tcp", "127.0.0.1:"+jsonNumber(port))
	if err != nil {
		t.Fatal("stop retained port", err)
	}
	listener.Close()
	code, data = request("GET", "/logs/", nil, "", "")
	if code != 200 || !bytes.Contains(data, []byte(`id="root"`)) {
		t.Fatalf("logs unavailable with proxy stopped: %d %s", code, data)
	}
	code, _ = request("GET", "/settings/providers", nil, "", "")
	if code != 200 {
		t.Fatal("SPA route unavailable")
	}
	code, _ = request("GET", "/assets/missing.js", nil, "", "")
	if code != 404 {
		t.Fatal("missing asset fell back to HTML")
	}
	req, _ := http.NewRequest("GET", server.URL+"/api/config", nil)
	req.Host = "evil.example"
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("foreign host accepted")
	}
}

func jsonNumber(number int) string { data, _ := json.Marshal(number); return string(data) }

func TestConfigUsesLiteralCredentialsAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.yaml")
	data := service.DefaultConfig
	data = strings.Replace(data, "providers: {}", "providers:\n  local:\n    base_url: http://127.0.0.1:1234/v1/\n    active_account: personal\n    accounts: [{id: personal, name: Personal, key: literal-secret}]", 1)
	manager, err := service.NewManager(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Save([]byte(data)); err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(filename)
	if !bytes.Contains(stored, []byte("literal-secret")) {
		t.Fatal("literal credential was not persisted")
	}
	cfg, err := service.LoadConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.LogDir != filepath.Join(dir, "logs") {
		t.Fatal("log path not relative to configuration")
	}
}

func TestDirectoryActionsUseManagedPaths(t *testing.T) {
	manager, err := service.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var opened []string
	server, err := New(manager, Options{Address: "127.0.0.1:0", OpenDirectory: func(directory string) error { opened = append(opened, directory); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	for _, kind := range []string{"config", "logs"} {
		req, err := http.NewRequest("POST", server.URL+"/api/directories/"+kind+"/open", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", server.URL)
		req.Header.Set("X-Tokenhub-Session", server.session)
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("%s: %d", kind, response.StatusCode)
		}
	}
	if len(opened) != 2 || opened[0] != filepath.Dir(manager.Filename) || opened[1] != filepath.Join(filepath.Dir(manager.Filename), "logs") {
		t.Fatalf("unexpected Finder directories: %v", opened)
	}
}
