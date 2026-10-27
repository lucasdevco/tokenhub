package service

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasdevco/tokenhub/internal/database"
	"golang.org/x/net/http/httpguts"
	"gopkg.in/yaml.v3"
)

// ProviderAccount is a named upstream credential.
type ProviderAccount struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	Key  string `yaml:"key" json:"key"`
}

// Provider defines an upstream service. Incoming protocol paths are preserved.
type Provider struct {
	Name          string               `yaml:"name,omitempty"`
	Kind          string               `yaml:"kind,omitempty"`
	Accounts      []ProviderAccount    `yaml:"accounts,omitempty"`
	ActiveAccount string               `yaml:"active_account,omitempty"`
	BaseURL       string               `yaml:"base_url"`
	Logging       *bool                `yaml:"logging"`
	ProxyEnabled  *bool                `yaml:"proxy_enabled,omitempty"`
	Authorization *AuthorizationConfig `yaml:"authorization"`
}

// AuthorizationConfig injects a backend credential and optionally requires a
// different credential from clients of the local proxy.
type AuthorizationConfig struct {
	Header     string `yaml:"header"`
	BackendKey string `yaml:"backend_key"`
	Template   string `yaml:"template"`
	ClientKey  string `yaml:"client_key"`
}

type HTTPClientConfig struct {
	ProxyURL             string `yaml:"proxy_url"`
	ProxyFromEnvironment *bool  `yaml:"proxy_from_environment"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type Config struct {
	DatabasePath string        `yaml:"-"`
	WebUI        *WebUIConfig  `yaml:"webui"`
	Server       *ServerConfig `yaml:"server"`
	Logging      struct {
		Enabled bool   `yaml:"enabled"`
		Console bool   `yaml:"console"`
		LogDir  string `yaml:"log_dir"`
	} `yaml:"logging"`
	HTTPClient     HTTPClientConfig    `yaml:"http_client"`
	Providers      map[string]Provider `yaml:"providers"`
	ActiveProvider string              `yaml:"active_provider"`
}

type WebUIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func renderAuthorizationValue(template, key string) string {
	return strings.Replace(template, "{}", key, 1)
}

func validateProviderAuthorization(providerName string, authorization *AuthorizationConfig) error {
	if authorization == nil {
		return nil
	}

	authorization.Header = strings.TrimSpace(authorization.Header)
	if authorization.Header == "" {
		authorization.Header = "Authorization"
	}
	if !httpguts.ValidHeaderFieldName(authorization.Header) {
		return fmt.Errorf("provider %s authorization header %q is invalid", providerName, authorization.Header)
	}
	if authorization.Template == "" {
		if strings.EqualFold(authorization.Header, "Authorization") {
			authorization.Template = "Bearer {}"
		} else {
			return fmt.Errorf("provider %s authorization template is required for header %q", providerName, authorization.Header)
		}
	}
	if strings.Count(authorization.Template, "{}") != 1 {
		return fmt.Errorf("provider %s authorization template must contain exactly one {} placeholder", providerName)
	}

	if authorization.BackendKey == "" && authorization.ClientKey == "" {
		return fmt.Errorf("provider %s authorization requires backend_key, client_key, or both", providerName)
	}

	for _, key := range []string{authorization.BackendKey, authorization.ClientKey} {
		if strings.HasPrefix(key, "${") && strings.HasSuffix(key, "}") {
			return fmt.Errorf("provider %s: 不再支持环境变量引用，请直接填写密钥", providerName)
		}
	}
	if authorization.BackendKey != "" {
		if !httpguts.ValidHeaderFieldValue(renderAuthorizationValue(authorization.Template, authorization.BackendKey)) {
			return fmt.Errorf("provider %s authorization backend value is not a valid HTTP header value", providerName)
		}
	}

	if authorization.ClientKey != "" {
		if !httpguts.ValidHeaderFieldValue(renderAuthorizationValue(authorization.Template, authorization.ClientKey)) {
			return fmt.Errorf("provider %s authorization client value is not a valid HTTP header value", providerName)
		}
	}
	return nil
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ParseConfig(data, filename)
}

// ParseConfig validates configuration without reading external credential sources or writing files.
func ParseConfig(data []byte, filename string) (*Config, error) {
	var sections map[string]yaml.Node
	if err := yaml.Unmarshal(data, &sections); err != nil {
		return nil, err
	}
	if _, exists := sections["proxy"]; exists {
		return nil, fmt.Errorf("proxy section is no longer supported; remove it and use server with providers")
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if _, exists := sections["routes"]; exists {
		return nil, fmt.Errorf("routes is no longer supported; use providers with base_url and active_provider; remove provider prefixes from client URLs")
	}
	if server, exists := sections["server"]; exists {
		var fields map[string]yaml.Node
		if err := server.Decode(&fields); err != nil {
			return nil, err
		}
		if _, exists := fields["not_found"]; exists {
			return nil, fmt.Errorf("server.not_found is no longer supported; requests without an active provider return 503")
		}
	}
	for providerName, provider := range config.Providers {
		if len(provider.Accounts) > 0 {
			ids, names := map[string]bool{}, map[string]bool{}
			selected := false
			common := provider.Authorization
			for _, account := range provider.Accounts {
				if strings.TrimSpace(account.ID) == "" || strings.TrimSpace(account.Name) == "" || ids[account.ID] || names[account.Name] || strings.TrimSpace(account.Key) == "" {
					return nil, fmt.Errorf("provider %s: 账号需要唯一编号、名称和非空密钥", providerName)
				}
				ids[account.ID], names[account.Name] = true, true
				auth := AuthorizationConfig{}
				if common != nil {
					auth = *common
				}
				auth.BackendKey = account.Key
				if err := validateProviderAuthorization(providerName, &auth); err != nil {
					return nil, err
				}
				if account.ID == provider.ActiveAccount {
					provider.Authorization = &auth
					selected = true
				}
			}
			if !selected {
				return nil, fmt.Errorf("provider %s: 请选择有效的账号", providerName)
			}
		} else if provider.ActiveAccount != "" {
			return nil, fmt.Errorf("provider %s: 当前账号不存在", providerName)
		}
		if len(provider.Accounts) == 0 {
			if err := validateProviderAuthorization(providerName, provider.Authorization); err != nil {
				return nil, err
			}
		}
		config.Providers[providerName] = provider
	}

	if config.Server == nil {
		return nil, fmt.Errorf("configuration must include a server section")
	}
	if config.Server != nil {
		if config.Server.Host == "" {
			config.Server.Host = "localhost"
		}
		if config.Server.Port == 0 {
			config.Server.Port = 5601
		}
	}
	if config.WebUI != nil {
		host, port := config.WebUI.Host, config.WebUI.Port
		ip := net.ParseIP(host)
		if (host != "" && host != "localhost" && (ip == nil || !ip.IsLoopback())) || port < 0 || port > 65535 {
			return nil, fmt.Errorf("webui requires a loopback host and a port between 1 and 65535")
		}
	}

	if config.Server != nil && (config.Server.Port < 1 || config.Server.Port > 65535) {
		return nil, fmt.Errorf("server.port must be between 1 and 65535")
	}
	client := buildHTTPClientProxyConfig(&config)
	endpoints, _, err := describeHTTPClientProxyConfig(client)
	if err != nil {
		return nil, err
	}
	if err := validateHTTPClientProxyEndpoints(endpoints, configuredListenerAddresses(&config)); err != nil {
		return nil, err
	}
	if config.Server != nil {
		if _, err := buildReverseProxy(&config, nil, client); err != nil {
			return nil, err
		}
	}
	base, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	config.DatabasePath = database.Path(base)
	if config.Logging.LogDir == "" {
		config.Logging.LogDir = "logs"
	}
	if !filepath.IsAbs(config.Logging.LogDir) {
		config.Logging.LogDir = filepath.Join(base, config.Logging.LogDir)
	}
	return &config, nil
}

// LoadConfig reads and validates a configuration file.
func LoadConfig(filename string) (*Config, error) { return loadConfig(filename) }
