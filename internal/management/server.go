package management

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucasdevco/tokenhub/internal/i18n"
	"github.com/lucasdevco/tokenhub/internal/service"
	"github.com/lucasdevco/tokenhub/web"
)

type Preferences struct {
	Locale      string `json:"locale,omitempty"`
	AutoStart   bool   `json:"auto_start"`
	OpenAtLogin bool   `json:"open_at_login"`
}

type Options struct {
	Address        string
	SetOpenAtLogin func(bool) error
	GetOpenAtLogin func() bool
	OpenDirectory  func(string) error
}

type Server struct {
	Manager   *service.Manager
	URL       string
	listener  net.Listener
	http      *http.Server
	session   string
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	logs      http.Handler
	logDir    string
	logCancel context.CancelFunc
	options   Options
}

func New(manager *service.Manager, options Options) (*Server, error) {
	if options.Address == "" {
		options.Address = "127.0.0.1:5602"
	}
	host, _, err := net.SplitHostPort(options.Address)
	if err != nil {
		return nil, err
	}
	if host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("management server must listen on loopback")
	}
	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return nil, fmt.Errorf("management listener (another tokenhub may already be running): %w", err)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		listener.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		Manager:  manager,
		listener: listener,
		URL:      "http://" + listener.Addr().String(),
		session:  hex.EncodeToString(random),
		ctx:      ctx,
		cancel:   cancel,
		options:  options,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]string{"token": s.session})
	})
	mux.HandleFunc("GET /api/capabilities", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]bool{"settings": true, "finder": s.options.OpenDirectory != nil})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		send(w, manager.Status())
	})
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("POST /api/config/validate", s.validate)
	mux.HandleFunc("POST /api/config", s.save)
	mux.HandleFunc("POST /api/provider", s.selectProvider)
	mux.HandleFunc("GET /api/providers", s.providers)
	mux.HandleFunc("POST /api/providers", s.providers)
	mux.HandleFunc("POST /api/service/{action}", s.control)
	mux.HandleFunc("GET /api/preferences", s.getPreferences)
	mux.HandleFunc("POST /api/preferences", s.savePreferences)
	mux.HandleFunc("POST /api/directories/{kind}/open", s.openDirectory)
	mux.HandleFunc("GET /api/", s.viewLogs)
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("GET /logs/", web.Handler())
	mux.Handle("/", web.Handler())
	s.http = &http.Server{Handler: s.secure(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return s, nil
}

func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Close() error {
	s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := s.http.Shutdown(ctx)
	_ = s.http.Close()
	return err
}

func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != s.listener.Addr().String() {
			http.Error(w, "Invalid management host", 403)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != s.URL {
			http.Error(w, "Cross-origin request denied", 403)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			http.Error(w, "Cross-site request denied", 403)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin != s.URL || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Tokenhub-Session")), []byte(s.session)) != 1 {
				http.Error(w, "Invalid management session", 403)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "JSON required", 415)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func send(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	send(w, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(w, 400, err)
		return false
	}
	return true
}

func version(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.Manager.Filename)
	if err != nil {
		fail(w, 500, err)
		return
	}
	send(w, map[string]string{"yaml": string(data), "version": version(data), "filename": s.Manager.Filename})
}

type configInput struct {
	YAML    string `json:"yaml"`
	Version string `json:"version"`
}

func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	var input configInput
	if !decode(w, r, &input) {
		return
	}
	if err := s.Manager.Validate([]byte(input.YAML)); err != nil {
		fail(w, 400, err)
		return
	}
	send(w, map[string]bool{"valid": true})
}

func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	var input configInput
	if !decode(w, r, &input) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.Manager.Filename)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if version(data) != input.Version {
		fail(w, 409, fmt.Errorf("配置已被修改，请重新加载后再保存"))
		return
	}
	if err := s.Manager.Save([]byte(input.YAML)); err != nil {
		fail(w, 400, err)
		return
	}
	send(w, map[string]string{"version": version([]byte(input.YAML))})
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.PathValue("action") {
	case "start":
		err = s.Manager.Start()
	case "stop":
		err = s.Manager.Stop()
	case "restart":
		err = s.Manager.Restart()
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, 400, err)
		return
	}
	send(w, s.Manager.Status())
}

