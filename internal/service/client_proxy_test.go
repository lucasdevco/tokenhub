package service

import (
	"strings"
	"testing"

	proxycore "github.com/lucasdevco/tokenhub/internal/proxy"
)

func TestDescribeHTTPClientProxyConfigExplicitProxy(t *testing.T) {
	endpoints, message, err := describeHTTPClientProxyConfig(proxycore.HTTPClientProxyConfig{
		ProxyURL: "http://user:pass@proxy.example:3128",
	})
	if err != nil {
		t.Fatalf("describeHTTPClientProxyConfig failed: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected one proxy endpoint, got %d", len(endpoints))
	}
	if endpoints[0].label != "http_client.proxy_url" {
		t.Fatalf("unexpected endpoint label %q", endpoints[0].label)
	}
	if endpoints[0].url.Host != "proxy.example:3128" {
		t.Fatalf("unexpected endpoint URL %q", endpoints[0].url.String())
	}
	if !strings.Contains(message, "http_client.proxy_url") || !strings.Contains(message, "proxy.example:3128") {
		t.Fatalf("unexpected proxy log message %q", message)
	}
	if strings.Contains(message, "pass") {
		t.Fatalf("proxy log message leaked password: %q", message)
	}
}

func TestValidateHTTPClientProxyRejectsSelfProxyOnWildcardListener(t *testing.T) {
	proxyURL, err := proxycore.ParseHTTPClientProxyURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("failed to parse proxy URL: %v", err)
	}

	err = validateHTTPClientProxyEndpoints(
		[]httpClientProxyEndpoint{{label: "HTTP_PROXY", url: proxyURL}},
		[]listenerAddress{{name: "reverse", host: "0.0.0.0", port: 8080}},
	)
	if err == nil {
		t.Fatal("expected self proxy to fail")
	}
	if !strings.Contains(err.Error(), "points to this process") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHTTPClientProxyRejectsSelfProxyOnResolvedListener(t *testing.T) {
	proxyURL, err := proxycore.ParseHTTPClientProxyURL("http://127.0.0.1:5601")
	if err != nil {
		t.Fatalf("failed to parse proxy URL: %v", err)
	}

	err = validateHTTPClientProxyEndpoints(
		[]httpClientProxyEndpoint{{label: "HTTP_PROXY", url: proxyURL}},
		[]listenerAddress{{name: "reverse", host: "localhost", port: 5601}},
	)
	if err == nil {
		t.Fatal("expected self proxy to fail")
	}
	if !strings.Contains(err.Error(), "points to this process") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHTTPClientProxyAllowsDifferentPort(t *testing.T) {
	proxyURL, err := proxycore.ParseHTTPClientProxyURL("http://127.0.0.1:3128")
	if err != nil {
		t.Fatalf("failed to parse proxy URL: %v", err)
	}

	err = validateHTTPClientProxyEndpoints(
		[]httpClientProxyEndpoint{{label: "HTTP_PROXY", url: proxyURL}},
		[]listenerAddress{{name: "reverse", host: "127.0.0.1", port: 8080}},
	)
	if err != nil {
		t.Fatalf("expected different port to pass, got %v", err)
	}
}
