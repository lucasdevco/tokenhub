package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrProviderConflict = errors.New("服务商配置已被修改，请重新加载后再操作")

type ProviderView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Kind         string            `json:"kind"`
	Accounts     []ProviderAccount `json:"accounts"`
	Active       string            `json:"active"`
	Header       string            `json:"header"`
	Template     string            `json:"template"`
	Logging      bool              `json:"logging"`
	ProxyEnabled bool              `json:"proxy_enabled"`
}

type ProviderCatalog struct {
	Services []ProviderView `json:"services"`
	Active   string         `json:"active"`
	Version  string         `json:"version"`
	Status   Status         `json:"status"`
}

type ProviderChange struct {
	Action   string       `json:"action"`
	Service  ProviderView `json:"service"`
	Provider string       `json:"provider"`
	Account  string       `json:"account"`
	Version  string       `json:"version"`
	New      bool         `json:"new"`
	Activate bool         `json:"activate"`
}

func (m *Manager) ReadProviders() (ProviderCatalog, error) {
	m.op.Lock()
	defer m.op.Unlock()
	_, catalog, err := m.readProviders()
	return catalog, err
}

func (m *Manager) readProviders() ([]byte, ProviderCatalog, error) {
	catalog := ProviderCatalog{Services: []ProviderView{}}
	data, err := os.ReadFile(m.Filename)
	if err != nil {
		return nil, catalog, err
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, catalog, err
	}
	catalog.Version = fmt.Sprintf("%x", sha256.Sum256(data))
	catalog.Active = raw.ActiveProvider
	catalog.Status = m.Status()
	for id, provider := range raw.Providers {
		view := ProviderView{ID: id, Name: provider.Name, Kind: provider.Kind, Address: provider.BaseURL, Accounts: provider.Accounts, Active: provider.ActiveAccount, Header: "Authorization", Template: "Bearer {}", Logging: raw.Logging.Enabled, ProxyEnabled: provider.ProxyEnabled == nil || *provider.ProxyEnabled}
		if view.Name == "" {
			view.Name = id
		}
		if provider.Logging != nil {
			view.Logging = *provider.Logging
		}
		if auth := provider.Authorization; auth != nil {
			if auth.Header != "" {
				view.Header = auth.Header
			}
			if auth.Template != "" {
				view.Template = auth.Template
			}
			if len(view.Accounts) == 0 && auth.BackendKey != "" {
				view.Accounts = []ProviderAccount{{ID: "default", Name: "默认账号", Key: auth.BackendKey}}
				view.Active = "default"
			}
		}
		if view.Accounts == nil {
			view.Accounts = []ProviderAccount{}
		}
		catalog.Services = append(catalog.Services, view)
	}
	sort.Slice(catalog.Services, func(i, j int) bool {
		if catalog.Services[i].Name == catalog.Services[j].Name {
			return catalog.Services[i].ID < catalog.Services[j].ID
		}
		return catalog.Services[i].Name < catalog.Services[j].Name
	})
	return data, catalog, nil
}

