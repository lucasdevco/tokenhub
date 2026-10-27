package service

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	proxycore "github.com/lucasdevco/tokenhub/internal/proxy"
)

func buildHTTPClientProxyConfig(config *Config) proxycore.HTTPClientProxyConfig {
	return proxycore.HTTPClientProxyConfig{
		ProxyURL:             strings.TrimSpace(config.HTTPClient.ProxyURL),
		ProxyFromEnvironment: config.HTTPClient.ProxyFromEnvironment,
	}
}

type httpClientProxyEndpoint struct {
	label string
	url   *url.URL
}

type listenerAddress struct {
	name string
	host string
	port int
}

func describeHTTPClientProxyConfig(config proxycore.HTTPClientProxyConfig) ([]httpClientProxyEndpoint, string, error) {
	if strings.TrimSpace(config.ProxyURL) != "" {
		proxyURL, err := proxycore.ParseHTTPClientProxyURL(config.ProxyURL)
		if err != nil {
			return nil, "", err
		}
		return []httpClientProxyEndpoint{{label: "http_client.proxy_url", url: proxyURL}},
			fmt.Sprintf("HTTP client proxy: %s (from http_client.proxy_url)", proxyURL.Redacted()), nil
	}

	if config.ProxyFromEnvironment != nil && !*config.ProxyFromEnvironment {
		return nil, "HTTP client proxy: disabled", nil
	}

	envConfig := proxycore.ReadHTTPClientProxyEnvironment()
	endpoints := []httpClientProxyEndpoint{}
	parts := []string{}
	if strings.TrimSpace(envConfig.HTTPProxy) != "" {
		proxyURL, err := proxycore.ParseHTTPClientProxyURL(envConfig.HTTPProxy)
		if err != nil {
			return nil, "", fmt.Errorf("invalid HTTP_PROXY: %w", err)
		}
		endpoints = append(endpoints, httpClientProxyEndpoint{label: "HTTP_PROXY", url: proxyURL})
		parts = append(parts, fmt.Sprintf("HTTP_PROXY=%s", proxyURL.Redacted()))
	}
	if strings.TrimSpace(envConfig.HTTPSProxy) != "" {
		proxyURL, err := proxycore.ParseHTTPClientProxyURL(envConfig.HTTPSProxy)
		if err != nil {
			return nil, "", fmt.Errorf("invalid HTTPS_PROXY: %w", err)
		}
		endpoints = append(endpoints, httpClientProxyEndpoint{label: "HTTPS_PROXY", url: proxyURL})
		parts = append(parts, fmt.Sprintf("HTTPS_PROXY=%s", proxyURL.Redacted()))
	}
	if strings.TrimSpace(envConfig.NoProxy) != "" {
		parts = append(parts, fmt.Sprintf("NO_PROXY=%q", envConfig.NoProxy))
	}
	if len(endpoints) == 0 {
		return nil, "HTTP client proxy: none configured", nil
	}
	return endpoints, "HTTP client proxy: " + strings.Join(parts, ", "), nil
}

func configuredListenerAddresses(config *Config) []listenerAddress {
	listeners := []listenerAddress{}
	if config.Server != nil {
		listeners = append(listeners, listenerAddress{name: "reverse", host: config.Server.Host, port: config.Server.Port})
	}
	return listeners
}

func validateHTTPClientProxyEndpoints(endpoints []httpClientProxyEndpoint, listeners []listenerAddress) error {
	for _, endpoint := range endpoints {
		for _, listener := range listeners {
			pointsToSelf, err := proxyEndpointPointsToListener(endpoint.url, listener)
			if err != nil {
				return fmt.Errorf("failed to resolve HTTP client proxy %s=%s against %s listener %s:%d: %w", endpoint.label, endpoint.url.Redacted(), listener.name, listener.host, listener.port, err)
			}
			if pointsToSelf {
				return fmt.Errorf("HTTP client proxy %s=%s points to this process's %s listener at %s:%d", endpoint.label, endpoint.url.Redacted(), listener.name, listener.host, listener.port)
			}
		}
	}
	return nil
}

func proxyEndpointPointsToListener(proxyURL *url.URL, listener listenerAddress) (bool, error) {
	if proxyURL == nil {
		return false, nil
	}
	proxyPort, err := proxyURLPort(proxyURL)
	if err != nil {
		return false, err
	}
	if proxyPort != listener.port {
		return false, nil
	}

	proxyHost := proxyURL.Hostname()
	listenerHost := listener.host
	if isWildcardHost(proxyHost) {
		return true, nil
	}
	if strings.EqualFold(normalizeAddressHost(proxyHost), normalizeAddressHost(listenerHost)) {
		return true, nil
	}
	if isLoopbackHost(proxyHost) && isLoopbackHost(listenerHost) {
		return true, nil
	}
	if isWildcardHost(listenerHost) {
		return hostResolvesToLocal(proxyHost)
	}

	proxyIPs, err := resolveHostIPs(proxyHost)
	if err != nil {
		return false, err
	}
	listenerIPs, err := resolveHostIPs(listenerHost)
	if err != nil {
		return false, err
	}
	return ipSetsIntersect(proxyIPs, listenerIPs), nil
}

func proxyURLPort(proxyURL *url.URL) (int, error) {
	if port := proxyURL.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil {
			return 0, fmt.Errorf("invalid proxy port %q", port)
		}
		return parsedPort, nil
	}

	switch strings.ToLower(proxyURL.Scheme) {
	case "http":
		return 80, nil
	case "https":
		return 443, nil
	case "socks5", "socks5h":
		return 1080, nil
	default:
		return 0, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
}

func hostResolvesToLocal(host string) (bool, error) {
	hostIPs, err := resolveHostIPs(host)
	if err != nil {
		return false, err
	}
	localIPs, err := localHostIPs()
	if err != nil {
		return false, err
	}
	return ipSetsIntersect(hostIPs, localIPs), nil
}

func localHostIPs() ([]net.IP, error) {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			switch value := addr.(type) {
			case *net.IPNet:
				ips = append(ips, value.IP)
			case *net.IPAddr:
				ips = append(ips, value.IP)
			}
		}
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if hostnameIPs, err := net.LookupIP(hostname); err == nil {
			ips = append(ips, hostnameIPs...)
		}
	}
	return ips, nil
}

func resolveHostIPs(host string) ([]net.IP, error) {
	host = normalizeAddressHost(host)
	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q resolved to no addresses", host)
	}
	return ips, nil
}

func ipSetsIntersect(a, b []net.IP) bool {
	for _, left := range a {
		for _, right := range b {
			if left != nil && right != nil && left.Equal(right) {
				return true
			}
		}
	}
	return false
}

func isWildcardHost(host string) bool {
	host = normalizeAddressHost(host)
	return host == "" || host == "0.0.0.0" || host == "::"
}

func isLoopbackHost(host string) bool {
	host = normalizeAddressHost(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeAddressHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}
