package tellus

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageSaveAndURL(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir, "/uploads")
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.Save(context.Background(), "photo.jpg", strings.NewReader("fake jpeg bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("Save returned an empty path")
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil || string(data) != "fake jpeg bytes" {
		t.Fatalf("file on disk: %v %q", err, data)
	}
	if got := s.URL(path); got != "/uploads/"+path {
		t.Errorf("URL(%q) = %q", path, got)
	}
}

func TestLocalStorageCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "uploads")
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("setup: directory should not exist yet")
	}
	if _, err := NewLocalStorage(dir, "/uploads"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("directory was not created: %v", err)
	}
}

func TestLocalStorageGivesEachFileAUniqueName(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir(), "/uploads")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Save(context.Background(), "photo.jpg", strings.NewReader("a"))
	b, _ := s.Save(context.Background(), "photo.jpg", strings.NewReader("b"))
	if a == b {
		t.Fatalf("two uploads of the same filename collided: %q", a)
	}
}

func TestLocalStorageRejectsFilenamesThatEscapeTheDirectory(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir(), "/uploads")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../../etc/passwd", "..\\..\\windows", "/etc/passwd", ""} {
		if _, err := s.Save(context.Background(), name, strings.NewReader("x")); err == nil {
			t.Errorf("Save(%q) should have been rejected", name)
		}
	}
}

func TestLocalStorageDelete(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStorage(dir, "/uploads")
	path, _ := s.Save(context.Background(), "a.png", strings.NewReader("x"))
	if err := s.Delete(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
		t.Fatalf("file still exists after Delete: %v", err)
	}
	// Deleting a file that is already gone is not an error.
	if err := s.Delete(context.Background(), path); err != nil {
		t.Errorf("Delete of a missing file: %v", err)
	}
}

func TestLocalStorageHandlerServesFilesAndRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStorage(dir, "/uploads")
	path, _ := s.Save(context.Background(), "a.png", strings.NewReader("image bytes"))

	h := http.StripPrefix("/uploads/", s.Handler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/"+path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != "image bytes" {
		t.Errorf("body: %q", body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("directory listing: %d", rec.Code)
	}
}

func TestLocalStorageSaveIsContextCancelable(t *testing.T) {
	s, _ := NewLocalStorage(t.TempDir(), "/uploads")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Save(ctx, "a.png", bytes.NewReader(nil)); err == nil {
		t.Error("Save with an already-canceled context should fail")
	}
}