func (s *Server) preferencesFile() string {
	return filepath.Join(filepath.Dir(s.Manager.Filename), "preferences.json")
}

func (s *Server) Preferences() (Preferences, error) {
	var prefs Preferences
	data, err := os.ReadFile(s.preferencesFile())
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	err = json.Unmarshal(data, &prefs)
	return prefs, err
}

func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := s.Preferences()
	if err != nil {
		fail(w, 500, err)
		return
	}
	if s.options.GetOpenAtLogin != nil {
		prefs.OpenAtLogin = s.options.GetOpenAtLogin()
	}
	send(w, map[string]any{"preferences": prefs, "login_supported": s.options.SetOpenAtLogin != nil})
}

func (s *Server) savePreferences(w http.ResponseWriter, r *http.Request) {
	var prefs Preferences
	if !decode(w, r, &prefs) {
		return
	}
	if prefs.Locale != "" && !i18n.Supported(prefs.Locale) {
		fail(w, 400, fmt.Errorf("unsupported locale: %s", prefs.Locale))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.Preferences()
	if err != nil {
		fail(w, 500, err)
		return
	}
	if s.options.SetOpenAtLogin == nil && prefs.OpenAtLogin {
		fail(w, 400, fmt.Errorf("登录启动需要 macOS 客户端"))
		return
	}
	if s.options.GetOpenAtLogin != nil {
		old.OpenAtLogin = s.options.GetOpenAtLogin()
	}
	loginChanged := s.options.SetOpenAtLogin != nil && prefs.OpenAtLogin != old.OpenAtLogin
	if loginChanged {
		if err := s.options.SetOpenAtLogin(prefs.OpenAtLogin); err != nil {
			fail(w, 400, err)
			return
		}
	}
	data, _ := json.MarshalIndent(prefs, "", "  ")
	if err := service.AtomicWrite(s.preferencesFile(), data); err != nil {
		if loginChanged {
			_ = s.options.SetOpenAtLogin(old.OpenAtLogin)
		}
		fail(w, 500, err)
		return
	}
	send(w, prefs)
}

func (s *Server) viewLogs(w http.ResponseWriter, r *http.Request) {
	dir, err := s.Manager.LogDirectory()
	if err != nil {
		fail(w, 500, err)
		return
	}
	s.mu.Lock()
	if s.logs == nil || dir != s.logDir {
		if err := os.MkdirAll(dir, 0700); err != nil {
			s.mu.Unlock()
			fail(w, 500, err)
			return
		}
		store, err := NewStore(dir, s.Manager.DatabasePath())
		if err != nil {
			s.mu.Unlock()
			fail(w, 500, err)
			return
		}
		if s.logCancel != nil {
			s.logCancel()
		}
		ctx, cancel := context.WithCancel(s.ctx)
		s.logCancel = cancel
		s.logDir = dir
		s.logs = NewLogHandler(store)
		go store.Run(ctx)
	}
	handler := s.logs
	s.mu.Unlock()
	handler.ServeHTTP(w, r)
}

// openDirectory accepts a fixed directory kind, never a caller-supplied path.
func (s *Server) openDirectory(w http.ResponseWriter, r *http.Request) {
	if s.options.OpenDirectory == nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("Open in Finder 需要使用 macOS 桌面客户端"))
		return
	}
	var directory string
	switch r.PathValue("kind") {
	case "config":
		directory = filepath.Dir(s.Manager.Filename)
	case "logs":
		var err error
		directory, err = s.Manager.LogDirectory()
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err := s.options.OpenDirectory(directory); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]bool{"opened": true})
}

func (s *Server) selectProvider(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider string `json:"provider"`
		Version  string `json:"version"`
	}
	if !decode(w, r, &input) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Manager.SelectProvider(input.Provider, input.Version); err != nil {
		fail(w, 400, err)
		return
	}
	s.config(w, r)
}
