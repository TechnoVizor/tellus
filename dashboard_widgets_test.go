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
// other test in this package needs only Register.
func newWidgetDashboardHarness(t *testing.T, widgets ...dashboard.Widget) *harness {
	t.Helper()
	cfg := Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret), InsecureCookies: true}
	p, err := New(cfg)
	if err != nil {
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
	h := newWidgetDashboardHarness(t, dashboard.Stat("Email Sent").Value(func(context.Context) (string, error) {
		return "1,251 Mail", nil
	}))
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "Email Sent", "1,251 Mail")
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
	h := newWidgetDashboardHarness(t, dashboard.Stat("Broken").Value(func(context.Context) (string, error) {
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
