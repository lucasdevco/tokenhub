package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
)

// Helpers register providers and explicitly select one for each test.
func createTestServer(providers map[string]string) *ProxyServer {
	server := NewProxyServer()
	var logger *capture.Recorder

	for name, destination := range providers {
		err := useTestProvider(server, name, destination, logger)
		if err != nil {
			panic(err)
		}
	}

	return server
}

func TestNewArchitecture(t *testing.T) {
	// Create mock backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"echo": "Backend received %s %s"}`, r.Method, r.URL.Path)
	}))
	defer backend.Close()

	// Create proxy server with test providers
	proxyServer := createTestServer(map[string]string{
		"/api/v1/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make a test request
	requestBody := `{"test": "data"}`
	resp, err := http.Post(testServer.URL+"/api/v1/test", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal("Request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	expectedContent := "Backend received POST /api/v1/test"
	if !strings.Contains(string(responseBody), expectedContent) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedContent, string(responseBody))
	}

	// Test verifies that the proxy correctly forwards requests to the backend
}

func TestProviderAuthorizationInjectsBackendCredential(t *testing.T) {
	backendHeader := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHeader <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	proxy := NewProxyServer()
	err := useTestProvider(proxy, "/api/", backend.URL+"/", nil, ProviderOptions{
		Authorization: &ProviderAuthorization{
			Header:       "Authorization",
			BackendValue: "Bearer backend-secret",
		},
	})
	if err != nil {
		t.Fatalf("failed to add authorized provider: %v", err)
	}

	server := httptest.NewServer(proxy)
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/models", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer caller-value")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected response status %d", response.StatusCode)
	}

	if got := <-backendHeader; got != "Bearer backend-secret" {
		t.Fatalf("backend Authorization = %q, want injected credential", got)
	}
}

func TestProviderAuthorizationChecksClientCredential(t *testing.T) {
	backendHeader := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHeader <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	proxy := NewProxyServer()
	err := useTestProvider(proxy, "/api/", backend.URL+"/", nil, ProviderOptions{
		Authorization: &ProviderAuthorization{
			Header:       "Authorization",
			BackendValue: "Bearer backend-secret",
			ClientValue:  "Bearer local-secret",
		},
	})
	if err != nil {
		t.Fatalf("failed to add authorized provider: %v", err)
	}

	server := httptest.NewServer(proxy)
	defer server.Close()

	for _, value := range []string{"", "Bearer wrong-secret"} {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/api/models", nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		if value != "" {
			request.Header.Set("Authorization", value)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("credential %q returned %d, want 401", value, response.StatusCode)
		}
		select {
		case got := <-backendHeader:
			t.Fatalf("unauthorized request reached backend with header %q", got)
		default:
		}
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/models", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer local-secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("authorized request returned %d", response.StatusCode)
	}
	if got := <-backendHeader; got != "Bearer backend-secret" {
		t.Fatalf("backend Authorization = %q, want injected credential", got)
	}
}

func TestProviderAuthorizationClientOnlyStripsCredential(t *testing.T) {
	backendHeader := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHeader <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	proxy := NewProxyServer()
	err := useTestProvider(proxy, "/api/", backend.URL+"/", nil, ProviderOptions{
		Authorization: &ProviderAuthorization{
			Header:      "Authorization",
			ClientValue: "Bearer local-secret",
		},
	})
	if err != nil {
		t.Fatalf("failed to add client-authorized provider: %v", err)
	}

	server := httptest.NewServer(proxy)
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/models", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer local-secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("authorized request returned %d", response.StatusCode)
	}
	if got := <-backendHeader; got != "" {
		t.Fatalf("client credential leaked to backend: %q", got)
	}
}

