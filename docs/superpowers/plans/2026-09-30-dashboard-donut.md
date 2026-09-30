# Dashboard Donut Widget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `dashboard.Donut` widget: a part-to-whole chart with up to six labeled, colored segments, a custom HTML legend, and a center total, rendered with a newly vendored Chart.js, composing into `Panel.Dashboard(...)` alongside `dashboard.Stat` with no change to `Panel` itself.

**Architecture:** `dashboard.Donut` is a second implementation of the already-shipped `dashboard.Widget`/`dashboard.Validated` interfaces, living in a new `dashboard/donut.go` sibling to `dashboard/stat.go`, with its own `render.templ` additions. `Panel.Dashboard`/`home()` (`panel.go`) already loop over `[]dashboard.Widget` generically from the `Stat` work and need no change. Chart.js is vendored the same way uPlot was: fetched once, committed, loaded on every page, driven from `app.js` by a small init function that reads a `data-segments` JSON attribute, mirroring `initDashboardCharts`.

**Tech Stack:** Go 1.26+, templ v0.3.1020, Chart.js 4.5.1 (new), Postgres not needed (this widget's tests, like Stat's, never touch a `DataSource`).

**Spec:** `docs/superpowers/specs/2026-09-30-dashboard-donut-design.md` (the plan argues from it; read both).

## Global Constraints

