package tellus

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// stubResource is a Registrable that does nothing, so panel behavior can be
// tested without a real resource.
type stubResource struct{ err error }

func (s stubResource) register(*Panel) error { return s.err }

func TestNewValidatesConfig(t *testing.T) {
	good := Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)}

	cases := map[string]func(*Config){
		"missing authenticator": func(c *Config) { c.Authenticator = nil },
		"short secret":          func(c *Config) { c.SessionSecret = []byte("short") },
		"prefix with query":     func(c *Config) { c.Prefix = "/admin?x=1" },
		"prefix with space":     func(c *Config) { c.Prefix = "/ad min" },
		"prefix with dotdot":    func(c *Config) { c.Prefix = "/../admin" },
		"prefix with dot":       func(c *Config) { c.Prefix = "/./admin" },
		"prefix double slash":   func(c *Config) { c.Prefix = "/a//b" },
	}
	for name, mutate := range cases {
		cfg := good
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	p, err := New(good)
	if err != nil || p.Prefix() != "/admin" {
		t.Fatalf("defaults: prefix=%q err=%v", p.Prefix(), err)
	}
	for in, want := range map[string]string{"panel": "/panel", "/x7f3/": "/x7f3", "/a/b": "/a/b"} {
		cfg := good
		cfg.Prefix = in
		p, err := New(cfg)
		if err != nil || p.Prefix() != want {
			t.Errorf("prefix %q: got %q err=%v, want %q", in, p.Prefix(), err, want)
		}
	}
}

func TestRegisterOrdering(t *testing.T) {
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err := p.Register(stubResource{}); err != nil {
		t.Fatalf("Register before Handler: %v", err)
	}
	if err := p.Register(stubResource{err: errors.New("bad resource")}); err == nil {
		t.Error("an error from a resource must be returned")
	}
	p.Handler()
	if err := p.Register(stubResource{}); err == nil {
		t.Error("Register after Handler must fail")
	}
}

func TestUnauthenticatedRequestsGoToLogin(t *testing.T) {
	h := newHarness(t, Config{})

	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/admin/login" {
		t.Fatalf("home: %d %q", res.status, res.location())
	}
	// An htmx request must not get a redirect that would swap the login page
	// into a region of the current page.
	res = h.get("/", "HX-Request", "true")
	if res.status != http.StatusUnauthorized || res.header.Get("HX-Redirect") != "/admin/login" {
		t.Fatalf("htmx: %d HX-Redirect=%q", res.status, res.header.Get("HX-Redirect"))
	}
}

func TestLoginPageAndSecurityHeaders(t *testing.T) {
	h := newHarness(t, Config{Name: "Acme Admin"})
	res := h.get("/login")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	for _, want := range []string{"Acme Admin", `name="email"`, `name="password"`, `name="_csrf"`, "/admin/assets/app.css?v="} {
		if !strings.Contains(res.body, want) {
			t.Errorf("login page missing %q", want)
		}
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy":         "same-origin",
		"Cache-Control":           "no-store",
	} {
		if got := res.header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestAssetsAreServedWithoutLogin(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.get("/assets/app.css")
	if res.status != http.StatusOK || !strings.Contains(res.body, ":root") {
		t.Fatalf("app.css: %d", res.status)
	}
	if !strings.HasPrefix(res.header.Get("Content-Type"), "text/css") {
		t.Errorf("content type %q", res.header.Get("Content-Type"))
	}
}

// The assets handler sees the path without its leading slash once the panel
// strips "/assets/", so caching rules must hold through the real mount too.
func TestFontsAreCachedImmutablyThroughThePanel(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.get("/assets/fonts/inter-latin.woff2")
	if res.status != http.StatusOK || res.header.Get("Content-Type") != "font/woff2" {
		t.Fatalf("font: %d %q", res.status, res.header.Get("Content-Type"))
	}
	if cc := res.header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("fonts should be immutable through the panel mount, got %q", cc)
	}
}

func TestLoginSuccessAndSessionCookie(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.login("Admin@Example.com", "correct horse")
	if res.status != http.StatusSeeOther || res.location() != "/admin/" {
		t.Fatalf("login: %d %q", res.status, res.location())
	}
	if h.sessionCookie() == nil {
		t.Fatal("no session cookie")
	}
	if res = h.get("/"); res.status != http.StatusOK || !strings.Contains(res.body, "Ada") {
		t.Fatalf("home after login: %d", res.status)
	}
	// The login page is pointless once signed in.
	if res = h.get("/login"); res.status != http.StatusSeeOther {
		t.Errorf("signed-in user on /login: %d", res.status)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	// Secure by default: drive the handler directly so the http test server does
	// not matter.
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret), Prefix: "/x7f3"})
	h := p.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x7f3/login", nil))
	var csrf *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "tellus_csrf" {
			csrf = c
		}
	}
	if csrf == nil || !csrf.Secure || !csrf.HttpOnly || csrf.Path != "/x7f3" {
		t.Fatalf("csrf cookie: %+v", csrf)
	}

	form := url.Values{"email": {"admin@example.com"}, "password": {"correct horse"}, "_csrf": {csrf.Value}}
	req := httptest.NewRequest(http.MethodPost, "/x7f3/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "tellus_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatalf("no session cookie, status %d", rec.Code)
	}
	if !session.Secure || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Path != "/x7f3" {
		t.Errorf("session cookie attributes: %+v", session)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	h := newHarness(t, Config{})
	wrong := h.login("admin@example.com", "wrong")
	unknown := h.login("nobody@example.com", "correct horse")
	noAccess := h.login("guest@example.com", "correct horse")

	for name, res := range map[string]result{"wrong password": wrong, "unknown email": unknown, "no panel access": noAccess} {
		if res.status != http.StatusUnauthorized {
			t.Errorf("%s: status %d", name, res.status)
		}
		if !strings.Contains(res.body, "Invalid email or password.") {
			t.Errorf("%s: missing the generic message", name)
		}
		if h.sessionCookie() != nil {
			t.Fatalf("%s: a session was issued", name)
		}
	}
}

