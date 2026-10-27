package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/lucasdevco/tokenhub/internal/database"
	"gopkg.in/yaml.v3"
)

const (
	StateStopped  = "stopped"
	StateStarting = "starting"
	StateRunning  = "running"
	StateStopping = "stopping"
	StateError    = "error"
)

const DefaultConfig = `server:
  host: "127.0.0.1"
  port: 5601
logging:
  enabled: true
  console: false
  log_dir: "logs"
http_client:
  proxy_from_environment: false
webui:
  host: "127.0.0.1"
  port: 5602
providers: {}
active_provider: ""
`

type Status struct {
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
	Pending        bool   `json:"pending"`
	Address        string `json:"address"`
	ConfigFile     string `json:"config_file"`
	ActiveProvider string `json:"active_provider"`
}

type Manager struct {
	Filename       string
	op             sync.Mutex
	mu             sync.Mutex
	run            *Runtime
	state          string
	lastError      string
	activeHash     [32]byte
	address        string
	logDir         string
	activeProvider string
}

func NewManager(filename string) (*Manager, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(absolute); os.IsNotExist(err) {
		if err := os.WriteFile(absolute, []byte(DefaultConfig), 0600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return &Manager{Filename: absolute, state: StateStopped}, nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := os.ReadFile(m.Filename)
	active := m.activeProvider
	if m.run == nil {
		var selection struct {
			ActiveProvider string `yaml:"active_provider"`
		}
		_ = yaml.Unmarshal(data, &selection)
		active = selection.ActiveProvider
	}
	return Status{
		State:          m.state,
		Error:          m.lastError,
		Pending:        m.state == StateRunning && sha256.Sum256(data) != m.activeHash,
		Address:        m.address,
		ConfigFile:     m.Filename,
		ActiveProvider: active,
	}
}

func (m *Manager) setState(state string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
	m.lastError = ""
	if err != nil {
		m.lastError = err.Error()
		log.Printf("Proxy %s: %v", state, err)
	}
}

func (m *Manager) Start() error { m.op.Lock(); defer m.op.Unlock(); return m.start() }
func (m *Manager) start() error {
	m.mu.Lock()
	running := m.run != nil
	m.mu.Unlock()
	if running {
		return nil
	}
	m.setState(StateStarting, nil)
	data, err := os.ReadFile(m.Filename)
	if err != nil {
		m.setState(StateError, err)
		return err
	}
	cfg, err := ParseConfig(data, m.Filename)
	if err != nil {
		m.setState(StateError, err)
		return err
	}
	run, err := Start(context.Background(), cfg)
	if err != nil {
		m.setState(StateError, err)
		return err
	}
	m.mu.Lock()
	m.run = run
	m.logDir = cfg.Logging.LogDir
	m.activeHash = sha256.Sum256(data)
	m.activeProvider = cfg.ActiveProvider
	m.state = StateRunning
	if cfg.Server != nil {
		m.address = proxyURL(cfg.Server.Host, cfg.Server.Port)
	}
	m.mu.Unlock()
	go func() {
		select {
		case err := <-run.Errors:
			m.op.Lock()
			m.mu.Lock()
			current := m.run == run
			m.mu.Unlock()
			if current {
				_ = m.stop()
				m.setState(StateError, err)
			}
			m.op.Unlock()
		case <-run.done:
		}
	}()
	return nil
}

func (m *Manager) Stop() error { m.op.Lock(); defer m.op.Unlock(); return m.stop() }
func (m *Manager) stop() error {
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	if run == nil {
		m.setState(StateStopped, nil)
		return nil
	}
	m.setState(StateStopping, nil)
	err := run.Stop()
	m.mu.Lock()
	m.run = nil
	m.address = ""
	m.mu.Unlock()
	m.setState(StateStopped, err)
	return err
}

func (m *Manager) Restart() error {
	m.op.Lock()
	defer m.op.Unlock()
	if err := m.stop(); err != nil {
		return err
	}
	return m.start()
}

func (m *Manager) Validate(data []byte) error { _, err := ParseConfig(data, m.Filename); return err }
func (m *Manager) Save(data []byte) error {
	m.op.Lock()
	defer m.op.Unlock()
	if err := m.Validate(data); err != nil {
		return err
	}
	return AtomicWrite(m.Filename, data)
}

// AtomicWrite keeps the last valid file intact until the replacement is flushed.
func AtomicWrite(filename string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(filename), ".tokenhub-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, filename)
}

// LogDirectory follows the last successful proxy run, including after stopping.
// A saved new directory becomes active when the next proxy run starts.
func (m *Manager) LogDirectory() (string, error) {
	m.mu.Lock()
	dir := m.logDir
	m.mu.Unlock()
	if dir != "" {
		return dir, nil
	}
	data, err := os.ReadFile(m.Filename)
	if err != nil {
		return "", err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	dir = cfg.Logging.LogDir
	if dir == "" {
		dir = "logs"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(m.Filename), dir)
	}
	return dir, nil
}

// proxyURL returns a connectable URL, rather than a wildcard bind address.
func proxyURL(host string, port int) string {
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// SelectProvider persists and applies only selection; other edits still require restart.
func (m *Manager) SelectProvider(name, expectedVersion string) error {
	m.op.Lock()
	defer m.op.Unlock()
	data, err := os.ReadFile(m.Filename)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != expectedVersion {
		return fmt.Errorf("配置已被修改，请重新加载后再切换")
	}
	m.mu.Lock()
	run, activeHash := m.run, m.activeHash
	m.mu.Unlock()
	if run != nil && sha256.Sum256(data) != activeHash {
		return fmt.Errorf("配置待应用，请先重启代理再切换 Provider")
	}
	if err := m.Validate(data); err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	root := document.Content[0]
	found := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "active_provider" {
			root.Content[i+1].SetString(name)
			found = true
			break
		}
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "active_provider"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	}
	updated, err := yaml.Marshal(&document)
	if err != nil {
		return err
	}
	if err := m.Validate(updated); err != nil {
		return err
	}
	if err := AtomicWrite(m.Filename, updated); err != nil {
		return err
	}
	if run != nil {
		if err := run.proxy.SelectProvider(name); err != nil {
			if restoreErr := AtomicWrite(m.Filename, data); restoreErr != nil {
				return fmt.Errorf("%v; failed to restore configuration: %w", err, restoreErr)
			}
			return err
		}
	}
	m.mu.Lock()
	m.activeProvider = name
	if run != nil {
		m.activeHash = sha256.Sum256(updated)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) DatabasePath() string { return database.Path(filepath.Dir(m.Filename)) }