- English only UI: any new user-facing string goes through `i18n.T("key")`, the key must exist in `internal/i18n/en.go`. This plan adds two keys, `dashboard.other` and `dashboard.no_data`, both in Task 1 (Task 1's own tests assert on the literal English text, so the keys cannot wait for a later task the way `dashboard.trend_up`/`trend_down` once did).
- No em-dashes anywhere: not in code, comments, docs, copy, or commit messages.
- Generated `*_templ.go` files are committed and regenerated with `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate` after every `.templ` change, before running tests.
- Colors stay fixed and non-configurable: a `Donut`'s six segment colors are assigned by slot position only (`chartColors[i]`), never a per-segment host override, matching every other color decision in this library.
- The six-segment cap and its "Other" fold are enforced inside `Render`, once per request, not inside `Validate`: segment count is data the host's function returns, which can change between requests, unlike `Stat.Value` being set or not, which is fixed the moment `Dashboard(...)` is called.
- Chart.js **4.5.1** (MIT) is a new pinned, vendored dependency: `internal/assets/static/chart.umd.min.js`, license text in `THIRD_PARTY_NOTICES.md`, alongside uPlot 1.6.32, htmx 2.0.11, Alpine.js 3.17.4, and Inter Variable 5.3.0.
- Commit messages follow Conventional Commits, no em-dashes.

## Review Focus

1. A `Donut`'s legend and center total must render escaped (arbitrary host or database text through `Segment.Label`), and nothing in this widget uses `templ.Raw` the way `Stat`'s icon does, so there is no trusted-verbatim exception to get backwards here (Task 1, `TestDonutRenderEscapesLabelsAndValues`).
2. A NaN or infinite `Segment.Value` would make `segmentsJSON`'s `json.Marshal` fail silently, breaking the chart with no visible error; `resolveSegments` must drop non-finite values before they reach it, the same defensive posture `Stat` already has for `Trend`'s percent (Task 1, `TestDonutRenderDropsNonFiniteSegmentValues`).
3. A `Segments` result of seven or more must fold the seventh and later into one "Other" entry (their values summed, colored `--ink-muted`, never a seventh chart color), not silently drop them or index past a fixed six-slot color array (Task 1, `TestDonutRenderFoldsExtraSegmentsIntoOther`).
4. An empty or all-zero `Segments` result must render the no-data state, not divide by zero or hand Chart.js a chart with nothing in it (Task 1, `TestDonutRenderShowsNoDataWhenTotalIsZero`).
5. A `Segments` function's error must surface as the generic 500 page, never raw error text, the same contract `Stat`'s `Value`/`Trend` already have (Task 2, `TestDashboardDonutSegmentsErrorIsServerError`).

---

### Task 1: The dashboard package: Segment, Donut, and its rendering

A self-contained, unit-tested addition to the existing `dashboard` package: `Segment`, `DonutWidget`, the six-slot color assignment with NaN/Inf-dropping and Other-folding, and its own rendering. No `Panel` or HTTP involvement yet, nothing here needs a running server.

**Files:**
- Create: `dashboard/donut.go`
- Modify: `dashboard/render.templ`
- Modify: `dashboard/render_templ.go` (regenerated, not hand-edited)
- Modify: `dashboard/widget.go` (doc comment only)
- Modify: `internal/i18n/en.go`
- Test: `dashboard/donut_test.go`

**Interfaces:**
- Consumes: `github.com/a-h/templ`, `github.com/TechnoVizor/tellus/internal/i18n` (both already used the same way by `stat.go`/`render.templ`); the package-level `render(t, w)` test helper already in `dashboard/stat_test.go`.
- Produces:
  - `type Segment struct { Label string; Value float64 }`
  - `func Donut(label string) *DonutWidget`
  - `(*DonutWidget) Segments(fn func(ctx context.Context) ([]Segment, error)) *DonutWidget`
  - `(*DonutWidget) Href(url string) *DonutWidget`
  - `(*DonutWidget) Validate() error`
  - `(*DonutWidget) Render(ctx context.Context) (templ.Component, error)`, satisfying `Validated` (and therefore `Widget`)

- [ ] **Step 1: Write the failing tests**

Create `dashboard/donut_test.go`:

```go
package dashboard

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDonutValidateRequiresSegments(t *testing.T) {
	if err := Donut("Broken").Validate(); err == nil {
		t.Error("Donut without Segments must be rejected")
	}
	ok := Donut("OK").Segments(func(context.Context) ([]Segment, error) {
		return []Segment{{Label: "A", Value: 1}}, nil
	})
	if err := ok.Validate(); err != nil {
		t.Error(err)
	}
}

func TestDonutValidateErrorNamesTheWidget(t *testing.T) {
	err := Donut("Orders by Status").Validate()
	if err == nil || !strings.Contains(err.Error(), "Orders by Status") {
		t.Errorf("Validate() = %v, want an error naming the widget", err)
	}
}

func TestDonutRenderPropagatesSegmentsError(t *testing.T) {
	boom := errors.New("boom")
	w := Donut("X").Segments(func(context.Context) ([]Segment, error) { return nil, boom })
	_, err := w.Render(context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("Render error = %v, want %v", err, boom)
	}
}

func TestDonutRenderEscapesLabelsAndValues(t *testing.T) {
	w := Donut(`<script>alert(0)</script>`).Segments(func(context.Context) ([]Segment, error) {
		return []Segment{{Label: `<script>alert(1)</script>`, Value: 1}}, nil
	})
	out := render(t, w)
	if strings.Contains(out, "<script>") {
		t.Fatalf("widget label or segment label was not escaped: %s", out)
	}
}

func TestDonutRenderShowsLabelsAndValuesWithColors(t *testing.T) {
	w := Donut("Orders by Status").Segments(func(context.Context) ([]Segment, error) {
		return []Segment{{Label: "Paid", Value: 30}, {Label: "Pending", Value: 12}}, nil
	})
	out := render(t, w)
	if !strings.Contains(out, "Orders by Status") {
		t.Errorf("missing widget label: %s", out)
	}
	if !strings.Contains(out, "Paid") || !strings.Contains(out, "Pending") {
		t.Errorf("missing segment labels: %s", out)
	}
	if !strings.Contains(out, "var(--chart-1)") || !strings.Contains(out, "var(--chart-2)") {
		t.Errorf("segments must get the first two chart colors, in order: %s", out)
	}
	if !strings.Contains(out, "42") { // 30 + 12
		t.Errorf("missing the center total: %s", out)
	}
	if strings.Contains(out, "Other") {
		t.Errorf("six or fewer segments must not produce an Other row: %s", out)
	}
}

func TestDonutRenderFoldsExtraSegmentsIntoOther(t *testing.T) {
	w := Donut("X").Segments(func(context.Context) ([]Segment, error) {
		return []Segment{
			{Label: "First", Value: 1}, {Label: "Second", Value: 1}, {Label: "Third", Value: 1},
			{Label: "Fourth", Value: 1}, {Label: "Fifth", Value: 1}, {Label: "Sixth", Value: 1},
			{Label: "Seventh", Value: 2}, {Label: "Eighth", Value: 3},
		}, nil
	})
	out := render(t, w)
	if !strings.Contains(out, "Other") {
		t.Errorf("expected an Other row for the 7th+ segments: %s", out)
	}
	if strings.Contains(out, "Seventh") || strings.Contains(out, "Eighth") {
		t.Errorf("the 7th and 8th segment labels must not render on their own: %s", out)
	}
	if !strings.Contains(out, "var(--chart-6)") {
		t.Errorf("the 6th kept segment (Sixth) must still get a real chart color: %s", out)
	}
	if strings.Contains(out, "--chart-7") {
		t.Errorf("there is no 7th chart color slot: %s", out)
	}
	if !strings.Contains(out, "var(--ink-muted)") {
		t.Errorf("Other must render with --ink-muted, not a chart color: %s", out)
	}
}

func TestDonutRenderDropsNonFiniteSegmentValues(t *testing.T) {
	w := Donut("X").Segments(func(context.Context) ([]Segment, error) {
		return []Segment{
			{Label: "Good", Value: 10},
			{Label: "Broken", Value: math.NaN()},
			{Label: "AlsoBroken", Value: math.Inf(1)},
		}, nil
	})
	out := render(t, w)
	if strings.Contains(out, "Broken") || strings.Contains(out, "AlsoBroken") {
		t.Errorf("a non-finite segment value must be dropped entirely: %s", out)
	}
	if !strings.Contains(out, "Good") || !strings.Contains(out, "10") {
		t.Errorf("the remaining finite segment must still render: %s", out)
	}
}

func TestDonutRenderShowsNoDataWhenTotalIsZero(t *testing.T) {
	empty := Donut("X").Segments(func(context.Context) ([]Segment, error) { return nil, nil })
	out := render(t, empty)
	if !strings.Contains(out, "No data") {
		t.Errorf("an empty Segments result must render the no-data state: %s", out)
	}

	allZero := Donut("X").Segments(func(context.Context) ([]Segment, error) {
		return []Segment{{Label: "A", Value: 0}, {Label: "B", Value: 0}}, nil
	})
	out = render(t, allZero)
	if !strings.Contains(out, "No data") {
		t.Errorf("an all-zero Segments result must render the no-data state: %s", out)
	}
}

func TestDonutRenderChevronOnlyWithHref(t *testing.T) {
	seg := func(context.Context) ([]Segment, error) { return []Segment{{Label: "A", Value: 1}}, nil }
	plain := Donut("X").Segments(seg)
	if strings.Contains(render(t, plain), "stat-chevron") {
		t.Error("a Donut without Href must not render a chevron")
	}
	linked := Donut("X").Segments(seg).Href("/somewhere")
	out := render(t, linked)
	if !strings.Contains(out, "stat-chevron") || !strings.Contains(out, `href="/somewhere"`) {
		t.Errorf("a Donut with Href must render a chevron and link: %s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./dashboard/... 2>&1 || true`
Expected: FAIL, `undefined: Donut`, `undefined: Segment`, and related undefined symbols (`dashboard/donut.go` does not exist yet).

- [ ] **Step 3: Add the two i18n keys**

In `internal/i18n/en.go`, the "layout" group currently reads (around line 39):

```go
	"dashboard.trend_up":   "Trending up",
	"dashboard.trend_down": "Trending down",
```

Change it to:

```go
	"dashboard.trend_up":   "Trending up",
	"dashboard.trend_down": "Trending down",
	"dashboard.other":      "Other",
	"dashboard.no_data":    "No data",
```

These cannot wait for a later task: Step 1's tests assert on the literal words "Other" and "No data", and `i18n.T` returns the key itself (`internal/i18n/i18n.go`) when a key is missing, so those assertions would otherwise fail on `"dashboard.other"`/`"dashboard.no_data"` literal strings instead.

- [ ] **Step 4: Update widget.go's doc comment**

`dashboard/widget.go` currently reads:

```go
// Widget is one piece of the dashboard's home page. A host builds a
// dashboard from a list of these; Stat is the only implementation in this
// phase, more follow in later work.
type Widget interface {
```

Change the comment to:

```go
// Widget is one piece of the dashboard's home page. A host builds a
// dashboard from a list of these; Stat and Donut are its two
// implementations so far, more follow in later work.
type Widget interface {
```

- [ ] **Step 5: Create dashboard/donut.go**

```go
package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/i18n"
)

// Segment is one labeled slice of a Donut's data. Value is assumed
// non-negative, matching the reference use (counts, sums, shares); a host
// returning a negative value gets an undefined visual result, not a
// validation error, the same trust boundary Value/Trend already have on
// Stat. A NaN or infinite Value is dropped entirely by resolveSegments, the
// same defensive posture Stat already has for Trend's percent.
type Segment struct {
	Label string
	Value float64
}

// resolvedSegment is Render's resolved, display-ready result for one
// slice: color is a CSS custom property name ("--chart-1", or
// "--ink-muted" for the folded Other bucket), not a literal hex, so a
// theme change repaints with the right set with no server round-trip.
type resolvedSegment struct {
	label string
	value float64
	color string
}

// chartColors is the fixed, six-slot categorical order validated against
// this project's actual light and dark surfaces
// (docs/superpowers/specs/2026-09-30-dashboard-donut-design.md section 6).
// A segment's position in the kept list decides its color, never its
// label or value.
var chartColors = [6]string{"--chart-1", "--chart-2", "--chart-3", "--chart-4", "--chart-5", "--chart-6"}

// DonutWidget is a part-to-whole chart with up to six colored, labeled
// segments, a legend, and a center total. The label and href are fixed
// once built; segments are resolved fresh on every request through a
// host-supplied function, with no assumption it is backed by a Tellus
// Resource.
type DonutWidget struct {
	label      string
	href       string
	segmentsFn func(ctx context.Context) ([]Segment, error)
}

// Donut returns a donut chart widget with the given label.
func Donut(label string) *DonutWidget {
	return &DonutWidget{label: label}
}

// Segments sets the function that computes the chart's slices on each
// request. Required.
func (w *DonutWidget) Segments(fn func(ctx context.Context) ([]Segment, error)) *DonutWidget {
	w.segmentsFn = fn
	return w
}

// Href makes the whole card a link, with a trailing chevron. Optional: a
// Donut with no Href renders as a plain, non-linked card.
func (w *DonutWidget) Href(url string) *DonutWidget { w.href = url; return w }

// Validate reports whether the widget is configured well enough to
// render, checked by Panel.Dashboard before the panel starts. Segment
// count is data, not configuration, so the six-slot cap (resolveSegments)
// is not checked here: it can only be known once Segments actually runs,
// per request.
func (w *DonutWidget) Validate() error {
	if w.segmentsFn == nil {
		return fmt.Errorf("dashboard.Donut(%q) needs .Segments(...)", w.label)
	}
	return nil
}

// Render resolves the widget's segments for one request and returns the
// component that draws the card: the chart and legend when there is
// anything to show, or a no-data state when every segment is missing,
// zero, or non-finite.
func (w *DonutWidget) Render(ctx context.Context) (templ.Component, error) {
	segs, err := w.segmentsFn(ctx)
	if err != nil {
		return nil, err
	}
	resolved, total := resolveSegments(segs)
	if total == 0 {
		return donutEmptyWidget(w), nil
	}
	return donutWidget(w, resolved, total), nil
}

// resolveSegments drops any non-finite value first (a NaN or Inf would
// make json.Marshal fail silently in segmentsJSON, breaking the chart
// with no visible error). It then keeps at most six of what remains,
// folding the seventh and later into one final entry, labeled Other,
// colored --ink-muted rather than a seventh chart color, its value the
// sum of every folded segment's Value.
func resolveSegments(segs []Segment) ([]resolvedSegment, float64) {
	finite := make([]Segment, 0, len(segs))
	for _, s := range segs {
		if !math.IsNaN(s.Value) && !math.IsInf(s.Value, 0) {
			finite = append(finite, s)
		}
	}
	kept := finite
	hasOther := len(finite) > 6
	var otherSum float64
	if hasOther {
		kept = finite[:6]
		for _, s := range finite[6:] {
			otherSum += s.Value
		}
	}
	resolved := make([]resolvedSegment, 0, len(kept)+1)
	var total float64
	for i, s := range kept {
		resolved = append(resolved, resolvedSegment{label: s.Label, value: s.Value, color: chartColors[i]})
		total += s.Value
	}
	if hasOther {
		resolved = append(resolved, resolvedSegment{label: i18n.T("dashboard.other"), value: otherSum, color: "--ink-muted"})
		total += otherSum
	}
	return resolved, total
}

// segmentsJSON encodes segs for Chart.js: an array of {label, value,
// color} objects, color being the CSS custom property name resolved
// client-side (app.js), not a literal hex. segs already excludes
// non-finite values (resolveSegments), so Marshal cannot fail here.
func segmentsJSON(segs []resolvedSegment) string {
	type payload struct {
		Label string  `json:"label"`
		Value float64 `json:"value"`
		Color string  `json:"color"`
	}
	out := make([]payload, len(segs))
	for i, s := range segs {
		out[i] = payload{Label: s.label, Value: s.value, Color: s.color}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// formatNumber renders v rounded to one decimal place, trimming a bare
// ".0", the same formatting trendValue.text already uses in stat.go.
func formatNumber(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}
```

- [ ] **Step 6: Append to dashboard/render.templ**

`dashboard/render.templ` currently ends after `statBody`. Append:

```templ
templ donutWidget(w *DonutWidget, segs []resolvedSegment, total float64) {
	if w.href != "" {
		<a class="card dashboard-card" href={ templ.SafeURL(w.href) }>
			@donutBody(w, segs, total)
			<span class="stat-chevron" aria-hidden="true">›</span>
		</a>
	} else {
		<div class="card dashboard-card">
			@donutBody(w, segs, total)
		</div>
	}
}

templ donutBody(w *DonutWidget, segs []resolvedSegment, total float64) {
	<div class="card-body">
		<div class="muted">{ w.label }</div>
		<div class="donut-wrap">
			<canvas class="donut-chart" data-segments={ segmentsJSON(segs) }></canvas>
			<div class="donut-total">{ formatNumber(total) }</div>
		</div>
		<ul class="donut-legend">
			for _, s := range segs {
				<li>
					<span class="donut-swatch" style={ "background:var(" + s.color + ")" }></span>
					{ s.label }
					<span class="muted">{ formatNumber(s.value) }</span>
				</li>
			}
		</ul>
	</div>
}

templ donutEmptyWidget(w *DonutWidget) {
	if w.href != "" {
		<a class="card dashboard-card" href={ templ.SafeURL(w.href) }>
			@donutEmptyBody(w)
			<span class="stat-chevron" aria-hidden="true">›</span>
		</a>
	} else {
		<div class="card dashboard-card">
			@donutEmptyBody(w)
		</div>
	}
}

templ donutEmptyBody(w *DonutWidget) {
	<div class="card-body">
		<div class="muted">{ w.label }</div>
		<div class="donut-wrap">
			<div class="donut-empty" aria-hidden="true"></div>
			<div class="donut-total">{ i18n.T("dashboard.no_data") }</div>
		</div>
	</div>
}
```

- [ ] **Step 7: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test ./dashboard/... -v`
Expected: PASS (all Stat tests plus the nine new Donut tests).

- [ ] **Step 8: Run gofmt and vet**

Run: `gofmt -l dashboard/ internal/i18n/ && go vet ./dashboard/... ./internal/i18n/...`
Expected: no output, no errors.

- [ ] **Step 9: Commit**

```bash
git add dashboard/donut.go dashboard/render.templ dashboard/render_templ.go dashboard/widget.go dashboard/donut_test.go internal/i18n/en.go
git commit -m "feat: add the dashboard.Donut widget with Other-folding and NaN/Inf guards"
```

---

### Task 2: Prove Donut composes through the real Panel and HTTP pipeline

`Panel.Dashboard` and `home()` (`panel.go`) already loop over `[]dashboard.Widget` generically, unchanged since the `Stat` work: they were written against the interface, not the concrete type. This task adds no production code; it is the end-to-end proof, through the real HTTP harness, that a second `Widget` implementation composes with the first and follows the same validation and error contracts, the way `TestDashboardWidgetsReplaceAutomaticCards` and `TestDashboardMethodRejectsMisconfiguredStat` already proved for `Stat` alone.

**Files:**
- Test: `dashboard_widgets_test.go` (existing file, modify)

**Interfaces:**
- Consumes: `dashboard.Donut`, `dashboard.Segment` (Task 1); `dashboard.Stat` (already shipped); `newWidgetDashboardHarness(t, regs, widgets...)`, `errBoom`, `newMemAuth`, `testSecret` (already in `dashboard_widgets_test.go`/`helpers_test.go`).
- Produces: nothing new; this task's deliverable is the three tests themselves.

- [ ] **Step 1: Write the tests**

Append to `dashboard_widgets_test.go`:

```go
func TestDashboardComposesStatAndDonut(t *testing.T) {
	h := newWidgetDashboardHarness(t, nil,
		dashboard.Stat("Email Sent").Value(func(context.Context) (string, error) {
			return "1,251 Mail", nil
		}),
		dashboard.Donut("Orders by Status").Segments(func(context.Context) ([]dashboard.Segment, error) {
			return []dashboard.Segment{{Label: "Paid", Value: 30}, {Label: "Pending", Value: 12}}, nil
		}),
	)
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "Email Sent", "1,251 Mail", "Orders by Status", "Paid", "Pending")
}