// ChangeProvider edits only the requested service and applies its new immutable
// targets after the config is safely persisted. Other pending settings are never applied.
func (m *Manager) ChangeProvider(change ProviderChange) (ProviderCatalog, error) {
	m.op.Lock()
	defer m.op.Unlock()
	data, catalog, err := m.readProviders()
	if err != nil {
		return catalog, err
	}
	if change.Version != catalog.Version {
		return catalog, ErrProviderConflict
	}
	if catalog.Status.Pending {
		return catalog, fmt.Errorf("其他配置待应用，请先重启代理后再操作服务商")
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return catalog, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return catalog, fmt.Errorf("配置无效，请检查高级设置")
	}
	root := document.Content[0]
	providers := mappingValue(root, "providers")
	if providers == nil || providers.Tag == "!!null" {
		providers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMappingNode(root, "providers", providers)
	}
	if providers.Kind != yaml.MappingNode {
		return catalog, fmt.Errorf("服务商配置格式无效")
	}
	find := func(id string) *ProviderView {
		for i := range catalog.Services {
			if catalog.Services[i].ID == id {
				return &catalog.Services[i]
			}
		}
		return nil
	}
	switch change.Action {
	case "save", "account":
		view := change.Service
		if change.New && view.ID != strings.TrimSpace(view.ID) {
			return catalog, fmt.Errorf("服务商编号不能以空格开头或结尾")
		}
		existing := find(view.ID)
		if change.Action == "account" {
			existing = find(change.Provider)
			if existing == nil {
				return catalog, fmt.Errorf("服务商不存在，请重新加载")
			}
			view = *existing
			view.Active = change.Account
			if change.Account == "" || !hasAccount(view.Accounts, change.Account) {
				return catalog, fmt.Errorf("账号不存在，请重新加载")
			}
		} else {
			if (change.New && existing != nil) || (!change.New && existing == nil) {
				return catalog, fmt.Errorf("服务商已被添加或删除，请重新加载")
			}
		}
		view.Name = strings.TrimSpace(view.Name)
		if strings.TrimSpace(view.ID) == "" || view.Name == "" || strings.TrimSpace(view.Address) == "" {
			return catalog, fmt.Errorf("请填写服务商名称和服务地址")
		}
		for _, other := range catalog.Services {
			if other.ID != view.ID && other.Name == view.Name {
				return catalog, fmt.Errorf("服务商名称已存在")
			}
		}
		for i := range view.Accounts {
			view.Accounts[i].Name = strings.TrimSpace(view.Accounts[i].Name)
		}
		node := mappingValue(providers, view.ID)
		if node == nil {
			node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingNode(providers, view.ID, node)
		}
		if node.Kind != yaml.MappingNode {
			return catalog, fmt.Errorf("服务商配置格式无效")
		}
		if err := writeProvider(node, view); err != nil {
			return catalog, err
		}
		if change.Action == "save" && change.Activate {
			setMappingValue(root, "active_provider", view.ID)
		}
	case "select":
		if change.Provider != "" && find(change.Provider) == nil {
			return catalog, fmt.Errorf("服务商不存在，请重新加载")
		}
		setMappingValue(root, "active_provider", change.Provider)
	case "delete":
		if find(change.Provider) == nil {
			return catalog, fmt.Errorf("服务商不存在，请重新加载")
		}
		deleteMappingValue(providers, change.Provider)
		if catalog.Active == change.Provider {
			setMappingValue(root, "active_provider", "")
		}
	default:
		return catalog, fmt.Errorf("不支持的服务商操作")
	}
	updated, err := yaml.Marshal(&document)
	if err != nil {
		return catalog, err
	}
	cfg, err := ParseConfig(updated, m.Filename)
	if err != nil {
		return catalog, err
	}
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	// A first provider may enable logging on a run that had no recorder.
	var created bool
	if run != nil && run.recorder == nil {
		run.recorder, err = buildRecorder(cfg)
		if err != nil {
			return catalog, err
		}
		created = run.recorder != nil
	}
	rollbackRecorder := func() {
		if created {
			_ = run.recorder.Shutdown(context.Background())
			run.recorder = nil
		}
	}
	if err := AtomicWrite(m.Filename, updated); err != nil {
		rollbackRecorder()
		return catalog, err
	}
	if run != nil {
		if err := run.proxy.ReplaceProviders(providerDefinitions(cfg, run.recorder), cfg.ActiveProvider); err != nil {
			rollbackRecorder()
			if restoreErr := AtomicWrite(m.Filename, data); restoreErr != nil {
				return catalog, fmt.Errorf("%v；恢复配置失败：%w", err, restoreErr)
			}
			return catalog, err
		}
	}
	m.mu.Lock()
	m.activeProvider = cfg.ActiveProvider
	if run != nil {
		m.activeHash = sha256.Sum256(updated)
	}
	m.mu.Unlock()
	_, result, err := m.readProviders()
	return result, err
}

func hasAccount(accounts []ProviderAccount, id string) bool {
	for _, account := range accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
func setMappingNode(node *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			old := node.Content[i+1]
			value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}
func setMappingValue(node *yaml.Node, key string, value any) {
	var encoded yaml.Node
	_ = encoded.Encode(value)
	setMappingNode(node, key, &encoded)
}
func deleteMappingValue(node *yaml.Node, key string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}
func writeProvider(node *yaml.Node, view ProviderView) error {
	setMappingValue(node, "name", view.Name)
	setMappingValue(node, "kind", view.Kind)
	setMappingValue(node, "base_url", strings.TrimSpace(view.Address))
	setMappingValue(node, "logging", view.Logging)
	setMappingValue(node, "proxy_enabled", view.ProxyEnabled)
	accounts := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	old := mappingValue(node, "accounts")
	for _, account := range view.Accounts {
		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if old != nil && old.Kind == yaml.SequenceNode {
			for _, candidate := range old.Content {
				if id := mappingValue(candidate, "id"); id != nil && id.Value == account.ID {
					item = candidate
					break
				}
			}
		}
		setMappingValue(item, "id", account.ID)
		setMappingValue(item, "name", account.Name)
		setMappingValue(item, "key", account.Key)
		accounts.Content = append(accounts.Content, item)
	}
	setMappingNode(node, "accounts", accounts)
	setMappingValue(node, "active_account", view.Active)
	auth := mappingValue(node, "authorization")
	if auth == nil || auth.Tag == "!!null" {
		auth = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if auth.Kind != yaml.MappingNode {
		return fmt.Errorf("鉴权配置格式无效")
	}
	deleteMappingValue(auth, "backend_key")
	if len(view.Accounts) > 0 || mappingValue(auth, "client_key") != nil {
		setMappingValue(auth, "header", view.Header)
		setMappingValue(auth, "template", view.Template)
		setMappingNode(node, "authorization", auth)
	} else {
		// With no credentials left, omit the auth block entirely.
		deleteMappingValue(node, "authorization")
	}
	return nil
}
