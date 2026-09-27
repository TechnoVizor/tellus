package tellus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/TechnoVizor/tellus/form"
)

// Storage is where an uploaded file's bytes are kept and how a browser reaches
// it. LocalStorage is the built-in implementation; a host application can
// supply its own, for example one backed by S3, by implementing this
// interface. It is the same interface form.Storage names, re-exported here
// under a name that reads better in Config.
type Storage = form.Storage

// LocalStorage saves uploads to a directory on disk and serves them back
// under urlPrefix. Give its Handler to your router at urlPrefix, or call
// Panel.Mount, which does this for you when Config.Storage is a *LocalStorage.
type LocalStorage struct {
	dir       string
	urlPrefix string
}

// NewLocalStorage creates dir if it does not exist and returns a Storage
// backed by it. urlPrefix is the path files are served at, for example
// "/uploads"; it is not mounted by this call, see Handler.
func NewLocalStorage(dir, urlPrefix string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("tellus: create upload directory %q: %w", dir, err)
	}
	return &LocalStorage{dir: dir, urlPrefix: strings.TrimSuffix(urlPrefix, "/")}, nil
}

// Save writes r under a name derived from filename's extension, unique on
// every call, and returns the path relative to dir. filename is used only for
// its extension; it must not point outside dir.
func (s *LocalStorage) Save(ctx context.Context, filename string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if filename == "" || filepath.Base(filename) != filename {
		return "", fmt.Errorf("tellus: invalid upload filename %q", filename)
	}
	name := randomHex(16) + strings.ToLower(filepath.Ext(filename))
	full := filepath.Join(s.dir, name)
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(full)
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return name, nil
}

// URL returns the address a browser fetches path from.
func (s *LocalStorage) URL(path string) string { return s.urlPrefix + "/" + path }

// Delete removes the file at path. A missing file is not an error.
func (s *LocalStorage) Delete(_ context.Context, path string) error {
	err := os.Remove(filepath.Join(s.dir, path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Handler serves the stored files. Mount it with http.StripPrefix at the
// urlPrefix given to NewLocalStorage.
func (s *LocalStorage) Handler() http.Handler {
	files := http.FileServerFS(os.DirFS(s.dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // no directory listings
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // names are unique per upload
		files.ServeHTTP(w, r)
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return hex.EncodeToString(b)
}
