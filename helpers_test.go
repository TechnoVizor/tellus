package tellus

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// --- fake authenticator ---

type memUser struct {
	id        string
	name      string
	canAccess bool
}

func (u memUser) UserID() string       { return u.id }
func (u memUser) DisplayName() string  { return u.name }
func (u memUser) CanAccessPanel() bool { return u.canAccess }

type memAccount struct {
	password string
	user     memUser
}

// memAuth is an in-memory Authenticator keyed by email.
type memAuth struct{ accounts map[string]memAccount }

func newMemAuth() *memAuth {
	return &memAuth{accounts: map[string]memAccount{
		"admin@example.com": {password: "correct horse", user: memUser{id: "1", name: "Ada", canAccess: true}},
		"guest@example.com": {password: "correct horse", user: memUser{id: "2", name: "Guest", canAccess: false}},
	}}
}

func (a *memAuth) Authenticate(_ context.Context, email, password string) (User, error) {
	acc, ok := a.accounts[strings.ToLower(email)]
	if !ok || acc.password != password {
		return nil, ErrInvalidCredentials
	}
	return acc.user, nil
}

func (a *memAuth) UserByID(_ context.Context, id string) (User, error) {
	for _, acc := range a.accounts {
		if acc.user.id == id {
			return acc.user, nil
		}
	}
	return nil, ErrUserNotFound
}

// --- HTTP harness ---

const testSecret = "0123456789abcdef0123456789abcdef"

type result struct {
	status int
	body   string
	header http.Header
}

func (r result) location() string { return r.header.Get("Location") }

type harness struct {
	t      *testing.T
	panel  *Panel
	srv    *httptest.Server
	client *http.Client
	base   string
}

// newHarness mounts a panel with the fake authenticator on a real test server.
// Redirects are not followed so tests can assert on them.
func newHarness(t *testing.T, cfg Config, regs ...Registrable) *harness {
	t.Helper()
	if cfg.Authenticator == nil {
		cfg.Authenticator = newMemAuth()
	}
	if cfg.SessionSecret == nil {
		cfg.SessionSecret = []byte(testSecret)
	}
	cfg.InsecureCookies = true // httptest serves plain http
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Register(regs...); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	p.Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	// A private transport per harness: the shared default transport could hand a
	// later test a pooled connection to a closed server that reused its port.
	transport := &http.Transport{DialContext: retryDial}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Jar:           jar,
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &harness{t: t, panel: p, srv: srv, client: client, base: srv.URL + p.Prefix()}
}

// retryDial dials like the default transport but retries a few times. On some
// Windows machines a loopback connect occasionally fails with a transient
// connectex timeout, and a quick retry gets past it.
func retryDial(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		conn, err := d.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}

func (h *harness) do(req *http.Request) result {
	h.t.Helper()
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return result{status: resp.StatusCode, body: string(body), header: resp.Header}
}

func (h *harness) get(path string, headers ...string) result {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.base+path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return h.do(req)
}

// csrfToken returns the CSRF token for this client, fetching one first if the
// jar has none.
func (h *harness) csrfToken() string {
	h.t.Helper()
	u, _ := url.Parse(h.base)
	find := func() string {
		for _, c := range h.client.Jar.Cookies(u) {
			if c.Name == "tellus_csrf" {
				return c.Value
			}
		}
		return ""
	}
	if tok := find(); tok != "" {
		return tok
	}
	h.get("/login")
	return find()
}

// post submits a form with a valid CSRF token.
func (h *harness) post(path string, v url.Values, headers ...string) result {
	h.t.Helper()
	if v == nil {
		v = url.Values{}
	}
	v.Set("_csrf", h.csrfToken())
	return h.postRaw(path, v, headers...)
}

// postRaw submits a form exactly as given.
func (h *harness) postRaw(path string, v url.Values, headers ...string) result {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.base+path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return h.do(req)
}

func (h *harness) login(email, password string) result {
	h.t.Helper()
	return h.post("/login", url.Values{"email": {email}, "password": {password}})
}

func (h *harness) loginAdmin() {
	h.t.Helper()
	if res := h.login("admin@example.com", "correct horse"); res.status != http.StatusSeeOther {
		h.t.Fatalf("admin login failed: %d %s", res.status, res.body)
	}
}

func (h *harness) sessionCookie() *http.Cookie {
	u, _ := url.Parse(h.base)
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == "tellus_session" {
			return c
		}
	}
	return nil
}