func TestDashboardMethodRejectsMisconfiguredDonut(t *testing.T) {
	p, err := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err != nil {
		t.Fatal(err)
	}
	err = p.Dashboard(dashboard.Donut("Broken"))
	if err == nil || !strings.Contains(err.Error(), "Broken") {
		t.Errorf("expected an error naming the widget, got %v", err)
	}
}

func TestDashboardDonutSegmentsErrorIsServerError(t *testing.T) {
	h := newWidgetDashboardHarness(t, nil, dashboard.Donut("Broken").Segments(func(context.Context) ([]dashboard.Segment, error) {
		return nil, errBoom
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

- [ ] **Step 2: Run the tests**

Run: `go test . -run 'TestDashboardComposesStatAndDonut|TestDashboardMethodRejectsMisconfiguredDonut|TestDashboardDonutSegmentsErrorIsServerError' -v`
Expected: PASS immediately. Task 1 already implements `dashboard.Donut` correctly on its own, and `Panel.Dashboard`/`home()` were already generic over any `dashboard.Widget` before this plan started. There is no implementation step in this task: it exists to prove that claim end to end, through the real HTTP pipeline, not to add new behavior. If any of the three fails, the bug is in Task 1's `Render`/`Validate` or in the existing `Panel.Dashboard`/`home()`, not in anything this task adds.

- [ ] **Step 3: Run gofmt and vet**

Run: `gofmt -l . && go vet ./...`
Expected: no output, no errors.

- [ ] **Step 4: Commit**

```bash
git add dashboard_widgets_test.go
git commit -m "test: prove dashboard.Donut composes with Stat through the real panel"
```

---

### Task 3: Vendor Chart.js

Fetches Chart.js's UMD build, adds its license, and loads it on every page the same unconditional way uPlot already is.

**Files:**
- Create: `internal/assets/static/chart.umd.min.js`
- Modify: `THIRD_PARTY_NOTICES.md`
- Modify: `internal/ui/layout.templ`
- Modify: `internal/ui/layout_templ.go` (regenerated, not hand-edited)
- Test: `internal/assets/assets_test.go`

**Interfaces:**
- Produces: the global `Chart` constructor, available in every page's browser context from Task 4 onward.

- [ ] **Step 1: Write the failing test**

In `internal/assets/assets_test.go`, `TestVendoredLibrariesAreReallyEmbedded` currently reads:

```go
func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js", "/uPlot.iife.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}
```

Add `"/chart.umd.min.js"` to that same list:

```go
func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js", "/uPlot.iife.min.js", "/chart.umd.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/assets/... -run TestVendoredLibrariesAreReallyEmbedded -v`
Expected: FAIL, `/chart.umd.min.js: status 404, 0 bytes` (the file does not exist yet).

- [ ] **Step 3: Fetch and save Chart.js**

```bash
curl -fsSL -o internal/assets/static/chart.umd.min.js https://cdn.jsdelivr.net/npm/chart.js@4.5.1/dist/chart.umd.min.js
ls -la internal/assets/static/chart.umd.min.js
```

Expected: a single file, roughly 200 KB.

- [ ] **Step 4: Add the license to THIRD_PARTY_NOTICES.md**

`THIRD_PARTY_NOTICES.md` currently has the `uPlot 1.6.32 (MIT)` section end just before the `## Inter Variable 5.3.0 (SIL Open Font License 1.1)` header:

```
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

## Inter Variable 5.3.0 (SIL Open Font License 1.1)
```

Insert a new section between them, so it reads:

```
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

## Chart.js 4.5.1 (MIT)

The MIT License (MIT)

Copyright (c) 2014-2024 Chart.js Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Inter Variable 5.3.0 (SIL Open Font License 1.1)
```

(This is the exact text at `https://raw.githubusercontent.com/chartjs/Chart.js/v4.5.1/LICENSE.md`, confirmed when this plan was written.)

- [ ] **Step 5: Add the script tag to layout.templ**

`internal/ui/layout.templ`'s `head` templ currently reads:

```templ
		<link rel="stylesheet" href={ asset("app.css") }/>
		<link rel="stylesheet" href={ asset("uPlot.min.css") }/>
		<script src={ asset("uPlot.iife.min.js") } defer></script>
		<script src={ asset("app.js") } defer></script>
		<script src={ asset("htmx.min.js") } defer></script>
		<script src={ asset("alpine.min.js") } defer></script>
	</head>
```

Add the Chart.js script tag right after uPlot's own, before `app.js`'s (so the global `Chart` exists before `app.js`'s `initDonutCharts`, added in Task 4, runs; deferred scripts execute in document order):

```templ
		<link rel="stylesheet" href={ asset("app.css") }/>
		<link rel="stylesheet" href={ asset("uPlot.min.css") }/>
		<script src={ asset("uPlot.iife.min.js") } defer></script>
		<script src={ asset("chart.umd.min.js") } defer></script>
		<script src={ asset("app.js") } defer></script>
		<script src={ asset("htmx.min.js") } defer></script>
		<script src={ asset("alpine.min.js") } defer></script>
	</head>
```

Chart.js ships no separate CSS file (unlike uPlot), so there is no new `<link>` to add.

- [ ] **Step 6: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test ./internal/assets/... ./internal/ui/... -v`
Expected: PASS, including the now-satisfied `TestVendoredLibrariesAreReallyEmbedded`.

- [ ] **Step 7: Run gofmt and vet**

Run: `gofmt -l internal/assets/ internal/ui/ && go vet ./internal/assets/... ./internal/ui/...`
Expected: no output, no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/assets/static/chart.umd.min.js internal/assets/assets_test.go internal/ui/layout.templ internal/ui/layout_templ.go THIRD_PARTY_NOTICES.md
git commit -m "feat: vendor Chart.js for the upcoming donut widget"
```

---

### Task 4: Wire the donut chart into app.css and app.js

Adds the six-slot categorical color tokens, the donut's own layout rules, and the client-side script that turns each `.donut-chart` canvas plus its `data-segments` JSON into an actual Chart.js doughnut, redrawn on theme toggle exactly like the sparkline already is.

**Files:**
- Modify: `internal/assets/static/app.css`
- Modify: `internal/assets/static/app.js`
- Test: `internal/assets/assets_test.go`

**Interfaces:**
- Consumes: `.donut-chart`/`data-segments` markup and `--chart-1`..`--chart-6`/`--ink-muted` custom properties (Task 1's `dashboard/render.templ`, Task 1's spec-mandated colors); the existing `initDashboardCharts` function and `themeToggle` Alpine component in `app.js`, both unchanged in shape.
- Produces: nothing consumed by a later task; this is the last task in this plan.

- [ ] **Step 1: Write the failing test**

Add to `internal/assets/assets_test.go`:

```go
func TestDonutCSSAndJSAreServed(t *testing.T) {
	css := get(t, "/app.css")
	if !strings.Contains(css.Body.String(), "--chart-1") {
		t.Error("app.css is missing the donut chart color tokens")
	}
	js := get(t, "/app.js")
	if !strings.Contains(js.Body.String(), "initDonutCharts") {
		t.Error("app.js is missing initDonutCharts")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/assets/... -run TestDonutCSSAndJSAreServed -v`
Expected: FAIL, both `t.Error` lines fire (neither string exists yet).

- [ ] **Step 3: Add the color tokens to app.css**

The light `:root` block in `internal/assets/static/app.css` currently has, around line 57:

```css
    --danger: #c13515;
    --success: #1a7d3a;
```

Add the six light chart tokens right after `--success`:

```css
    --danger: #c13515;
    --success: #1a7d3a;
    --chart-1: #2a78d6;
    --chart-2: #eb6834;
    --chart-3: #1baf7a;
    --chart-4: #eda100;
    --chart-5: #e87ba4;
    --chart-6: #008300;
```

The same file has two dark-mode blocks with the identical `--danger`/`--success` pair, `@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { ... } }` around line 79, and `:root[data-theme="dark"] { ... }` around line 96. In **both**, add the six dark chart tokens right after that block's own `--success`:

```css
      --danger: #f26d5b;
      --success: #4ade80;
      --chart-1: #3987e5;
      --chart-2: #d95926;
      --chart-3: #199e70;
      --chart-4: #c98500;
      --chart-5: #d55181;
      --chart-6: #008300;
```

(indentation matches whichever of the two dark blocks you are editing: the media-query block is nested one level deeper than the `:root[data-theme="dark"]` block, same as `--danger`/`--success` already are in each).

- [ ] **Step 4: Add the donut layout rules to app.css**

The components layer currently has, right after `.trend-down` and right before `.notice, .alert`:

```css
  .trend-up { color: var(--success); background: color-mix(in srgb, var(--success) 12%, transparent); }
  .trend-down { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .notice, .alert {
```

Insert the donut rules between them:

```css
  .trend-up { color: var(--success); background: color-mix(in srgb, var(--success) 12%, transparent); }
  .trend-down { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .donut-wrap { position: relative; width: 140px; height: 140px; margin: 12px auto 0; }
  .donut-total { position: absolute; inset: 0; display: grid; place-items: center; text-align: center; font-size: 20px; font-weight: 700; }
  .donut-empty { position: absolute; inset: 0; border-radius: 50%; border: 16px solid var(--surface-strong); }
  .donut-legend { list-style: none; margin: 12px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
  .donut-legend li { display: flex; align-items: center; gap: 8px; }
  .donut-legend li .muted { margin-left: auto; }
  .donut-swatch { display: inline-block; width: 10px; height: 10px; border-radius: 2px; flex-shrink: 0; }
  .notice, .alert {
```

- [ ] **Step 5: Add initDonutCharts to app.js**

`internal/assets/static/app.js` currently reads, right after `initDashboardCharts`:

```js
document.addEventListener("DOMContentLoaded", initDashboardCharts);

document.addEventListener("alpine:init", function () {
```

Insert `initDonutCharts` and its own `DOMContentLoaded` listener between them:

```js
document.addEventListener("DOMContentLoaded", initDashboardCharts);

function initDonutCharts() {
  document.querySelectorAll(".donut-chart").forEach(function (el) {
    var existing = Chart.getChart(el);
    if (existing) existing.destroy(); // clear a previous draw, same reason initDashboardCharts clears uPlot's
    var segs = JSON.parse(el.dataset.segments);
    var style = getComputedStyle(document.documentElement);
    new Chart(el, {
      type: "doughnut",
      data: {
        labels: segs.map(function (s) { return s.label; }),
        datasets: [{
          data: segs.map(function (s) { return s.value; }),
          backgroundColor: segs.map(function (s) { return style.getPropertyValue(s.color).trim(); }),
          borderWidth: 0,
        }],
      },
      options: {
        cutout: "60%",
        plugins: { legend: { display: false } },
      },
    });
  });
}
document.addEventListener("DOMContentLoaded", initDonutCharts);

document.addEventListener("alpine:init", function () {
```

- [ ] **Step 6: Redraw the donut on theme toggle**

`themeToggle`'s `toggle()` method currently reads:

```js
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
        initDashboardCharts();
      },
```

Add a call to `initDonutCharts` right after `initDashboardCharts()`:

```js
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
        initDashboardCharts();
        initDonutCharts();
      },
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/assets/... -v`
Expected: PASS, including `TestDonutCSSAndJSAreServed`.

- [ ] **Step 8: Run the whole project's tests, gofmt, and vet**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, no vet errors, every test passes except the one pre-existing unrelated failure, `TestLocalStorageRejectsFilenamesThatEscapeTheDirectory` (Windows-path handling on Linux, untouched by this plan).

- [ ] **Step 9: Commit**

```bash
git add internal/assets/static/app.css internal/assets/static/app.js internal/assets/assets_test.go
git commit -m "feat: wire the donut chart's colors and Chart.js init into the panel"
```

---

## After this plan

**Manual verification, before relying on this in a real dashboard:** `go test` proves the server-rendered HTML and the vendored file's presence, but nothing in this project's test suite runs a browser (`docs/spec.md` section 13 defers Playwright to later), so Chart.js actually drawing a correct, correctly colored, correctly tooltipped donut in both themes is unverified by any automated test in this plan. Before trusting this widget: temporarily add a `dashboard.Donut(...)` call to `examples/shop/main.go` (`docker compose up -d --wait && cd examples/shop && go run .`, then open `http://localhost:8091/admin`), check the chart against both light and dark themes, check the hover tooltip, and check a case with seven or more segments to see the Other fold for real. Whether to keep that wiring in `examples/shop` afterward, augmenting or replacing its current `Stat` widgets, is a natural follow-up commit, not part of this plan, the same way wiring `Stat` into `examples/shop` was its own commit after the `Stat` plan landed.

The embedded, sortable/filterable data table widget is the next sub-project in the decomposition (`docs/superpowers/specs/2026-09-29-dashboard-widgets-design.md` section 1), a separate spec, not designed here.
