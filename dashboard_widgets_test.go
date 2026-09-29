package tellus

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TechnoVizor/tellus/dashboard"
)

// newWidgetDashboardHarness mounts a panel with Dashboard(widgets...)
// already called, the same way newHarness (helpers_test.go) mounts one
// with Register(regs...). A separate helper, not an addition to
// newHarness: Dashboard and Register are independent calls, and every
// other test in this package needs only Register. regs is optional, for
// tests that need a real resource registered alongside the widgets (for
// example to prove Dashboard(...) really replaces that resource's
// automatic card rather than the test just having nothing to replace).
func newWidgetDashboardHarness(t *testing.T, regs []Registrable, widgets ...dashboard.Widget) *harness {
	t.Helper()
	cfg := Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret), InsecureCookies: true}
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Register(regs...); err != nil {
		t.Fatal(err)
	}
	if err := p.Dashboard(widgets...); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	p.Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
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

func TestDashboardWidgetsReplaceAutomaticCards(t *testing.T) {
	prod := productResource(newMemSource(1))
	h := newWidgetDashboardHarness(t, []Registrable{prod}, dashboard.Stat("Email Sent").Value(func(context.Context) (string, error) {
		return "1,251 Mail", nil
	}))
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "Email Sent", "1,251 Mail", `aria-current="page">Dashboard`)
	if strings.Contains(res.body, `class="muted">Products<`) {
		t.Errorf("the registered resource's automatic card must not render once Dashboard(...) is called: %s", res.body)
	}
}

func TestDashboardNavShowsDashboardLinkWithOnlyWidgetsRegistered(t *testing.T) {
	h := newWidgetDashboardHarness(t, nil, dashboard.Stat("X").Value(func(context.Context) (string, error) {
		return "1", nil
	}))
	h.loginAdmin()
	body := h.get("/").body
	if !strings.Contains(body, `aria-current="page">Dashboard`) {
		t.Errorf("the Dashboard nav link must show even when Dashboard(...) is the only thing configured, no Register(...): %s", body)
	}
}

func TestDashboardMethodRejectsMisconfiguredStat(t *testing.T) {
	p, err := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err != nil {
		t.Fatal(err)
	}
	err = p.Dashboard(dashboard.Stat("Broken"))
	if err == nil || !strings.Contains(err.Error(), "Broken") {
		t.Errorf("expected an error naming the widget, got %v", err)
	}
}

func TestDashboardWidgetValueErrorIsServerError(t *testing.T) {
	h := newWidgetDashboardHarness(t, nil, dashboard.Stat("Broken").Value(func(context.Context) (string, error) {
		return "", errBoom
	}))
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusInternalServerError {
		t.Fatalf("status %d", res.status)
	}
	if strings.Contains(res.body, "boom") || strings.Contains(res.body, "secret") {
		t.Error("internal error text leaked to the client")
	}
}
