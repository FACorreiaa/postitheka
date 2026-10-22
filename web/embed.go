// Package web serves the built web app from inside the binary, so production
// is one container with no separate static host and no cross-origin setup.
//
// dist/ is Vite's output (`task web:build`). Only dist/.gitkeep is committed,
// so a fresh clone compiles; it then serves a page saying the app is unbuilt
// instead of failing to start.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves files from dist, and index.html for any other path so the
// app's client-side routes survive a reload.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(root)
	index, err := fs.ReadFile(root, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err != nil {
			http.Error(w, "The web app is not built into this binary. Run `task web:build`, then rebuild the server.", http.StatusServiceUnavailable)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if _, statErr := fs.Stat(root, name); statErr == nil {
				// Vite fingerprints everything under assets/, so a name
				// there never changes content and can be cached for good.
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}

		// index.html is the one file that changes between deploys under the
		// same name, so it is never cached.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
