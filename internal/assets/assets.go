// Package assets embeds the panel's CSS and JavaScript so a host application
// needs no CDN and no frontend build.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var static embed.FS

var version = computeVersion()

// Version is a short content hash used for cache-busting asset URLs.
func Version() string { return version }

// Handler serves the embedded files. Mount it with http.StripPrefix. Requests
// carrying the current ?v=<Version()> are cached as immutable, and so is
// everything under fonts/ (a changed font gets a new file name). Everything
// else revalidates through an ETag.
func Handler() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // no directory listings
			return
		}
		// Set explicitly: on Windows the OS registry can map .js to text/plain.
		switch path.Ext(r.URL.Path) {
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".woff2":
			w.Header().Set("Content-Type", "font/woff2")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("ETag", `"`+version+`"`)
		if r.URL.Query().Get("v") == version || strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func computeVersion() string {
	h := sha256.New()
	err := fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := static.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		h.Write(data)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}
