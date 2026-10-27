package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	proxycore "github.com/lucasdevco/tokenhub/internal/proxy"
)

// Runtime owns every listener and connection belonging to one proxy run.
type Runtime struct {
	Errors      chan error
	servers     []*http.Server
	cancel      context.CancelFunc
	done        <-chan struct{}
	mu          sync.Mutex
	connections map[net.Conn]bool
	once        sync.Once
	proxy       *proxycore.ProxyServer
	recorder    *capture.Recorder
}

func Start(parent context.Context, config *Config) (*Runtime, error) {
	ctx, cancel := context.WithCancel(parent)
	run := &Runtime{Errors: make(chan error, 3), cancel: cancel, done: ctx.Done(), connections: map[net.Conn]bool{}}
	logger, err := buildRecorder(config)
	if err != nil {
		cancel()
		return nil, err
	}
	run.recorder = logger
	started := false
	defer func() {
		if !started && logger != nil {
			_ = logger.Shutdown(context.Background())
		}
	}()
	client := buildHTTPClientProxyConfig(config)
	var handlers []namedServer
	if config.Server != nil {
		handler, err := buildReverseProxy(config, logger, client)
		if err != nil {
			cancel()
			return nil, err
		}
		run.proxy = handler
		handlers = append(handlers, namedServer{
			name: "reverse",
			server: &http.Server{
				Addr:                         net.JoinHostPort(config.Server.Host, fmt.Sprint(config.Server.Port)),
				Handler:                      handler,
				DisableGeneralOptionsHandler: true,
			},
		})
	}
	// Bind all listeners before reporting success; roll back every bind on failure.
	listeners := make([]net.Listener, 0, len(handlers))
	for _, entry := range handlers {
		listener, err := net.Listen("tcp", entry.server.Addr)
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			cancel()
			return nil, fmt.Errorf("%s: %w", entry.name, err)
		}
		listeners = append(listeners, &trackedListener{Listener: listener, owner: run})
		entry.server.ReadHeaderTimeout = 10 * time.Second
		run.servers = append(run.servers, entry.server)
	}
	for i, entry := range handlers {
		log.Printf("%s listening on %s", entry.name, listeners[i].Addr())
		go func(entry namedServer, listener net.Listener) {
			err := entry.server.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				select {
				case run.Errors <- fmt.Errorf("%s: %w", entry.name, err):
				default:
				}
			}
		}(entry, listeners[i])
	}
	started = true
	return run, nil
}

func (r *Runtime) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.stop(ctx)
}

func (r *Runtime) stop(ctx context.Context) error {
	var result error
	r.once.Do(func() {
		log.Print("Stopping proxy service")
		r.cancel()
		for _, server := range r.servers {
			if err := server.Shutdown(ctx); err != nil {
				result = errors.Join(result, err)
			}
			_ = server.Close()
		}
		// http.Server does not close hijacked WebSocket connections.
		r.mu.Lock()
		connections := make([]net.Conn, 0, len(r.connections))
		for conn := range r.connections {
			connections = append(connections, conn)
		}
		r.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		if r.proxy != nil || r.recorder != nil {
			// HTTP shutdown may have exhausted its budget. Give metadata writes
			// a fresh grace period after active connections have been closed.
			captureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if r.proxy != nil {
				result = errors.Join(result, r.proxy.ShutdownCaptures(captureCtx))
			}
			// The runtime also owns an unused recorder (for example, no providers
			// configured yet); it still has a database connection to release.
			if r.recorder != nil {
				result = errors.Join(result, r.recorder.Shutdown(captureCtx))
			}
		}
		if result != nil {
			log.Printf("Proxy shutdown failed: %v", result)
		} else {
			log.Print("Proxy service stopped")
		}
	})
	return result
}

// Tracking at the connection boundary also observes closes after HTTP hijacking.
// ConnState does not report those closes; retaining them would leak over time.
type trackedListener struct {
	net.Listener
	owner *Runtime
}

func (l *trackedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tracked := &trackedConn{Conn: conn, owner: l.owner}
	l.owner.mu.Lock()
	l.owner.connections[tracked] = true
	l.owner.mu.Unlock()
	return tracked, nil
}

type trackedConn struct {
	net.Conn
	owner *Runtime
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.owner.mu.Lock()
	delete(c.owner.connections, c)
	c.owner.mu.Unlock()
	return err
}

type namedServer struct {
	name   string
	server *http.Server
}

func buildRecorder(config *Config) (*capture.Recorder, error) {
	enabled := config.Logging.Enabled
	for _, provider := range config.Providers {
		enabled = enabled || (provider.Logging != nil && *provider.Logging)
	}
	if !enabled {
		return nil, nil
	}
	recorder, err := capture.NewRecorder(config.Logging.LogDir, config.DatabasePath, config.Logging.Console)
	if err != nil {
		return nil, fmt.Errorf("failed to create recorder: %w", err)
	}
	log.Printf("Logging requests/responses to: %s", config.Logging.LogDir)
	return recorder, nil
}

func buildReverseProxy(config *Config, globalLogger *capture.Recorder, clientProxyConfig proxycore.HTTPClientProxyConfig) (*proxycore.ProxyServer, error) {
	proxy, err := proxycore.NewProxyServerWithHTTPClientProxy(clientProxyConfig)
	if err != nil {
		return nil, err
	}
	if err := proxy.ReplaceProviders(providerDefinitions(config, globalLogger), config.ActiveProvider); err != nil {
		return nil, err
	}
	return proxy, nil
}

func providerDefinitions(config *Config, globalLogger *capture.Recorder) map[string]proxycore.ProviderConfig {
	providers := make(map[string]proxycore.ProviderConfig, len(config.Providers))
	for name, provider := range config.Providers {
		var logger *capture.Recorder
		enabled := config.Logging.Enabled
		if provider.Logging != nil {
			enabled = *provider.Logging
		}
		if enabled {
			logger = globalLogger
		}
		options := proxycore.ProviderOptions{ProxyEnabled: provider.ProxyEnabled}
		if auth := provider.Authorization; auth != nil {
			options.Authorization = &proxycore.ProviderAuthorization{Header: auth.Header}
			if auth.BackendKey != "" {
				options.Authorization.BackendValue = renderAuthorizationValue(auth.Template, auth.BackendKey)
			}
			if auth.ClientKey != "" {
				options.Authorization.ClientValue = renderAuthorizationValue(auth.Template, auth.ClientKey)
			}
		}
		providers[name] = proxycore.ProviderConfig{BaseURL: provider.BaseURL, Recorder: logger, Options: options}
	}
	return providers
}
