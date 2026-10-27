package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasdevco/tokenhub/internal/database"
)

func TestDatabasePathUsesConfigDirectory(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "config.yaml")
	for _, logDir := range []string{"logs", filepath.Join(t.TempDir(), "external-logs")} {
		config, err := ParseConfig([]byte("server:\n  port: 5601\nlogging:\n  log_dir: '"+logDir+"'\nhttp_client:\n  proxy_from_environment: false\n"), filename)
		if err != nil {
			t.Fatal(err)
		}
		if config.DatabasePath != database.Path(root) {
			t.Fatalf("database follows logs instead of config: %s", config.DatabasePath)
		}
	}
	manager, err := NewManager(filename)
	if err != nil {
		t.Fatal(err)
	}
	if manager.DatabasePath() != database.Path(root) {
		t.Fatal("manager and capture use different database paths")
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestWebUIConfiguration(t *testing.T) {
	for _, host := range []string{"", "127.0.0.1", "localhost", "::1", "0.0.0.0", "::", "external.test"} {
		t.Run(host, func(t *testing.T) {
			filename := writeTestConfig(t, "server:\n  port: 5601\nwebui:\n  host: '"+host+"'\n  port: 5602\n")
			config, err := loadConfig(filename)
			if host == "0.0.0.0" || host == "::" || host == "external.test" {
				if err == nil {
					t.Fatal("nonloopback web UI accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.WebUI == nil || config.WebUI.Port != 5602 {
				t.Fatal("web UI config missing")
			}
		})
	}
}

func TestWebUIPortValidation(t *testing.T) {
	for _, port := range []string{"-1", "0", "1", "65535", "65536"} {
		t.Run(port, func(t *testing.T) {
			_, err := ParseConfig([]byte("server: {}\nwebui:\n  port: "+port+"\n"), filepath.Join(t.TempDir(), "config.yaml"))
			invalid := port == "-1" || port == "65536"
			if (err != nil) != invalid {
				t.Fatalf("port %s: %v", port, err)
			}
		})
	}
}

func TestLoadConfigAppliesServerDefaultsWhenServerPresent(t *testing.T) {
	config, err := loadConfig(writeTestConfig(t, `
server: {}
logging:
  enabled: false
`))
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if config.Server == nil {
		t.Fatal("expected server config")
	}
	if config.Server.Host != "localhost" || config.Server.Port != 5601 {
		t.Fatalf("unexpected default server address %s:%d", config.Server.Host, config.Server.Port)
	}
}

func TestLoadConfigAppliesProviderAuthorizationDefaults(t *testing.T) {
	config, err := loadConfig(writeTestConfig(t, `
server: {}
providers:
  api:
    base_url: "https://example.com/"
    authorization:
      backend_key: "literal-secret"
`))
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	authorization := config.Providers["api"].Authorization
	if authorization == nil {
		t.Fatal("expected provider authorization")
	}
	if authorization.Header != "Authorization" || authorization.Template != "Bearer {}" {
		t.Fatalf("unexpected authorization defaults: %#v", authorization)
	}
	if got := renderAuthorizationValue(authorization.Template, authorization.BackendKey); got != "Bearer literal-secret" {
		t.Fatalf("rendered backend value = %q", got)
	}
}

func TestLoadConfigAllowsClientOnlyAuthorization(t *testing.T) {
	config, err := loadConfig(writeTestConfig(t, `
server: {}
providers:
  api:
    base_url: "https://example.com/"
    authorization:
      client_key: "local-secret"
      template: "Bearer {}"
`))
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	authorization := config.Providers["api"].Authorization
	if authorization == nil || authorization.BackendKey != "" || authorization.ClientKey != "local-secret" {
		t.Fatalf("unexpected client-only authorization: %#v", authorization)
	}
}

func TestLoadConfigRequiresTemplateForCustomAuthorizationHeader(t *testing.T) {
	_, err := loadConfig(writeTestConfig(t, `
server: {}
providers:
  api:
    base_url: "https://example.com/"
    authorization:
      header: "X-API-Key"
      backend_key: "backend-secret"
`))
	if err == nil {
		t.Fatal("expected a custom authorization header without a template to fail")
	}
	if !strings.Contains(err.Error(), `template is required for header "X-API-Key"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsEmptyAuthorization(t *testing.T) {
	_, err := loadConfig(writeTestConfig(t, `
server: {}
providers:
  api:
    base_url: "https://example.com/"
    authorization: {}
`))
	if err == nil {
		t.Fatal("expected empty authorization to fail")
	}
	if !strings.Contains(err.Error(), "backend_key, client_key, or both") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigIgnoresEnvironmentFilesAndRejectsCredentialReferences(t *testing.T) {
	t.Setenv("TOKENHUB_TEST_KEY", "process-secret")
	path := writeTestConfig(t, "server: {}\nproviders:\n  api:\n    base_url: https://example.com\n    active_account: personal\n    accounts: [{id: personal, name: Personal, key: literal-secret}]\n")
	// Malformed env files are never opened, even when present beside the config.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("TOKENHUB_TEST_KEY=\"unterminated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env-profiles.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["api"].Authorization.BackendKey != "literal-secret" {
		t.Fatal("credential did not come from configuration")
	}
	for _, fields := range []string{
		"authorization: {backend_key: '${TOKENHUB_TEST_KEY}'}",
		"authorization: {client_key: '${TOKENHUB_TEST_KEY}'}",
		"active_account: personal\n    accounts: [{id: personal, name: Personal, key: '${TOKENHUB_TEST_KEY}'}]",
	} {
		_, err := ParseConfig([]byte("server: {}\nproviders:\n  api:\n    base_url: https://example.com\n    "+fields+"\n"), path)
		if err == nil || !strings.Contains(err.Error(), "不再支持环境变量引用") {
			t.Fatalf("environment reference accepted: %v", err)
		}
	}
}

func TestLoadConfigValidatesAuthorizationTemplate(t *testing.T) {
	for _, template := range []string{"Bearer key", "{} {}"} {
		_, err := loadConfig(writeTestConfig(t, `
server: {}
providers:
  api:
    base_url: "https://example.com/"
    authorization:
      backend_key: "backend-secret"
      template: "`+template+`"
`))
		if err == nil {
			t.Fatalf("expected template %q to fail", template)
		}
		if !strings.Contains(err.Error(), "exactly one {}") {
			t.Fatalf("unexpected error for template %q: %v", template, err)
		}
	}
}

func TestLoadConfigRejectsProvidersWithoutServer(t *testing.T) {
	_, err := loadConfig(writeTestConfig(t, `
logging:
  enabled: false
providers:
  api:
    base_url: "https://example.com/"
`))
	if err == nil {
		t.Fatal("expected providers without server to fail")
	}
	if !strings.Contains(err.Error(), "configuration must include a server section") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsConfigWithoutListeners(t *testing.T) {
	_, err := loadConfig(writeTestConfig(t, `
logging:
  enabled: false
`))
	if err == nil {
		t.Fatal("expected config without server to fail")
	}
	if !strings.Contains(err.Error(), "include a server section") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsRemovedProxySection(t *testing.T) {
	for _, content := range []string{"proxy: {}\n", "server: {}\nproxy: {}\n"} {
		_, err := loadConfig(writeTestConfig(t, content))
		if err == nil || !strings.Contains(err.Error(), "proxy section is no longer supported") {
			t.Fatalf("expected removed proxy section error, got %v", err)
		}
	}
}

func TestProviderConfigurationRejectsLegacyAndInvalidSelection(t *testing.T) {
	for _, source := range []string{
		"server: {}\nroutes: {}\n",
		"server: {not_found: /404/}\n",
		"server: {}\nactive_provider: missing\nproviders: {}\n",
		"server: {}\nproviders:\n  test: {base_url: relative}\n",
	} {
		if _, err := ParseConfig([]byte(source), filepath.Join(t.TempDir(), "config.yaml")); err == nil {
			t.Fatalf("invalid config accepted: %s", source)
		}
	}
}
