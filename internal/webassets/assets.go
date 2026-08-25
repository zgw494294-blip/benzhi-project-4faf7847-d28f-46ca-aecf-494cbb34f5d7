package webassets

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var embedded embed.FS

type Handler struct{ files fs.FS }

func NewHandler() http.Handler {
	files, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	return &Handler{files: files}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	if requested != "index.html" && requested != "app.css" && requested != "features.css" && requested != "app.js" {
		requested = "index.html"
	}
	data, err := fs.ReadFile(h.files, requested)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(requested) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
