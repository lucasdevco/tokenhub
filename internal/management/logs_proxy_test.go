package management

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lucasdevco/tokenhub/internal/database"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
	"github.com/lucasdevco/tokenhub/internal/proxy"
)

func TestLargeRequestInspectableAndTrailerForwarded(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Trailer", "X-Checksum")
		io.WriteString(w, `{"model":"test","usage":{"output_tokens":4}}`)
		w.Header().Set("X-Checksum", "final")
	}))
	defer backend.Close()
	dir := t.TempDir()
	recorder, err := capture.NewRecorder(dir, database.Path(dir), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := recorder.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	p := proxy.NewProxyServer()
	if err := p.AddProvider("test", backend.URL, recorder, proxy.ProviderOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := p.SelectProvider("test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(p)
	defer server.Close()
	payload := `{"input":"` + strings.Repeat("x", 5<<20) + `"}`
	resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.Trailer.Get("X-Checksum") != "final" {
		t.Fatal("trailer lost")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.ShutdownCaptures(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir, database.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Scan(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	NewLogHandler(store).ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/requests", nil))
	var result struct{ Items []Transaction }
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Items) != 1 {
		t.Fatal(w.Body.String())
	}
	item := result.Items[0]
	if item.State != "completed" || item.CaptureState != "completed" || item.RequestFile == "" || item.Metadata.OutputTokens == nil {
		t.Fatalf("summary lost: %+v", item)
	}
	data, _ := os.ReadFile(filepath.Join(dir, item.RequestFile))
	_, body, _ := bytes.Cut(data, []byte("\r\n\r\n"))
	if string(body) != payload {
		t.Fatal("captured request truncated")
	}
}
