package secure

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func csrfHandler(seen *string) http.Handler {
	return CSRF{Path: "/admin"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = CSRFToken(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
}

func issueToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookie {
			return c.Value
		}
	}
	t.Fatal("no CSRF cookie issued on GET")
	return ""
}

func TestCSRFIssuesCookieOnGet(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("cookie not set")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/admin" {
		t.Fatalf("bad cookie attributes: %+v", cookie)
	}
	if seen != cookie.Value {
		t.Fatalf("context token %q != cookie %q", seen, cookie.Value)
	}
}

func TestCSRFKeepsExistingToken(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	token := issueToken(t, h)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("cookie was re-issued although a valid one was sent")
	}
	if seen != token {
		t.Fatal("token in context differs from cookie")
	}
}

func TestCSRFRejectsUnsafeRequests(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	token := issueToken(t, h)
	other := strings.Repeat("a", 64)

	form := func(tok string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{CSRFField: {tok}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	withCookie := func(r *http.Request, v string) *http.Request {
		r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: v})
		return r
	}
	noToken := httptest.NewRequest(http.MethodPost, "/", nil)
	header := httptest.NewRequest(http.MethodDelete, "/", nil)
	header.Header.Set(CSRFHeader, token)

	cases := []struct {
		name string
		req  *http.Request
		want int
	}{
		{"no cookie no token", noToken, http.StatusForbidden},
		{"cookie but no token", withCookie(httptest.NewRequest(http.MethodPost, "/", nil), token), http.StatusForbidden},
		{"token but no cookie", form(token), http.StatusForbidden},
		{"mismatch", withCookie(form(other), token), http.StatusForbidden},
		{"valid form field", withCookie(form(token), token), http.StatusNoContent},
		{"valid header", withCookie(header, token), http.StatusNoContent},
		{"malformed cookie", withCookie(form("short"), "short"), http.StatusForbidden},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, tc.req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}
