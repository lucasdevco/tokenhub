package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if file, err := dist.Open(path); err == nil {
				info, _ := file.Stat()
				file.Close()
				if info != nil && !info.IsDir() {
					if strings.HasPrefix(path, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					if strings.HasSuffix(path, ".js") {
						w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
					}
					if strings.HasSuffix(path, ".css") {
						w.Header().Set("Content-Type", "text/css; charset=utf-8")
					}
					files.ServeHTTP(w, r)
					return
				}
			}
		}
		if strings.HasPrefix(path, "assets/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			_, _ = w.Write(index)
		}
	})
}
