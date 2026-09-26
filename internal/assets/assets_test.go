package assets

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestServesCSSAndJSWithExplicitTypes(t *testing.T) {
	css := get(t, "/app.css")
	if css.Code != http.StatusOK || !strings.HasPrefix(css.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("css: %d %q", css.Code, css.Header().Get("Content-Type"))
	}
	if !strings.Contains(css.Body.String(), ":root") {
		t.Error("css body looks wrong")
	}
	js := get(t, "/app.js")
	if js.Code != http.StatusOK || !strings.HasPrefix(js.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("js: %d %q", js.Code, js.Header().Get("Content-Type"))
	}
	if js.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
}

func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}

func TestCachingDependsOnVersionParam(t *testing.T) {
	versioned := get(t, "/app.css?v="+Version())
	if cc := versioned.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("versioned URL should be immutable, got %q", cc)
	}
	stale := get(t, "/app.css?v=old")
	if cc := stale.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("unversioned URL should be no-cache, got %q", cc)
	}
}

func TestFontsAreServedImmutableWithFontType(t *testing.T) {
	rec := get(t, "/fonts/inter-latin.woff2")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/woff2" {
		t.Fatalf("font: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("fonts should be immutable, got %q", cc)
	}
	if rec.Body.Len() < 10_000 {
		t.Errorf("font looks truncated: %d bytes", rec.Body.Len())
	}
}

func TestETagRevalidation(t *testing.T) {
	first := get(t, "/app.css")
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	req := httptest.NewRequest(http.MethodGet, "/app.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("matching If-None-Match should give 304, got %d", rec.Code)
	}
}

func TestNoDirectoryListingAndUnknownFile(t *testing.T) {
	if rec := get(t, "/"); rec.Code != http.StatusNotFound {
		t.Errorf("directory listing: %d", rec.Code)
	}
	if rec := get(t, "/nope.css"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown file: %d", rec.Code)
	}
}

func TestVersionIsShortAndStable(t *testing.T) {
	if len(Version()) != 10 || Version() != computeVersion() {
		t.Errorf("unexpected version %q", Version())
	}
}
