# Dashboard Widgets, Part 1: Widget API and Stat Cards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `dashboard.Widget` API and one concrete widget, `dashboard.Stat` (icon, host-computed value, optional host-computed trend, optional link), plus `Panel.Dashboard(widgets ...dashboard.Widget) error`, which fully replaces the automatic per-resource dashboard when called.

**Architecture:** A new top-level `dashboard` package (sibling to `form` and `table`) owns the `Widget`/`Validated` interfaces, the concrete `StatWidget` type, and its own templ rendering, the same split `form` and `table` already use for fields and columns. `Panel` gains a second, independent dashboard data path (`dashboardWidgets`) alongside the existing automatic one (`dashboardCards`); `home()` branches on which one is populated, defaulting to the untouched automatic behavior when `Dashboard(...)` is never called.

**Tech Stack:** Go 1.26+, templ v0.3.1020 (unchanged from prior plans).

**Spec:** `docs/superpowers/specs/2026-09-29-dashboard-widgets-design.md` (the plan argues from it; read both).

## Global Constraints

- English only UI: any new user-facing string goes through `i18n.T("key")`, the key must exist in `internal/i18n/en.go`. This plan adds two keys, `dashboard.trend_up` and `dashboard.trend_down`.
- No em-dashes anywhere: not in code, comments, docs, copy or commit messages.
- Generated `*_templ.go` files are committed and regenerated with `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate` after every `.templ` change, before running tests.
- `Stat`'s `Value`/`Trend` functions are host-written closures with no assumption they touch a database at all; none of this plan's tests need `TELLUS_TEST_DSN` or real Postgres. Say so explicitly in each task rather than defaulting to the project's usual Postgres-backed test setup.
- The widget-validation interface (`dashboard.Validated`, with an exported `Validate() error` method) must be declared inside the `dashboard` package and exported, the same shape `form.FileField` (`form/file.go`) already uses. An unexported method in a root-package interface can never be satisfied by a method the `dashboard` package defines: Go scopes unexported interface method names per package, so that pairing would compile but the type assertion would silently always fail.
- `Panel.Dashboard(...)` must be callable before `Handler()`/`Mount()`, the same rule `Register` already follows.
- Commit messages follow Conventional Commits, no em-dashes.

## Review Focus

