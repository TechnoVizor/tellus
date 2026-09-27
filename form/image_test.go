package form

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStorage is an in-memory Storage for tests: it never touches disk.
type fakeStorage struct {
	saved map[string]string // path -> content
	next  int
	err   error
}

func newFakeStorage() *fakeStorage { return &fakeStorage{saved: map[string]string{}} }

func (s *fakeStorage) Save(_ context.Context, filename string, r io.Reader) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.next++
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r); err != nil {
		return "", err
	}
	path := fmt.Sprintf("file%d-%s", s.next, filename)
	s.saved[path] = buf.String()
	return path, nil
}

func (s *fakeStorage) URL(path string) string { return "/uploads/" + path }

// uploadFile builds a real *multipart.FileHeader carrying content, the way a
// browser submission would, by round-tripping through an actual HTTP request.
func uploadFile(t *testing.T, fieldName, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatal(err)
	}
	fhs := req.MultipartForm.File[fieldName]
	if len(fhs) != 1 {
		t.Fatalf("expected one file, got %d", len(fhs))
	}
	return fhs[0]
}

var pngBytes = []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 100))
var jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte(strings.Repeat("x", 100))...)

func TestImageParseFileSavesAndReturnsTheURL(t *testing.T) {
	store := newFakeStorage()
	f := Image("Photo")
	fh := uploadFile(t, "Photo", "cat.png", pngBytes)

	v, msg := f.ParseFile(context.Background(), fh, tString, store)
	if msg != "" {
		t.Fatalf("unexpected message: %s", msg)
	}
	url, ok := v.(string)
	if !ok || !strings.HasPrefix(url, "/uploads/") {
		t.Fatalf("value: %v (%T)", v, v)
	}
	if len(store.saved) != 1 {
		t.Fatalf("nothing was saved: %+v", store.saved)
	}
}

func TestImageParseFileRejectsWrongType(t *testing.T) {
	store := newFakeStorage()
	f := Image("Photo")
	fh := uploadFile(t, "Photo", "notes.txt", []byte("plain text content, not an image at all"))

	_, msg := f.ParseFile(context.Background(), fh, tString, store)
	if msg == "" {
		t.Fatal("expected a validation message for a non-image upload")
	}
	if len(store.saved) != 0 {
		t.Error("a rejected upload must not be saved")
	}
}

func TestImageParseFileRejectsOversizedFiles(t *testing.T) {
	store := newFakeStorage()
	f := Image("Photo").MaxSize(10)
	fh := uploadFile(t, "Photo", "big.png", pngBytes) // well over 10 bytes

	_, msg := f.ParseFile(context.Background(), fh, tString, store)
	if msg == "" {
		t.Fatal("expected a validation message for an oversized file")
	}
	if len(store.saved) != 0 {
		t.Error("a rejected upload must not be saved")
	}
}

func TestImageParseFileAcceptsJPEGAndDetectsBySniffingNotExtension(t *testing.T) {
	store := newFakeStorage()
	f := Image("Photo")
	// The extension lies (.txt), the bytes are a real JPEG signature.
	fh := uploadFile(t, "Photo", "photo.txt", jpegBytes)

	_, msg := f.ParseFile(context.Background(), fh, tString, store)
	if msg != "" {
		t.Fatalf("content sniffing should have accepted this: %s", msg)
	}
}

func TestImageParseFileStorageErrorIsSurfaced(t *testing.T) {
	store := newFakeStorage()
	store.err = errors.New("disk full")
	f := Image("Photo")
	fh := uploadFile(t, "Photo", "cat.png", pngBytes)

	_, msg := f.ParseFile(context.Background(), fh, tString, store)
	if msg == "" {
		t.Fatal("a storage failure must not look like a valid upload")
	}
}

func TestImageInfoLabelAndCheck(t *testing.T) {
	if got := Image("Photo").Info().Label; got != "Photo" {
		t.Errorf("label: %q", got)
	}
	if err := Image("Photo").Check(tString); err != nil {
		t.Error(err)
	}
	if err := Image("Photo").Check(tInt); err == nil {
		t.Error("Image on a non-string field must be rejected")
	}
}

func TestImageRenderShowsCurrentPreviewAndEscapesIt(t *testing.T) {
	out := render(t, Image("Photo"), Value{Raw: `/uploads/"><script>x</script>.png`})
	if strings.Contains(out, "<script>x</script>") {
		t.Fatal("current image URL was not escaped")
	}
	if !strings.Contains(out, `type="file"`) {
		t.Error("missing the file input")
	}
	empty := render(t, Image("Photo"), Value{})
	if strings.Contains(empty, "<img") {
		t.Error("no preview expected without a current value")
	}
}

func TestImageIsAFileField(t *testing.T) {
	var _ FileField = Image("Photo")
}
