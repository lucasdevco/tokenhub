package management

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lucasdevco/tokenhub/web"
)

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func NewLogHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/requests", store.list)
	mux.HandleFunc("GET /api/requests/{key}", func(w http.ResponseWriter, r *http.Request) {
		t, ok := store.transaction(r.PathValue("key"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		send(w, t)
	})
	mux.HandleFunc("GET /api/files", store.listFiles)
	mux.HandleFunc("GET /api/file", store.file)
	mux.HandleFunc("GET /api/tail", store.tail)
	mux.HandleFunc("GET /api/events", store.events)
	mux.HandleFunc("GET /api/capabilities", func(w http.ResponseWriter, r *http.Request) { send(w, map[string]bool{"settings": false}) })
	mux.HandleFunc("GET /api/", http.NotFound)
	mux.Handle("GET /", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		host := r.Host
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		if !loopback(host) {
			http.Error(w, "Loopback host required", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				http.Error(w, "Cross-origin access denied", http.StatusForbidden)
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Read-only viewer", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodHead && (r.URL.Path == "/api/tail" || r.URL.Path == "/api/events") {
			w.Header().Set("Content-Type", "text/event-stream")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func page(r *http.Request) (int, int, error) {
	limit, offset := 100, 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, err
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, err
		}
	}
	if limit < 1 || limit > 500 || offset < 0 {
		return 0, 0, fmt.Errorf("invalid pagination")
	}
	return limit, offset, nil
}

type requestBucket struct {
	Started time.Time `json:"started"`
	Count   int       `json:"count"`
}

type requestHistogram struct {
	Buckets         []requestBucket `json:"buckets"`
	IntervalSeconds int             `json:"interval_seconds"`
	Total           int             `json:"total"`
}

func newRequestHistogram(hours string, now time.Time) (requestHistogram, error) {
	interval, count := 30*time.Minute, 48
	switch hours {
	case "1":
		interval, count = 2*time.Minute, 30
	case "", "24":
	case "168":
		interval, count = 3*time.Hour, 56
	default:
		return requestHistogram{}, fmt.Errorf("invalid histogram hours: use 1, 24 or 168")
	}
	start := now.UTC().Truncate(interval).Add(interval).Add(-time.Duration(count) * interval)
	histogram := requestHistogram{Buckets: make([]requestBucket, count), IntervalSeconds: int(interval.Seconds())}
	for i := range histogram.Buckets {
		histogram.Buckets[i].Started = start.Add(time.Duration(i) * interval)
	}
	return histogram, nil
}

func offsetValue(r *http.Request) (int64, error) {
	value := r.URL.Query().Get("offset")
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		value = id
	}
	if value == "" {
		return 0, nil
	}
	offset, err := strconv.ParseInt(value, 10, 64)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid byte offset")
	}
	return offset, nil
}

func (s *Store) file(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	f, err := s.open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "Cannot stat capture", 500)
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		http.ServeContent(w, r, name, info.ModTime(), f)
		return
	}
	offset, err := offsetValue(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if offset > info.Size() {
		http.Error(w, "Offset exceeds current capture size", 416)
		return
	}
	data := make([]byte, 64<<10)
	n, err := f.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		http.Error(w, "Cannot read capture", 500)
		return
	}
	send(w, map[string]any{"data": base64.StdEncoding.EncodeToString(data[:n]), "offset": offset, "next": offset + int64(n), "size": info.Size()})
}

func eventStream(w http.ResponseWriter) (*http.ResponseController, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	return controller, controller.Flush()
}

func sendEvent(w http.ResponseWriter, controller *http.ResponseController, kind, id string, value any) error {
	_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err = fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
		return err
	}
	err = controller.Flush()
	_ = controller.SetWriteDeadline(time.Time{})
	return err
}

func (s *Store) events(w http.ResponseWriter, r *http.Request) {
	controller, err := eventStream(w)
	if err != nil {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		s.mu.RLock()
		revision, wake, problem := s.revision, s.wake, s.problem
		s.mu.RUnlock()
		if err := sendEvent(w, controller, "change", "", map[string]any{"revision": revision, "problem": problem}); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-wake:
		case <-heartbeat.C:
		}
	}
}

func (s *Store) tail(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if !strings.HasSuffix(name, ".bin") {
		http.Error(w, "Only capture streams can be followed", 400)
		return
	}
	offset, err := offsetValue(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, err := s.open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "Cannot stat capture", 500)
		return
	}
	if offset > info.Size() {
		http.Error(w, "Offset exceeds current capture size", 416)
		return
	}
	controller, err := eventStream(w)
	if err != nil {
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	data := make([]byte, 64<<10)
	lastHeartbeat := time.Now()
	for {
		if r.Context().Err() != nil {
			return
		}
		// Observe completion BEFORE reading EOF. This prevents a final append
		// between the read and completion lookup from being lost.
		finished := s.finished(name)
		n, readErr := f.ReadAt(data, offset)
		if n > 0 {
			next := offset + int64(n)
			if sendEvent(w, controller, "chunk", strconv.FormatInt(next, 10), map[string]any{"offset": offset, "next": next, "data": base64.StdEncoding.EncodeToString(data[:n])}) != nil {
				return
			}
			offset = next
			continue
		}
		if readErr != nil && readErr != io.EOF {
			_ = sendEvent(w, controller, "failure", "", map[string]string{"error": "Cannot read capture"})
			return
		}
		if finished {
			_ = sendEvent(w, controller, "end", strconv.FormatInt(offset, 10), map[string]any{"offset": offset})
			return
		}
		if time.Since(lastHeartbeat) >= 15*time.Second {
			if sendEvent(w, controller, "waiting", "", map[string]any{"offset": offset}) != nil {
				return
			}
			lastHeartbeat = time.Now()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