1. A `Stat`'s icon must render unescaped (it is host-authored SVG, trusted verbatim) while its label and value must still render escaped (they can carry arbitrary runtime data): mixing these up in either direction is the single most likely bug in this widget (Task 1, `TestStatRenderIconIsNotEscaped`, `TestStatRenderEscapesLabelAndValue`).
2. A `Stat` with no `Value()` must be rejected when `Dashboard(...)` is called, not render blank or panic at the first real request (Task 1's `Validate`, Task 2's `TestDashboardMethodRejectsMisconfiguredStat`).
3. A chevron must render only when `Href` was set, never on a plain stat card (Task 1, `TestStatRenderChevronOnlyWithHref`).
4. Calling `Dashboard(...)` must fully replace the automatic per-resource cards; a panel that never calls it must render exactly as the already-shipped automatic dashboard, unchanged (Task 2, `TestDashboardWidgetsReplaceAutomaticCards` plus the existing suite's automatic-dashboard tests staying green).
5. A `Value` or `Trend` function's error must surface as the generic 500 page, never raw error text (Task 2, `TestDashboardWidgetValueErrorIsServerError`).

## Deliberately not in this plan

The donut chart widget, the embedded data table widget (both separate, later specs), a built-in icon set, automatic trend computation from a resource's own data, a `dashboard.ResourceStat[T]` convenience constructor, widget layout/ordering configuration beyond registration order, and any per-user or per-role dashboard authorization. All deferred in `docs/superpowers/specs/2026-09-29-dashboard-widgets-design.md` section 9.

---

### Task 1: The dashboard package: Widget, Validated, and Stat

A self-contained, unit-tested package: the `Widget`/`Validated` interfaces, the `StatWidget` builder, and its own rendering. No `Panel` involvement yet, nothing here touches HTTP or the rest of the library.

**Files:**
- Create: `dashboard/widget.go`
- Create: `dashboard/stat.go`
- Create: `dashboard/render.templ`
- Create: `dashboard/render_templ.go` (regenerated, not hand-edited)
- Test: `dashboard/stat_test.go`

**Interfaces:**
- Consumes: `github.com/a-h/templ` (already a dependency), `github.com/TechnoVizor/tellus/internal/i18n` (already used the same way by `form`/`table`).
- Produces:
  - `type Widget interface { Render(ctx context.Context) (templ.Component, error) }`
  - `type Validated interface { Widget; Validate() error }`
  - `func Stat(label string) *StatWidget`
  - `(*StatWidget) Icon(svg string) *StatWidget`
  - `(*StatWidget) Value(fn func(ctx context.Context) (string, error)) *StatWidget`
  - `(*StatWidget) Trend(fn func(ctx context.Context) (percent float64, up bool, err error)) *StatWidget`
  - `(*StatWidget) Href(url string) *StatWidget`
  - `(*StatWidget) Validate() error`
  - `(*StatWidget) Render(ctx context.Context) (templ.Component, error)`, satisfying `Validated` (and therefore `Widget`)

- [ ] **Step 1: Write the failing tests**

Create `dashboard/stat_test.go`:

```go
package dashboard

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func render(t *testing.T, w Widget) string {
	t.Helper()
	c, err := w.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestStatValidateRequiresValue(t *testing.T) {
	if err := Stat("Broken").Validate(); err == nil {
		t.Error("Stat without Value must be rejected")
	}
	if err := Stat("OK").Value(func(context.Context) (string, error) { return "1", nil }).Validate(); err != nil {
		t.Error(err)
	}
}

func TestStatValidateErrorNamesTheWidget(t *testing.T) {
	err := Stat("Email Sent").Validate()
	if err == nil || !strings.Contains(err.Error(), "Email Sent") {
		t.Errorf("Validate() = %v, want an error naming the widget", err)
	}
}

func TestStatRenderPropagatesValueError(t *testing.T) {
	boom := errors.New("boom")
	w := Stat("X").Value(func(context.Context) (string, error) { return "", boom })
	_, err := w.Render(context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("Render error = %v, want %v", err, boom)
	}
}

func TestStatRenderPropagatesTrendError(t *testing.T) {
	boom := errors.New("boom")
	w := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return 0, false, boom })
	_, err := w.Render(context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("Render error = %v, want %v", err, boom)
	}
}

func TestStatRenderIconIsNotEscaped(t *testing.T) {
	w := Stat("X").
		Icon(`<svg data-test="icon"><path d="M0 0"></path></svg>`).
		Value(func(context.Context) (string, error) { return "1", nil })
	out := render(t, w)
	if !strings.Contains(out, `<svg data-test="icon">`) {
		t.Errorf("icon markup was escaped or dropped: %s", out)
	}
}

func TestStatRenderEscapesLabelAndValue(t *testing.T) {
	w := Stat(`<script>alert(1)</script>`).
		Value(func(context.Context) (string, error) { return `<script>alert(2)</script>`, nil })
	out := render(t, w)
	if strings.Contains(out, "<script>") {
		t.Fatalf("label or value was not escaped: %s", out)
	}
}

func TestStatRenderTrendClassAndText(t *testing.T) {
	up := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return 12, true, nil })
	out := render(t, up)
	if !strings.Contains(out, "trend-up") || !strings.Contains(out, "12%") {
		t.Errorf("up trend render: %s", out)
	}
	down := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return 5, false, nil })
	out = render(t, down)
	if !strings.Contains(out, "trend-down") || !strings.Contains(out, "5%") {
		t.Errorf("down trend render: %s", out)
	}
}

func TestStatRenderNoTrendWhenTrendNotSet(t *testing.T) {
	w := Stat("X").Value(func(context.Context) (string, error) { return "1", nil })
	out := render(t, w)
	if strings.Contains(out, "trend-up") || strings.Contains(out, "trend-down") {
		t.Errorf("no Trend() call must mean no trend markup: %s", out)
	}
}

func TestStatRenderChevronOnlyWithHref(t *testing.T) {
	plain := Stat("X").Value(func(context.Context) (string, error) { return "1", nil })
	if strings.Contains(render(t, plain), "stat-chevron") {
		t.Error("a Stat without Href must not render a chevron")
	}
	linked := Stat("X").Value(func(context.Context) (string, error) { return "1", nil }).Href("/somewhere")
	out := render(t, linked)
	if !strings.Contains(out, "stat-chevron") || !strings.Contains(out, `href="/somewhere"`) {
		t.Errorf("a Stat with Href must render a chevron and link: %s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./dashboard/... 2>&1 || true` (the directory does not exist yet, so this fails with "no such file or directory" or a build error once the test file exists with no package to compile against)
Expected: FAIL, `undefined: Widget`, `undefined: Stat`, and related undefined symbols once `dashboard/widget.go`/`dashboard/stat.go` are absent.

- [ ] **Step 3: Create dashboard/widget.go**

```go
// Package dashboard holds the widget builders used to describe the panel's
// dashboard home page. Custom widgets implement Widget.
package dashboard

import (
	"context"

	"github.com/a-h/templ"
)

// Widget is one piece of the dashboard's home page. A host builds a
// dashboard from a list of these; Stat is the only implementation in this
// phase, more follow in later work.
type Widget interface {
	// Render draws the widget for one request. ctx carries the request's
	// context, so a widget's own data-fetching functions can use it the
	// same way a DataSource already does.
	Render(ctx context.Context) (templ.Component, error)
}

// Validated is implemented by a widget with configuration Dashboard checks
// before the panel starts, the same "reported at startup" guarantee the
// rest of this library already gives (a typo in a Table/Form field name is
// caught the same way). Stat implements it.
type Validated interface {
	Widget
	Validate() error
}
```

- [ ] **Step 4: Create dashboard/stat.go**

```go
package dashboard

import (
	"context"
	"fmt"
	"strconv"
)

// StatWidget is a single number, optionally with an icon, a trend, and a
// link. The label, icon, and href are fixed once built; the value and
// trend are resolved fresh on every request through host-supplied
// functions, with no assumption either one is backed by a Tellus Resource.
type StatWidget struct {
	label   string
	icon    string
	href    string
	valueFn func(ctx context.Context) (string, error)
	trendFn func(ctx context.Context) (percent float64, up bool, err error)
}

// trendValue is Trend's resolved, display-ready result: the sign is baked
// into text (for example "12%"), up says which arrow and color to use.
type trendValue struct {
	up   bool
	text string
}

// Stat returns a stat card widget with the given label.
func Stat(label string) *StatWidget {
	return &StatWidget{label: label}
}

// Icon sets the widget's icon to raw SVG markup, rendered verbatim. This is
// code the host writes, the same trust boundary as every other builder
// call in this library, never end-user or database input.
func (w *StatWidget) Icon(svg string) *StatWidget { w.icon = svg; return w }

// Value sets the function that computes the card's displayed value on each
// request. Required.
func (w *StatWidget) Value(fn func(ctx context.Context) (string, error)) *StatWidget {
	w.valueFn = fn
	return w
}

// Trend sets the function that computes the card's trend badge on each
// request. Optional: a Stat with no Trend renders no badge at all.
func (w *StatWidget) Trend(fn func(ctx context.Context) (percent float64, up bool, err error)) *StatWidget {
	w.trendFn = fn
	return w
}

// Href makes the whole card a link, with a trailing chevron. Optional: a
// Stat with no Href renders as a plain, non-linked card.
func (w *StatWidget) Href(url string) *StatWidget { w.href = url; return w }

func (w *StatWidget) Validate() error {
	if w.valueFn == nil {
		return fmt.Errorf("dashboard.Stat(%q) needs .Value(...)", w.label)
	}
	return nil
}

func (w *StatWidget) Render(ctx context.Context) (templ.Component, error) {
	value, err := w.valueFn(ctx)
	if err != nil {
		return nil, err
	}
	var trend *trendValue
	if w.trendFn != nil {
		percent, up, err := w.trendFn(ctx)
		if err != nil {
			return nil, err
		}
		trend = &trendValue{up: up, text: strconv.FormatFloat(percent, 'f', -1, 64) + "%"}
	}
	return statWidget(w, value, trend), nil
}
```

`Value` no longer needs a separate "was it set" flag: `w.valueFn == nil` already answers that, one field instead of two.

`Render`'s signature needs `templ.Component` in scope; add `"github.com/a-h/templ"` to this file's imports alongside `"context"`, `"fmt"`, `"strconv"`.

- [ ] **Step 5: Create dashboard/render.templ**

```templ
package dashboard

import "github.com/TechnoVizor/tellus/internal/i18n"

templ statWidget(w *StatWidget, value string, trend *trendValue) {
	if w.href != "" {
		<a class="card dashboard-card" href={ templ.SafeURL(w.href) }>
			@statBody(w, value, trend)
			<span class="stat-chevron" aria-hidden="true">›</span>
		</a>
	} else {
		<div class="card dashboard-card">
			@statBody(w, value, trend)
		</div>
	}
}

templ statBody(w *StatWidget, value string, trend *trendValue) {
	<div class="card-body">
		if w.icon != "" {
			<span class="stat-icon" aria-hidden="true">@templ.Raw(w.icon)</span>
		}
		<div class="muted">{ w.label }</div>
		<div class="stat-value">{ value }</div>
		if trend != nil {
			<span class={ "trend", templ.KV("trend-up", trend.up), templ.KV("trend-down", !trend.up) }>
				<span aria-hidden="true">
					if trend.up {
						▲
					} else {
						▼
					}
				</span>
				{ trend.text }
				<span class="sr-only">
					if trend.up {
						{ i18n.T("dashboard.trend_up") }
					} else {
						{ i18n.T("dashboard.trend_down") }
					}
				</span>
			</span>
		}
	</div>
}
```

`templ.Raw(w.icon)` is the one place in this widget that intentionally
bypasses templ's usual escaping: the icon string is Go code the host wrote,
never request or database data. Do not "fix" this into `{ w.icon }` later,
that would print literal angle brackets instead of an icon.

`i18n.T("dashboard.trend_up")` and `i18n.T("dashboard.trend_down")` do not
exist yet, added in Task 2 alongside the rest of that task's `en.go` change;
`templ generate` in the next step does not need the keys to exist, only
`go build`/`go test` do, and Task 2 adds them before this package is
exercised end to end. If `go vet`/`go build` on this package alone fails
before Task 2 because of `internal/i18n/keys_test.go` (which scans the
whole repository, not just this package), that test is in a different
package and does not block this task's own `go test ./dashboard/...`; if it
does surface an ordering problem in practice, add the two `en.go` keys here
in this task instead of waiting for Task 2, and skip re-adding them there.

- [ ] **Step 6: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test ./dashboard/... -v`
Expected: PASS (all ten tests). If `templ generate` reports a syntax error
on the `class={ "trend", templ.KV(...), templ.KV(...) }` expression or the
inline `if trend.up { ▲ } else { ▼ }` block, fix the syntax to whatever
templ v0.3.1020 actually accepts (for example, splitting the arrow onto its
own line inside each branch, or wrapping the glyph in a `<span>` per
branch) rather than abandoning the approach; the visual result (an
aria-hidden arrow glyph, a visible percent, a screen-reader-only direction
word) is the requirement, not the exact markup shape above.

- [ ] **Step 7: Run gofmt and vet**

Run: `gofmt -l dashboard/ && go vet ./dashboard/...`
Expected: no output, no errors.

- [ ] **Step 8: Commit**

```bash
git add dashboard/widget.go dashboard/stat.go dashboard/render.templ dashboard/render_templ.go dashboard/stat_test.go
git commit -m "feat: add the dashboard package with a Widget API and Stat cards"
```

---

### Task 2: Wire widgets into the panel and prove it end to end

`Panel.Dashboard(...)` validates and stores widgets; `home()` renders them
when present, falling back to the untouched automatic dashboard otherwise;
new CSS and i18n; proven through the HTTP harness, no database needed.

**Files:**
- Modify: `panel.go`
- Modify: `internal/ui/views.go`
- Modify: `internal/ui/dashboard.templ`
- Modify: `internal/ui/dashboard_templ.go` (regenerated, not hand-edited)
- Modify: `internal/assets/static/app.css`
- Modify: `internal/i18n/en.go`
- Test: `dashboard_widgets_test.go` (new file, root package; kept separate
  from the existing `dashboard_test.go`, which covers the automatic
  dashboard's own data layer, a different concern)

**Interfaces:**
- Consumes: `dashboard.Widget`, `dashboard.Validated`, `dashboard.Stat` (Task 1); `p.render`, `p.serverError`, `p.renderMessage`, `p.shell` (already in `panel.go`); `errBoom`, `newMemAuth`, `testSecret`, `retryDial`, `harness` (already in `helpers_test.go`/`resource_helpers_test.go`).
- Produces:
  - `Panel.dashboardWidgets []dashboard.Widget`
  - `func (p *Panel) Dashboard(widgets ...dashboard.Widget) error`
  - `ui.WidgetDashboardView{Shell, Heading, Widgets []templ.Component}`
  - `ui.WidgetDashboardPage(v WidgetDashboardView) templ.Component`

- [ ] **Step 1: Write the failing tests**

Create `dashboard_widgets_test.go`:

```go
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
```

None of these three tests need `TELLUS_TEST_DSN`: `newWidgetDashboardHarness` never calls `Register`, so no `DataSource`/GORM is involved at all.

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test . -run TestDashboardWidgets`
Expected: FAIL, `undefined: dashboard`, `p.Dashboard undefined`, and related undefined symbols (`Panel.Dashboard` does not exist yet).

- [ ] **Step 3: Add dashboardWidgets and Dashboard to panel.go**

The `Panel` struct currently reads:

```go
	mu       sync.Mutex
	built    bool
	handler  http.Handler
	nav      []navEntry
	slugs    map[string]bool
	mounters []func(*http.ServeMux)
	dashboardCards []dashboardCard
}
```

Add `dashboardWidgets`:

```go
	mu               sync.Mutex
	built            bool
	handler          http.Handler
	nav              []navEntry
	slugs            map[string]bool
	mounters         []func(*http.ServeMux)
	dashboardCards   []dashboardCard
	dashboardWidgets []dashboard.Widget
}
```

Add the import, alongside the existing ones at the top of `panel.go`:

```go
	"github.com/TechnoVizor/tellus/dashboard"
```

Add a new method, right after `Register`:

```go
// Dashboard sets the dashboard home page's widgets, in the order given.
// Calling it at all replaces the automatic per-resource dashboard that
// otherwise renders on its own; a panel that never calls Dashboard keeps
// that automatic behavior. Call it before Handler, the same rule Register
// already follows.
func (p *Panel) Dashboard(widgets ...dashboard.Widget) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.built {
		return errors.New("tellus: Dashboard must be called before Handler")
	}
	for _, w := range widgets {
		if vw, ok := w.(dashboard.Validated); ok {
			if err := vw.Validate(); err != nil {
				return fmt.Errorf("tellus: %w", err)
			}
		}
	}
	p.dashboardWidgets = widgets
	return nil
}
```

(`p.mu`/`p.built` are the same guard `Register` already uses, read `Register`'s own body in `panel.go` to confirm the exact lock/built-check shape before writing this, keep it consistent rather than inventing a different guard style.) `fmt` is not yet imported in `panel.go` if it is not already there for another reason, check the current import block and add it if missing.

- [ ] **Step 4: Rewrite home() to branch on dashboardWidgets first**

`home()` currently reads:

```go
func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.dashboardCards) == 0 {
		p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
		return
	}
	cards := make([]ui.DashboardCardView, len(p.dashboardCards))
	for i, c := range p.dashboardCards {
		count, err := c.countFn(r.Context())
		if err != nil {
			p.serverError(w, err)
			return
		}
		cv := ui.DashboardCardView{Label: c.label, Href: c.href, Count: count}
		if c.seriesFn != nil {
			series, err := c.seriesFn(r.Context(), dashboardDays)
			if err != nil {
				p.serverError(w, err)
				return
			}
			cv.HasSeries = true
			cv.SeriesJSON = sparklineJSON(series, dashboardDays)
		}
		cards[i] = cv
	}
	p.render(w, r, http.StatusOK, ui.DashboardPage(ui.DashboardView{
		Shell:   p.shell(r, u, i18n.T("ui.dashboard"), ""),
		Heading: i18n.T("ui.dashboard"),
		Cards:   cards,
	}))
}
```

Change it to add a new branch before the existing one, which stays exactly
as it is:

```go
func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.dashboardWidgets) > 0 {
		comps := make([]templ.Component, len(p.dashboardWidgets))
		for i, wg := range p.dashboardWidgets {
			c, err := wg.Render(r.Context())
			if err != nil {
				p.serverError(w, err)
				return
			}
			comps[i] = c
		}
		p.render(w, r, http.StatusOK, ui.WidgetDashboardPage(ui.WidgetDashboardView{
			Shell:   p.shell(r, u, i18n.T("ui.dashboard"), ""),
			Heading: i18n.T("ui.dashboard"),
			Widgets: comps,
		}))
		return
	}
	if len(p.dashboardCards) == 0 {
		p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
		return
	}
	cards := make([]ui.DashboardCardView, len(p.dashboardCards))
	for i, c := range p.dashboardCards {
		count, err := c.countFn(r.Context())
		if err != nil {
			p.serverError(w, err)
			return
		}
		cv := ui.DashboardCardView{Label: c.label, Href: c.href, Count: count}
		if c.seriesFn != nil {
			series, err := c.seriesFn(r.Context(), dashboardDays)
			if err != nil {
				p.serverError(w, err)
				return
			}
			cv.HasSeries = true
			cv.SeriesJSON = sparklineJSON(series, dashboardDays)
		}
		cards[i] = cv
	}
	p.render(w, r, http.StatusOK, ui.DashboardPage(ui.DashboardView{
		Shell:   p.shell(r, u, i18n.T("ui.dashboard"), ""),
		Heading: i18n.T("ui.dashboard"),
		Cards:   cards,
	}))
}
```

`templ.Component` is already in scope in `panel.go` (used by `render`'s own
signature), no new import needed for that part.

- [ ] **Step 5: Add WidgetDashboardView to internal/ui/views.go**

Append, after the existing `DashboardView` struct:

```go
// WidgetDashboardView is the data of the host-configured dashboard home
// page (Panel.Dashboard). Each Widgets entry is already fully rendered by
// its own dashboard.Widget implementation; this view does no rendering of
// its own beyond placing them in the grid.
type WidgetDashboardView struct {
	Shell   Shell
	Heading string
	Widgets []templ.Component
}
```

- [ ] **Step 6: Add WidgetDashboardPage to internal/ui/dashboard.templ**

Append, after the existing `DashboardPage` templ:

```templ
templ WidgetDashboardPage(v WidgetDashboardView) {
	@Layout(v.Shell) {
		<div class="page-head">
			<h1>{ v.Heading }</h1>
		</div>
		<div class="dashboard-grid">
			for _, w := range v.Widgets {
				@w
			}
		</div>
	}
}
```

- [ ] **Step 7: Add the CSS**

In `internal/assets/static/app.css`, the tokens layer's light `:root` block
has `--danger: #c13515;` around line 56; add `--success` right after it:

```css
    --danger: #c13515;
    --success: #1a7d3a;
```

The same file has two dark-mode blocks, `@media (prefers-color-scheme:
dark) { :root:not([data-theme="light"]) { ... --danger: #f26d5b; ... } }`
around line 78, and `:root[data-theme="dark"] { ... --danger: #f26d5b; ...
}` around line 94; add `--success: #4ade80;` right after `--danger` in
both.

In the components layer, the existing dashboard rules read (around line
170):

```css
  .dashboard-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 20px; }
  .dashboard-card { display: block; text-decoration: none; transition: border-color 0.15s var(--ease); }
  .dashboard-card:hover { border-color: var(--border-strong); }
  .stat-value { font-size: 32px; font-weight: 700; letter-spacing: -0.02em; margin-top: 4px; }
  .dashboard-chart { width: 100%; height: 60px; margin-top: 12px; }
```

Change the `.dashboard-card` rule to add `position: relative` (needed for
`.stat-chevron` below; the automatic dashboard's own cards also use this
class but never have a `.stat-chevron` child, so this is a no-op for them),
and add the new rules after `.dashboard-chart`:

```css
  .dashboard-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 20px; }
  .dashboard-card { display: block; position: relative; text-decoration: none; transition: border-color 0.15s var(--ease); }
  .dashboard-card:hover { border-color: var(--border-strong); }
  .stat-value { font-size: 32px; font-weight: 700; letter-spacing: -0.02em; margin-top: 4px; }
  .dashboard-chart { width: 100%; height: 60px; margin-top: 12px; }
  .stat-icon { display: block; width: 20px; height: 20px; color: var(--ink-muted); margin-bottom: 8px; }
  .stat-icon svg { width: 100%; height: 100%; }
  .stat-chevron { position: absolute; top: 20px; right: 20px; color: var(--ink-muted); }
  .trend { display: inline-flex; align-items: center; gap: 4px; margin-top: 8px; padding: 2px 8px; border-radius: var(--radius-pill); font-size: 12px; font-weight: 600; }
  .trend-up { color: var(--success); background: color-mix(in srgb, var(--success) 12%, transparent); }
  .trend-down { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
```

- [ ] **Step 8: Add the i18n keys**

In `internal/i18n/en.go`, in the "layout" group alongside `ui.dashboard`,
add:

```go
"dashboard.trend_up":   "Trending up",
"dashboard.trend_down": "Trending down",
```

- [ ] **Step 9: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test . -run TestDashboardWidgets -v`
Expected: PASS (all three tests).

- [ ] **Step 10: Run the whole project's tests, gofmt, and vet**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, no vet errors, every test passes except the one
pre-existing unrelated failure, `TestLocalStorageRejectsFilenamesThatEscapeTheDirectory`
(Windows-path handling on Linux); this task does not touch it and does not
need to fix it. Confirm specifically that `TestHomeRendersDashboard` and
the rest of the automatic dashboard's existing tests are still green
unchanged: that is this task's proof that a panel which never calls
`Dashboard(...)` still behaves exactly as already shipped.

- [ ] **Step 11: Commit**

```bash
git add panel.go internal/ui/views.go internal/ui/dashboard.templ internal/ui/dashboard_templ.go internal/assets/static/app.css internal/i18n/en.go dashboard_widgets_test.go
git commit -m "feat: add Panel.Dashboard, replacing the automatic dashboard when used"
```

---

## After this plan

The donut chart widget and the embedded data table widget are separate,
later specs, both building on the `dashboard.Widget` interface this plan
introduces. `examples/shop` is not touched here; wiring a `Stat` or two into
it (for example, replacing or augmenting its current automatic dashboard) is
a natural follow-up once this lands, the same way Date, Money, and the
Category relation were each wired in after their own features shipped, not
part of this plan.
