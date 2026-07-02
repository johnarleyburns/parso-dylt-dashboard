// Package webui embeds the built React dashboard so the single oilfield binary
// can serve the frontend itself (no Cloudflare Pages, no separate static host).
//
// The Makefile copies dashboard/web/frontend/dist into ./webroot before `go build`.
// A committed placeholder index.html keeps the package buildable before any
// frontend build has run.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:webroot
var embedded embed.FS

// Embedded returns the built frontend as a filesystem rooted at the webroot dir.
func Embedded() fs.FS {
	sub, err := fs.Sub(embedded, "webroot")
	if err != nil {
		panic(err)
	}
	return sub
}

// Handler serves a single-page app from fsys: real files when they exist,
// otherwise index.html so client-side routing works.
func Handler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/"
			http.ServeFileFS(w, r2, fsys, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