func TestLoginKeepsEmailAndEscapesIt(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.login(`"><script>alert(1)</script>@x.co`, "nope")
	if strings.Contains(res.body, "<script>alert(1)</script>") {
		t.Fatal("email was reflected without escaping")
	}
}

func TestLoginRequiresCSRFToken(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.postRaw("/login", url.Values{"email": {"admin@example.com"}, "password": {"correct horse"}})
	if res.status != http.StatusForbidden {
		t.Fatalf("login without CSRF token: %d", res.status)
	}
	if h.sessionCookie() != nil {
		t.Fatal("session issued without a CSRF token")
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	h := newHarness(t, Config{})
	for i := 0; i < loginAttempts; i++ {
		if res := h.login("admin@example.com", "wrong"); res.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, res.status)
		}
	}
	res := h.login("admin@example.com", "correct horse")
	if res.status != http.StatusTooManyRequests || !strings.Contains(res.body, "Too many attempts") {
		t.Fatalf("attempt over the limit: %d", res.status)
	}
	if h.sessionCookie() != nil {
		t.Fatal("a blocked attempt must not sign in, even with the right password")
	}
}

func TestSessionExpiresAndRejectsTampering(t *testing.T) {
	h := newHarness(t, Config{SessionTTL: time.Hour})
	h.loginAdmin()
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatalf("signed-in home: %d", res.status)
	}

	// Tampered cookie value.
	u, _ := url.Parse(h.base)
	c := h.sessionCookie()
	forged := &http.Cookie{Name: c.Name, Value: "A" + c.Value[1:], Path: "/admin"}
	h.client.Jar.SetCookies(u, []*http.Cookie{forged})
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("tampered cookie accepted: %d", res.status)
	}

	// Expired session.
	h.client.Jar.SetCookies(u, []*http.Cookie{{Name: c.Name, Value: c.Value, Path: "/admin"}})
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatalf("restored cookie should work: %d", res.status)
	}
	h.panel.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("expired session accepted: %d", res.status)
	}
}

func TestDeletedAccountLosesAccess(t *testing.T) {
	auth := newMemAuth()
	h := newHarness(t, Config{Authenticator: auth})
	h.loginAdmin()
	delete(auth.accounts, "admin@example.com")
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("deleted account still has access: %d", res.status)
	}
}

func TestAccountWithoutPanelAccessIsForbidden(t *testing.T) {
	auth := newMemAuth()
	h := newHarness(t, Config{Authenticator: auth})
	h.loginAdmin()
	acc := auth.accounts["admin@example.com"]
	acc.user.canAccess = false
	auth.accounts["admin@example.com"] = acc
	if res := h.get("/"); res.status != http.StatusForbidden {
		t.Errorf("revoked access: %d", res.status)
	}
}

func TestLogoutNeedsCSRFAndEndsSession(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()

	if res := h.postRaw("/logout", url.Values{}); res.status != http.StatusForbidden {
		t.Fatalf("logout without CSRF token: %d", res.status)
	}
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatal("a rejected logout must not end the session")
	}

	res := h.post("/logout", nil)
	if res.status != http.StatusSeeOther || res.location() != "/admin/login" {
		t.Fatalf("logout: %d %q", res.status, res.location())
	}
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("still signed in after logout: %d", res.status)
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	h := newHarness(t, Config{})
	big := url.Values{
		"email":    {"a@b.co"},
		"password": {strings.Repeat("x", 10<<20)}, // over the 8 MiB default
		"_csrf":    {h.csrfToken()},
	}
	req, _ := http.NewRequest(http.MethodPost, h.base+"/login", strings.NewReader(big.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	// A transport error also counts as a rejection: the server may reset the
	// connection while the client is still sending, and Windows reports that as
	// a send error instead of the 4xx response.
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("2 MiB body accepted: %d", resp.StatusCode)
		}
	}
	if h.sessionCookie() != nil {
		t.Fatal("session issued for an oversized body")
	}
}

func TestCustomPrefixMovesEverything(t *testing.T) {
	h := newHarness(t, Config{Prefix: "/secret-x7f3"})
	if !strings.HasSuffix(h.base, "/secret-x7f3") {
		t.Fatalf("base %s", h.base)
	}
	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/secret-x7f3/login" {
		t.Fatalf("redirect: %d %q", res.status, res.location())
	}
	page := h.get("/login")
	if !strings.Contains(page.body, "/secret-x7f3/assets/app.css") || strings.Contains(page.body, `"/admin/`) {
		t.Errorf("links do not follow the prefix: %s", page.body)
	}
	// The default path must not answer.
	resp, err := h.client.Get(h.srv.URL + "/admin/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/admin still answers: %d", resp.StatusCode)
	}
}

func TestHomeWithoutResources(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK || !strings.Contains(res.body, "No resources are registered yet.") {
		t.Fatalf("empty home: %d", res.status)
	}
}
