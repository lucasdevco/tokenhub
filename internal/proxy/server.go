package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	"golang.org/x/net/http/httpguts"
)

type ProviderAuthorization struct{ Header, BackendValue, ClientValue string }
type ProviderOptions struct {
	Authorization *ProviderAuthorization
	// ProxyEnabled defaults to true; false bypasses all upstream proxy settings.
	ProxyEnabled *bool
}

type providerTarget struct {
	name          string
	recorder      *capture.Recorder
	authorization *ProviderAuthorization
	handler       *httputil.ReverseProxy
}

// Each request snapshots an immutable provider target;
// provider handlers share separate proxied and direct connection pools.
type ProxyServer struct {
	mu              sync.RWMutex
	transport       http.RoundTripper
	directTransport *http.Transport
	providers       map[string]*providerTarget
	active          atomic.Pointer[providerTarget]
}

func NewProxyServer() *ProxyServer {
	return &ProxyServer{transport: newDirectTransport(), directTransport: newDirectTransport(), providers: make(map[string]*providerTarget)}
}

func NewProxyServerWithHTTPClientProxy(config HTTPClientProxyConfig) (*ProxyServer, error) {
	transport, err := newHTTPTransport(config)
	if err != nil {
		return nil, err
	}
	return &ProxyServer{transport: transport, directTransport: newDirectTransport(), providers: make(map[string]*providerTarget)}, nil
}

func (s *ProxyServer) AddProvider(name, baseURL string, recorder *capture.Recorder, options ProviderOptions) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("provider name must not be empty")
	}
	if _, exists := s.providers[name]; exists {
		return fmt.Errorf("provider %q already exists", name)
	}
	target, err := url.Parse(baseURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" {
		return fmt.Errorf("provider %q base_url must be an HTTP(S) URL without credentials, query or fragment", name)
	}
	authorization, err := validateProviderAuthorization(options.Authorization)
	if err != nil {
		return err
	}
	handler := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			// Preserve native escaped paths and query bytes; TokenHub never parses them.
			r.Out.URL.Path = strings.TrimRight(target.Path, "/") + r.In.URL.Path
			r.Out.URL.RawPath = strings.TrimRight(target.EscapedPath(), "/") + r.In.URL.EscapedPath()
			r.Out.URL.RawQuery = r.In.URL.RawQuery
			if authorization != nil {
				if authorization.BackendValue == "" {
					r.Out.Header.Del(authorization.Header)
				} else {
					r.Out.Header.Set(authorization.Header, authorization.BackendValue)
				}
			}
		},
		Transport:  captureTransport{server: s, direct: options.ProxyEnabled != nil && !*options.ProxyEnabled},
		BufferPool: copyBuffers{},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			message := fmt.Sprintf("proxy request failed: %v", err)
			http.Error(w, message, http.StatusBadGateway)
			if tx, _ := r.Context().Value(transactionKey{}).(*capture.Transaction); tx != nil {
				tx.Fail(message, err)
			}
		},
	}
	s.providers[name] = &providerTarget{name, recorder, authorization, handler}
	return nil
}

type ProviderConfig struct {
	BaseURL  string
	Recorder *capture.Recorder
	Options  ProviderOptions
}

// ReplaceProviders reuses the connection pool and leaves in-flight targets intact.
func (s *ProxyServer) ReplaceProviders(providers map[string]ProviderConfig, active string) error {
	next := &ProxyServer{transport: s.transport, directTransport: s.directTransport, providers: make(map[string]*providerTarget)}
	for name, provider := range providers {
		if err := next.AddProvider(name, provider.BaseURL, provider.Recorder, provider.Options); err != nil {
			return err
		}
	}
	if err := next.SelectProvider(active); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = next.providers
	s.active.Store(next.active.Load())
	return nil
}

func (s *ProxyServer) SelectProvider(name string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if name == "" {
		s.active.Store(nil)
		return nil
	}
	target, exists := s.providers[name]
	if !exists {
		return fmt.Errorf("provider %q does not exist", name)
	}
	s.active.Store(target)
	return nil
}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := s.active.Load()
	if target == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"type":"provider_not_selected","message":"No provider selected. Select a provider in TokenHub settings."}}`)
		return
	}
	if auth := target.authorization; auth != nil && auth.ClientValue != "" && r.Header.Get(auth.Header) != auth.ClientValue {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if target.recorder != nil {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		tx := target.recorder.Begin(r.Context(), capture.Metadata{
			Provider: target.name, Method: r.Method, UserAgent: r.UserAgent(),
			SourceURL: scheme + "://" + r.Host + r.URL.RequestURI(), RequestStartedAt: time.Now(),
		})
		defer func() {
			if recovered := recover(); recovered != nil {
				tx.End(fmt.Errorf("proxy interrupted: %v", recovered))
				panic(recovered)
			}
			tx.End(r.Context().Err())
		}()
		r = r.WithContext(context.WithValue(r.Context(), transactionKey{}, tx))
	}
	target.handler.ServeHTTP(w, r)
}

type transactionKey struct{}
type captureTransport struct {
	server *ProxyServer
	direct bool
}

func (t captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tx, _ := r.Context().Value(transactionKey{}).(*capture.Transaction)
	if tx != nil {
		tx.CaptureRequest(r, r.Header.Get("Upgrade") != "" && httpguts.HeaderValuesContainsToken(r.Header["Connection"], "upgrade"))
	}
	transport := t.server.transport
	if t.direct {
		transport = t.server.directTransport
	}
	response, err := transport.RoundTrip(r)
	if err == nil && tx != nil {
		if err = tx.CaptureResponse(response); err != nil {
			response.Body.Close()
			return nil, err
		}
	}
	return response, err
}

func (s *ProxyServer) ShutdownCaptures(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[*capture.Recorder]bool)
	var result error
	for _, target := range s.providers {
		if target.recorder != nil && !seen[target.recorder] {
			seen[target.recorder] = true
			result = errors.Join(result, target.recorder.Shutdown(ctx))
		}
	}
	if transport, ok := s.transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
	s.directTransport.CloseIdleConnections()
	return result
}

var copyBufferPool = sync.Pool{New: func() any { return make([]byte, 32<<10) }}

type copyBuffers struct{}

func (copyBuffers) Get() []byte  { return copyBufferPool.Get().([]byte) }
func (copyBuffers) Put(p []byte) { copyBufferPool.Put(p) }

func validateProviderAuthorization(authorization *ProviderAuthorization) (*ProviderAuthorization, error) {
	if authorization == nil {
		return nil, nil
	}

	validated := *authorization
	validated.Header = strings.TrimSpace(validated.Header)
	if validated.Header == "" {
		validated.Header = "Authorization"
	}
	if !httpguts.ValidHeaderFieldName(validated.Header) {
		return nil, fmt.Errorf("invalid authorization header name %q", validated.Header)
	}
	if validated.BackendValue == "" && validated.ClientValue == "" {
		return nil, fmt.Errorf("authorization requires a backend value, a client value, or both")
	}
	if validated.BackendValue != "" && !httpguts.ValidHeaderFieldValue(validated.BackendValue) {
		return nil, fmt.Errorf("authorization backend value is not a valid HTTP header value")
	}
	if validated.ClientValue != "" && !httpguts.ValidHeaderFieldValue(validated.ClientValue) {
		return nil, fmt.Errorf("authorization client value is not a valid HTTP header value")
	}
	return &validated, nil
}
