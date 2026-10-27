package service

import (
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

func TestProviderAccountsPersistAndSwitchWithoutInterruptingRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		if r.URL.Path == "/slow" {
			close(started)
			<-release
		}
		_, _ = io.WriteString(w, key)
	}))
	defer upstream.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	manager, err := NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf("# preserved\nserver: {host: 127.0.0.1, port: %d}\nlogging: {enabled: false}\nhttp_client: {proxy_from_environment: false}\nactive_provider: api\nproviders:\n  api:\n    base_url: %s\n    custom: keep\n    active_account: personal\n    accounts: [{id: personal, name: Personal, key: first}, {id: work, name: Work, key: second}]\n    authorization: {custom_auth: keep}\ncustom_root: keep\n", port, upstream.URL)
	if err := manager.Save([]byte(source)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	initialRun := manager.run
	catalog, err := manager.ReadProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Services) != 1 || len(catalog.Services[0].Accounts) != 2 || catalog.Services[0].Active != "personal" {
		t.Fatalf("configured accounts not exposed: %#v", catalog.Services)
	}
	request := func(path string) string {
		response, err := http.Get(manager.Status().Address + path)
		if err != nil {
			return err.Error()
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return string(data)
	}
	slow := make(chan string, 1)
	go func() { slow <- request("/slow") }()
	<-started
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	oldVersion := catalog.Version
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "account", Provider: "api", Account: "work", Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if manager.run != initialRun || manager.Status().Pending {
		t.Fatal("account switch restarted or left pending changes")
	}
	if got := request("/v1/messages"); got != "Bearer second" {
		t.Fatalf("new request: %q", got)
	}
	close(release)
	if got := <-slow; got != "Bearer first" {
		t.Fatalf("in-flight request changed: %q", got)
	}
	if _, err := manager.ChangeProvider(ProviderChange{Action: "select", Provider: "api", Version: oldVersion}); err != ErrProviderConflict {
		t.Fatalf("stale switch accepted: %v", err)
	}
	before, _ := os.ReadFile(manager.Filename)
	bad := catalog.Services[0]
	bad.Accounts = append([]ProviderAccount(nil), bad.Accounts...)
	bad.Accounts[1].Key = "bad\r\nInjected: header"
	if _, err := manager.ChangeProvider(ProviderChange{Action: "save", Service: bad, Version: catalog.Version}); err == nil {
		t.Fatal("header injection accepted")
	}
	after, _ := os.ReadFile(manager.Filename)
	if string(before) != string(after) {
		t.Fatal("invalid update changed configuration")
	}
	view := catalog.Services[0]
	view.Name = "我的服务"
	view.Logging = true // first capture recorder is created during the live update
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "save", Service: view, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if manager.run.recorder == nil || manager.Status().Pending {
		t.Fatal("logging was not applied")
	}
	stored, _ := os.ReadFile(manager.Filename)
	for _, preserved := range []string{"# preserved", "custom: keep", "custom_auth: keep", "custom_root: keep"} {
		if !strings.Contains(string(stored), preserved) {
			t.Errorf("lost %q", preserved)
		}
	}
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	if got := request("/v1/messages"); got != "Bearer second" {
		t.Fatalf("restart lost account: %q", got)
	}
	info, _ := os.Stat(manager.Filename)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions: %v", info.Mode())
	}
	catalog, _ = manager.ReadProviders()
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "delete", Provider: "api", Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Active != "" || len(catalog.Services) != 0 || !strings.Contains(request("/v1/messages"), "provider_not_selected") {
		t.Fatal("deleted provider still selected")
	}
	persisted, _ := os.ReadFile(manager.Filename)
	if err := manager.Save(append(persisted, []byte("# unrelated pending change\n")...)); err != nil {
		t.Fatal(err)
	}
	pending, _ := manager.ReadProviders()
	if _, err := manager.ChangeProvider(ProviderChange{Action: "select", Version: pending.Version}); err == nil {
		t.Fatal("pending settings were silently applied")
	}
	// Stale files cannot affect the catalog, version or account list.
	beforeEnv, _ := manager.ReadProviders()
	for _, name := range []string{".env", ".env-profiles.json"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(manager.Filename), name), []byte("invalid stale content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	afterEnv, err := manager.ReadProviders()
	if err != nil || beforeEnv.Version != afterEnv.Version {
		t.Fatalf("legacy files affected catalog: %v", err)
	}

}

func TestProviderCRUDAndValidation(t *testing.T) {
	manager, err := NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := manager.ReadProviders()
	if err != nil {
		t.Fatal(err)
	}
	view := ProviderView{ID: "local", Name: "本地", Address: "http://localhost:11434", Accounts: []ProviderAccount{}, Header: "Authorization", Template: "Bearer {}"}
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "save", New: true, Activate: true, Service: view, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Active != "local" {
		t.Fatal("new provider not selected")
	}
	for _, invalid := range []ProviderView{
		{ID: "local", Name: "本地", Address: "ftp://localhost"},
		{ID: "local", Name: "本地", Address: view.Address, Active: "missing"},
		{ID: "local", Name: "本地", Address: view.Address, Accounts: []ProviderAccount{{ID: "a", Name: "账号", Key: "secret"}, {ID: "a", Name: "其他", Key: "secret"}}, Active: "a"},
	} {
		if _, err := manager.ChangeProvider(ProviderChange{Action: "save", Service: invalid, Version: catalog.Version}); err == nil {
			t.Fatal("invalid provider accepted")
		}
	}
	if _, err := manager.ChangeProvider(ProviderChange{Action: "save", New: true, Service: view, Version: catalog.Version}); err == nil {
		t.Fatal("creation replaced existing provider")
	}
	duplicate := view
	duplicate.ID = " local "
	if _, err := manager.ChangeProvider(ProviderChange{Action: "save", New: true, Service: duplicate, Version: catalog.Version}); err == nil {
		t.Fatal("whitespace ID replaced an existing provider")
	}
	view.Accounts = []ProviderAccount{{ID: "personal", Name: "个人", Key: "first"}, {ID: "work", Name: "公司", Key: "second"}}
	view.Active = "work"
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "save", Service: view, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	other := ProviderView{ID: "other", Name: "另一个", Address: view.Address, Accounts: []ProviderAccount{}}
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "save", Service: other, New: true, Activate: true, Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err = manager.ChangeProvider(ProviderChange{Action: "select", Provider: "local", Version: catalog.Version})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Active != "local" {
		t.Fatal("service selection lost")
	}
	for _, service := range catalog.Services {
		if service.ID == "local" && service.Active != "work" {
			t.Fatal("per-service account selection lost")
		}
	}

}