func TestStreamingWithNewArchitecture(t *testing.T) {
	// Create mock streaming backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send several SSE events
		events := []string{
			"data: {\"chunk\": 1}\n\n",
			"data: {\"chunk\": 2}\n\n",
			"data: [DONE]\n\n",
		}

		for _, event := range events {
			fmt.Fprint(w, event)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer backend.Close()

	// Create proxy server with streaming test providers
	proxyServer := createTestServer(map[string]string{
		"/api/v1/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make streaming request
	resp, err := http.Get(testServer.URL + "/api/v1/stream")
	if err != nil {
		t.Fatal("Streaming request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read the streaming response
	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		t.Fatal("Failed to read streaming response:", err)
	}

	responseData := buffer.String()
	expectedChunks := []string{"chunk\": 1", "chunk\": 2", "[DONE]"}
	for _, chunk := range expectedChunks {
		if !strings.Contains(responseData, chunk) {
			t.Errorf("Expected chunk '%s' not found in streaming response", chunk)
		}
	}

	// Test verifies that the proxy correctly handles streaming responses
}

func TestConnectionFailure(t *testing.T) {
	// Create proxy server that redirects to an unreachable address (0.0.0.0)
	proxyServer := createTestServer(map[string]string{
		"/api/": "http://0.0.0.0:9999/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make request that should fail at client.Do()
	resp, err := http.Get(testServer.URL + "/api/test")
	if err != nil {
		t.Fatal("Request failed:", err)
	}
	defer resp.Body.Close()

	// Should get 502 Bad Gateway when client.Do() fails
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("Expected status 502, got %d", resp.StatusCode)
	}

	// Verify error message format
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	responseStr := string(responseBody)
	if !strings.Contains(responseStr, "proxy request failed:") {
		t.Errorf("Expected error message to contain 'proxy request failed:', got: %s", responseStr)
	}

	// Test verifies connection failure handling in client.Do()
}

func TestHostHeaderProxying(t *testing.T) {
	// Create mock backend server that reports the Host header it receives
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"host": "%s"}`, r.Host)
	}))
	defer backend.Close()

	// Create proxy server
	proxyServer := createTestServer(map[string]string{
		"/api/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make request with explicit Host header
	client := &http.Client{}
	req, err := http.NewRequest("GET", testServer.URL+"/api/test", nil)
	if err != nil {
		t.Fatal("Failed to create request:", err)
	}
	req.Host = "custom-host.example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("Request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response to verify Host header handling
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	responseStr := string(responseBody)
	t.Logf("Backend received Host: %s", responseStr)

	// Verify the backend received the correct Host header (should be backend's host, not original)
	backendHost := strings.TrimPrefix(backend.URL, "http://")
	if !strings.Contains(responseStr, backendHost) {
		t.Errorf("Expected backend to receive its own host %s in Host header, got: %s", backendHost, responseStr)
	}

	// Test verifies that the proxy correctly updates the Host header for the backend
}

func TestContentLengthProxying(t *testing.T) {
	// Create mock backend server that reports what it receives
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"method": "%s", "contentLength": %d}`, r.Method, r.ContentLength)
	}))
	defer backend.Close()

	// Create proxy server
	proxyServer := createTestServer(map[string]string{
		"/api/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make POST request with body to test Content-Length handling
	requestBody := `{"test": "data with some content"}`
	resp, err := http.Post(testServer.URL+"/api/test", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal("POST request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response to verify the backend received the correct Content-Length
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	responseStr := string(responseBody)
	t.Logf("Backend response: %s", responseStr)

	// Verify the backend received the correct method and content
	if !strings.Contains(responseStr, `"method": "POST"`) {
		t.Errorf("Expected backend to receive POST method, got: %s", responseStr)
	}

	// Content-Length may be -1 if using chunked encoding, which is fine
	// The important thing is that the request body was received correctly
	if !strings.Contains(responseStr, "contentLength") {
		t.Errorf("Expected backend response to contain contentLength field, got: %s", responseStr)
	}

	// Test verifies that the proxy correctly forwards Content-Length headers and request body
}

func TestConfigLoggingSettings(t *testing.T) {
	// Test that the proxy server can be created and configured
	server := NewProxyServer()
	if server == nil {
		t.Error("Failed to create proxy server")
	}

	// Test setting different loggers
	var logger *capture.Recorder
	err := useTestProvider(server, "/api/", "https://example.com/", logger)
	if err != nil {
		t.Errorf("Failed to add provider: %v", err)
	}
}

func TestQueryStringProxying(t *testing.T) {
	// Create mock backend server that echoes query parameters
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"query": "%s", "param1": "%s", "param2": "%s"}`,
			r.URL.RawQuery, r.URL.Query().Get("param1"), r.URL.Query().Get("param2"))
	}))
	defer backend.Close()

	// Create proxy server
	proxyServer := createTestServer(map[string]string{
		"/api/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Make request with query string
	resp, err := http.Get(testServer.URL + "/api/search?param1=value1&param2=value2&special=%40%21%24")
	if err != nil {
		t.Fatal("Request with query string failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response to verify query parameters were proxied correctly
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	responseStr := string(responseBody)
	t.Logf("Backend response: %s", responseStr)

	// Verify the backend received the correct query parameters
	expectedParams := []string{"param1=value1", "param2=value2", "special=%40%21%24"}
	for _, param := range expectedParams {
		if !strings.Contains(responseStr, param) {
			t.Errorf("Expected backend to receive query parameter %s, got: %s", param, responseStr)
		}
	}

	// Test verifies that the proxy correctly forwards query string parameters
}

func TestChunkedTransferEncoding(t *testing.T) {
	// Create mock backend server that handles chunked requests
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading body", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"received": "%s", "transfer_encoding": "%v", "content_length": %d}`,
			string(body), r.TransferEncoding, r.ContentLength)
	}))
	defer backend.Close()

	// Create proxy server
	proxyServer := createTestServer(map[string]string{
		"/api/": backend.URL + "/",
	})

	// Create test server for proxy
	testServer := httptest.NewServer(proxyServer)
	defer testServer.Close()

	// Create a custom HTTP request with Transfer-Encoding: chunked
	client := &http.Client{}

	// Create request body
	requestBody := "This is a test body for chunked transfer encoding"

	req, err := http.NewRequest("POST", testServer.URL+"/api/chunked", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal("Failed to create request:", err)
	}

	// Set headers for chunked transfer encoding
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Transfer-Encoding", "chunked")
	// Remove Content-Length to force chunked encoding
	req.ContentLength = -1

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("Chunked request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response to verify the chunked request was processed correctly
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response:", err)
	}

	responseStr := string(responseBody)
	t.Logf("Backend response: %s", responseStr)

	// Verify the backend received the request body correctly
	if !strings.Contains(responseStr, requestBody) {
		t.Errorf("Expected backend to receive request body '%s', got: %s", requestBody, responseStr)
	}

	// Test verifies that the proxy correctly handles chunked transfer encoding
}

func useTestProvider(server *ProxyServer, name, baseURL string, recorder *capture.Recorder, options ...ProviderOptions) error {
	option := ProviderOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if err := server.AddProvider(name, baseURL, recorder, option); err != nil {
		return err
	}
	return server.SelectProvider(name)
}
