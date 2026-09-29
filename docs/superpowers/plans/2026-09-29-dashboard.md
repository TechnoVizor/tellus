# Dashboard Home Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the panel's "redirect to the first resource" home page with an automatic dashboard: a stat card per registered resource (a row count, and a 30-day new-record sparkline where the model supports it), with zero extra host configuration.

**Architecture:** `resource[T].register()` (already generic over the resource's own model type) builds a plain, non-generic `dashboardCard` the same way it already builds `optionLoaders` for relations: register-time reflection, request-time data access through a closure. `Panel.home` gathers every card's numbers fresh on each request and renders a new `ui.DashboardPage`. Charts draw with uPlot, vendored and embedded exactly like htmx and Alpine already are.

**Tech Stack:** Go 1.26+, templ v0.3.1020, GORM v1.31.2, uPlot 1.6.32 (new), Postgres 17 in Docker for integration tests (unchanged).

**Spec:** `docs/superpowers/specs/2026-09-29-dashboard-design.md` (the plan argues from it; read both).

## Global Constraints

- English only UI: any new user-facing string goes through `i18n.T("key")`, the key must exist in `internal/i18n/en.go`. This plan adds one key, `ui.dashboard`.
- No em-dashes anywhere: not in code, comments, docs, copy or commit messages.
- Generated `*_templ.go` files are committed and regenerated with `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate` after every `.templ` change, before running tests.
- Integration tests use the real Postgres from `docker-compose.yml` (host port 55432) through `TELLUS_TEST_DSN`, and skip themselves when it is unset (`internal/testdb.Open(t)` already does this).
- Time-series data is GORM-only, resolved through the `*gorm.DB` passed to `Resource[T](db)`, never through a custom `DataSource[T]`, the same stance `relation.go` already takes for relations.
- `date_trunc` is Postgres-specific, the same "Postgres first" tradeoff `gormsource.go` already makes for its `ILIKE` search (`docs/spec.md` section 5); flag it the same way, with a `// ponytail:` comment.
- uPlot 1.6.32 (MIT) is a new pinned, vendored dependency: both `internal/assets/static/uPlot.iife.min.js` and `internal/assets/static/uPlot.min.css`, license text in `THIRD_PARTY_NOTICES.md`, alongside htmx 2.0.11, Alpine.js 3.17.4, and Inter Variable.
- Commit messages follow Conventional Commits, no em-dashes.

## Review Focus

1. A model with `CreatedAt` but no rows at all in the last 30 days must still render a full, zero-filled 30-point series, not error and not omit the card (Task 2, `TestResolveTimeSeriesEmptyWindow`).
2. A resource on a custom (non-GORM) `DataSource` must still show its count, with no chart and no error (Task 3, `TestHomeRendersDashboard` asserting no `data-series` attribute for that card).
3. Dashboard cards must appear in the same order resources were registered in, the same order the sidebar nav already uses (Task 3, `TestDashboardCardOrderMatchesRegistration`).
4. A count must be fresh on every request, never cached from an earlier one, since the spec explicitly promises this (Task 3, `TestDashboardCountsAreFreshPerRequest`).
5. A `countFn` or `seriesFn` failure must surface as the generic 500 page, never raw database error text (Task 3, `TestHomeCountErrorIsServerError` and `TestHomeSeriesErrorIsServerError`).

## Deliberately not in this plan

Host-defined custom widgets or metrics, non-count aggregations (sum, average, min, max), a configurable time range (30 days is fixed), per-resource dashboard opt-out, drill-down from a chart point to its filtered list, a real-time or auto-refreshing dashboard, per-user dashboard permissions, and date bucketing for a database other than Postgres. All deferred in `docs/superpowers/specs/2026-09-29-dashboard-design.md` section 8.

---

### Task 1: Vendor uPlot and wire it into every page

Fetches uPlot's two distribution files, adds its license, and loads it on every page the same unconditional way htmx and Alpine already are.

**Files:**
- Create: `internal/assets/static/uPlot.iife.min.js`
- Create: `internal/assets/static/uPlot.min.css`
- Modify: `THIRD_PARTY_NOTICES.md`
- Modify: `internal/ui/layout.templ`
- Modify: `internal/ui/layout_templ.go` (regenerated, not hand-edited)
- Test: `internal/assets/assets_test.go`

**Interfaces:**
- Consumes: `internal/assets.Handler()` (already serves every file under `internal/assets/static/`, no change needed there).
- Produces: the global `uPlot` constructor, available in every page's browser context from Task 3 onward.

- [ ] **Step 1: Fetch and save the two uPlot files**

```bash
curl -fsSL -o internal/assets/static/uPlot.iife.min.js https://cdn.jsdelivr.net/npm/uplot@1.6.32/dist/uPlot.iife.min.js
curl -fsSL -o internal/assets/static/uPlot.min.css https://cdn.jsdelivr.net/npm/uplot@1.6.32/dist/uPlot.min.css
ls -la internal/assets/static/uPlot.iife.min.js internal/assets/static/uPlot.min.css
```

Expected: both files present, the `.js` file at least 10 KB, the `.css` file under 5 KB.

- [ ] **Step 2: Write the failing test for the new vendored files**

In `internal/assets/assets_test.go`, the existing `TestVendoredLibrariesAreReallyEmbedded` reads:

```go
func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}
```

Add `"/uPlot.iife.min.js"` to that same list (it is well over 10 KB, the same threshold already used for htmx and Alpine), and add a new test right after it for the CSS file, whose 1 to 2 KB size does not fit that threshold:

```go
func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js", "/uPlot.iife.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}

func TestUPlotCSSIsEmbedded(t *testing.T) {
	rec := get(t, "/uPlot.min.css")
	if rec.Code != http.StatusOK || rec.Body.Len() < 500 {
		t.Errorf("uPlot.min.css: status %d, %d bytes", rec.Code, rec.Body.Len())
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/assets/... -run "TestVendoredLibrariesAreReallyEmbedded|TestUPlotCSSIsEmbedded" -v`
Expected: FAIL, `/uPlot.iife.min.js: status 404` and `uPlot.min.css: status 404` (the files exist on disk from Step 1, but nothing has rebuilt against them yet, `go:embed` only needs a rebuild, so this should already almost pass; if it does pass here because `go:embed` picked the files up automatically, that is fine, move straight to Step 4, the point of this step is to confirm the two new files are genuinely served, not to force a contrived failure).

- [ ] **Step 4: Confirm and move on**

Run: `go test ./internal/assets/... -v`
Expected: PASS. (`go:embed static` in `internal/assets/assets.go` already embeds every file under the directory with no per-file listing to update, so Step 1's new files are picked up automatically; this step exists to prove that, not to write new embedding code.)

- [ ] **Step 5: Add the uPlot license to THIRD_PARTY_NOTICES.md**

Append, after the existing Alpine.js section and before whatever comes next (Inter, most likely):

```markdown
## uPlot 1.6.32 (MIT)

The MIT License (MIT)

Copyright (c) 2022 Leon Sorokin

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
```

- [ ] **Step 6: Wire the two files into every page's head**

In `internal/ui/layout.templ`, the `head()` templ currently reads:

```templ
templ head(title, brand string, asset func(string) string) {
	<head>
		<meta charset="utf-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<meta name="robots" content="noindex, nofollow"/>
		<meta name="htmx-config" content={ `{"refreshOnHistoryMiss":true}` }/>
		<title>{ title } | { brand }</title>
		<script src={ asset("theme-init.js") }></script>
		<link rel="stylesheet" href={ asset("app.css") }/>
		<script src={ asset("app.js") } defer></script>
		<script src={ asset("htmx.min.js") } defer></script>
		<script src={ asset("alpine.min.js") } defer></script>
	</head>
}
```

Change it to:

```templ
templ head(title, brand string, asset func(string) string) {
	<head>
		<meta charset="utf-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<meta name="robots" content="noindex, nofollow"/>
		<meta name="htmx-config" content={ `{"refreshOnHistoryMiss":true}` }/>
		<title>{ title } | { brand }</title>
		<script src={ asset("theme-init.js") }></script>
		<link rel="stylesheet" href={ asset("app.css") }/>
		<link rel="stylesheet" href={ asset("uPlot.min.css") }/>
		<script src={ asset("uPlot.iife.min.js") } defer></script>
		<script src={ asset("app.js") } defer></script>
		<script src={ asset("htmx.min.js") } defer></script>
		<script src={ asset("alpine.min.js") } defer></script>
	</head>
}
```

`uPlot.iife.min.js` is placed before `app.js`'s script tag: deferred scripts run in document order, and `app.js` (from Task 3 onward) calls the global `uPlot` constructor from a `DOMContentLoaded` listener, so `uPlot` must already be defined by then. uPlot loads unconditionally on every page (login included), the same way htmx and Alpine already do regardless of whether a given page actually uses them, consistency with existing precedent rather than a new decision.

- [ ] **Step 7: Regenerate templ and run the whole test suite**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `gofmt -l . && go vet ./... && go test ./internal/...`
Expected: no gofmt output, no vet errors, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/assets/static/uPlot.iife.min.js internal/assets/static/uPlot.min.css internal/assets/assets_test.go internal/ui/layout.templ internal/ui/layout_templ.go THIRD_PARTY_NOTICES.md
git commit -m "feat: vendor uPlot for the upcoming dashboard charts"
```

---

### Task 2: dashboardCard, resolveTimeSeries, and sparklineJSON

The pure data layer: what one resource contributes to the dashboard, and how its time series becomes chart-ready JSON. No page rendering yet, no wiring into registration yet.

**Files:**
- Create: `dashboard.go`
- Test: `dashboard_test.go`

**Interfaces:**
- Consumes: `isIntKind`/`isUintKind` are not needed here; this task only needs `gorm.Statement` (already used the same way in `gormsource.go`'s `NewGormSource` and `relation.go`'s `resolveRelation`) and `internal/testdb.Open` for its tests.
- Produces:
  - `type dailyCount struct { Day time.Time; Count int64 }`
  - `type dashboardCard struct { label, href string; countFn func(ctx context.Context) (int64, error); seriesFn func(ctx context.Context, days int) ([]dailyCount, error) }`
  - `func resolveTimeSeries(db *gorm.DB, model reflect.Type) func(ctx context.Context, days int) ([]dailyCount, error)`
  - `func sparklineJSON(rows []dailyCount, days int) string`

- [ ] **Step 1: Write the failing tests**

Create `dashboard_test.go`:

```go
package tellus

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/TechnoVizor/tellus/internal/testdb"
)

// dashActivity and dashNoTimestamp are fixtures dedicated to dashboard
// tests: one with the CreatedAt convention resolveTimeSeries looks for, one
// without, kept separate from the shared Product/Note/relation fixtures.
type dashActivity struct {
	ID        uint
	Name      string
	CreatedAt time.Time
}

type dashNoTimestamp struct {
	ID   uint
	Name string
}

func TestResolveTimeSeriesGroupsByDayAndExcludesOutsideTheWindow(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	create := func(daysAgo int, n int) {
		for i := 0; i < n; i++ {
			if err := db.Create(&dashActivity{Name: "x", CreatedAt: now.AddDate(0, 0, -daysAgo)}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	create(0, 2)  // today
	create(5, 3)  // 5 days ago
	create(40, 1) // outside the 30-day window, must be excluded

	loader := resolveTimeSeries(db, reflect.TypeOf(dashActivity{}))
	if loader == nil {
		t.Fatal("expected a series loader for a model with CreatedAt")
	}
	rows, err := loader(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range rows {
		total += r.Count
	}
	if total != 5 {
		t.Errorf("total in window = %d, want 5 (the row 40 days ago must be excluded)", total)
	}
	found := false
	for _, r := range rows {
		if r.Day.Format("2006-01-02") == now.AddDate(0, 0, -5).Format("2006-01-02") && r.Count == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("missing the 3-row day 5 days ago: %+v", rows)
	}
}

func TestResolveTimeSeriesEmptyWindow(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	// Only a row outside the window: the query must return no error and no
	// rows, not fail, and sparklineJSON on that empty result must still
	// produce a full zero-filled series (checked in the sibling test below).
	if err := db.Create(&dashActivity{Name: "old", CreatedAt: time.Now().AddDate(0, 0, -90)}).Error; err != nil {
		t.Fatal(err)
	}
	loader := resolveTimeSeries(db, reflect.TypeOf(dashActivity{}))
	rows, err := loader(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows inside the window, got %+v", rows)
	}
	raw := sparklineJSON(rows, 30)
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded[0]) != 30 || len(decoded[1]) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(decoded[0]), len(decoded[1]))
	}
	for i, y := range decoded[1] {
		if y != 0 {
			t.Errorf("day %d should be zero, got %d", i, y)
		}
	}
}

func TestResolveTimeSeriesNilCases(t *testing.T) {
	db := testdb.Open(t)
	if resolveTimeSeries(nil, reflect.TypeOf(dashActivity{})) != nil {
		t.Error("nil db must yield a nil series loader")
	}
	if resolveTimeSeries(db, reflect.TypeOf(dashNoTimestamp{})) != nil {
		t.Error("a model with no CreatedAt field must yield a nil series loader")
	}
}

func TestSparklineJSONFillsGapsAndKeepsRealCounts(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	rows := []dailyCount{
		{Day: today.AddDate(0, 0, -29), Count: 7},
		{Day: today, Count: 3},
	}
	raw := sparklineJSON(rows, 30)
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	xs, ys := decoded[0], decoded[1]
	if len(xs) != 30 || len(ys) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(xs), len(ys))
	}
	if ys[0] != 7 {
		t.Errorf("first day count = %d, want 7", ys[0])
	}
	if ys[29] != 3 {
		t.Errorf("last day (today) count = %d, want 3", ys[29])
	}
	for i := 1; i < 29; i++ {
		if ys[i] != 0 {
			t.Errorf("day %d should be zero-filled, got %d", i, ys[i])
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test . -run "TestResolveTimeSeries|TestSparklineJSON"`
Expected: FAIL, `undefined: resolveTimeSeries`, `undefined: sparklineJSON`, `undefined: dailyCount`.

- [ ] **Step 3: Create dashboard.go**

```go
package tellus

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"gorm.io/gorm"
)

// dashboardCard is what one registered resource contributes to the
// dashboard home page. Built once at Register time (resource.go), the same
// split relation.go's optionLoaders already use: register-time reflection
// with the resource's own concrete type, request-time data access through a
// plain, non-generic closure.
type dashboardCard struct {
	label    string
	href     string
	countFn  func(ctx context.Context) (int64, error)
	seriesFn func(ctx context.Context, days int) ([]dailyCount, error) // nil when unavailable
}

// dailyCount is one day's row count, as resolveTimeSeries's query returns
// it: sparse, only days with at least one row.
type dailyCount struct {
	Day   time.Time
	Count int64
}

// resolveTimeSeries returns nil when db is nil (a custom Source with no
// *gorm.DB, the same GORM-only stance relation.go already takes for
// relations) or when model has no CreatedAt field of type time.Time with a
// real column. Otherwise it returns a closure that groups the model's rows
// by day.
//
// ponytail: date_trunc is Postgres-specific, the same tradeoff
// gormsource.go's List already makes for its ILIKE search (docs/spec.md
// section 5, "Postgres first"). Use a portable bucketing expression when
// another database is supported.
func resolveTimeSeries(db *gorm.DB, model reflect.Type) func(ctx context.Context, days int) ([]dailyCount, error) {
	if db == nil {
		return nil
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(reflect.New(model).Interface()); err != nil {
		return nil
	}
	sf, ok := stmt.Schema.FieldsByName["CreatedAt"]
	if !ok || sf.FieldType != reflect.TypeOf(time.Time{}) || sf.DBName == "" {
		return nil
	}
	table, quotedCol := stmt.Schema.Table, db.Statement.Quote(sf.DBName)
	return func(ctx context.Context, days int) ([]dailyCount, error) {
		since := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
		var rows []struct {
			Day   time.Time
			Count int64
		}
		// quotedCol and table are schema names resolved once above, never
		// request input, so building the query from them is safe, the same
		// argument relation.go's loader already makes. The "day" and "count"
		// aliases, and the Group/Order calls below, are hardcoded literals
		// this function chose itself, never a model's own field or column
		// name, so unlike relation.go's label column they carry no
		// reserved-word risk.
		err := db.WithContext(ctx).Table(table).
			Select("date_trunc('day', " + quotedCol + ") AS day, COUNT(*) AS count").
			Where(quotedCol+" >= ?", since).
			Group("day").Order("day").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		out := make([]dailyCount, len(rows))
		for i, r := range rows {
			out[i] = dailyCount{Day: r.Day, Count: r.Count}
		}
		return out, nil
	}
}

// sparklineJSON fills any day with no rows to zero, so the chart's x-axis
// spacing is even, then encodes uPlot's own data shape: a two-element array
// of a timestamps array and a values array, x as Unix seconds.
func sparklineJSON(rows []dailyCount, days int) string {
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Day.Format("2006-01-02")] = r.Count
	}
	start := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	xs := make([]int64, days)
	ys := make([]int64, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		xs[i] = day.Unix()
		ys[i] = counts[day.Format("2006-01-02")]
	}
	b, _ := json.Marshal([2][]int64{xs, ys})
	return string(b)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run "TestResolveTimeSeries|TestSparklineJSON" -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Run gofmt and vet**

Run: `gofmt -l . && go vet ./...`
Expected: no output, no errors.

- [ ] **Step 6: Commit**

```bash
git add dashboard.go dashboard_test.go
git commit -m "feat: add dashboardCard, resolveTimeSeries, and sparklineJSON"
```

---

### Task 3: Wire the dashboard into registration and the home page

Registration builds a `dashboardCard` per resource; `Panel.home` renders them; the sidebar gets a "Dashboard" link; proven end to end with the HTTP harness and real Postgres.

**Files:**
- Modify: `resource.go`
- Modify: `panel.go`
- Modify: `internal/ui/views.go`
- Create: `internal/ui/dashboard.templ`
- Create: `internal/ui/dashboard_templ.go` (regenerated, not hand-edited)
- Modify: `internal/assets/static/app.css`
- Modify: `internal/assets/static/app.js`
- Modify: `internal/i18n/en.go`
- Modify: `resource_test.go` (rewrites `TestHomeRedirectsToFirstResource`, whose asserted behavior this task removes)
- Modify: `dashboard_test.go`

**Interfaces:**
- Consumes: `dashboardCard`, `dailyCount`, `resolveTimeSeries`, `sparklineJSON` (Task 2); `ListQuery`, `DataSource[T].List` (already in `datasource.go`); `p.serverError`, `p.render`, `p.renderMessage`, `p.url`, `p.shell` (already in `panel.go`).
- Produces: `Panel.dashboardCards []dashboardCard`; `ui.DashboardCardView`, `ui.DashboardView`, `ui.DashboardPage(v DashboardView) templ.Component`; the global JS function `initDashboardCharts()`.

- [ ] **Step 1: Write the failing tests**

`resource_test.go` currently has:

```go
func TestHomeRedirectsToFirstResource(t *testing.T) {
	h := loggedIn(t, newMemSource(1))
	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/admin/products" {
		t.Fatalf("home: %d %q", res.status, res.location())
	}
}
```

This behavior is exactly what this task removes; replace it in place with:

```go
func TestHomeRendersDashboard(t *testing.T) {
	h := loggedIn(t, newMemSource(3))
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "Dashboard", "Product", "3")
	if strings.Contains(res.body, "data-series") {
		t.Error("a resource on a custom DataSource has no *gorm.DB, so it must have no chart")
	}
}

func TestHomeWithNoResourcesShowsMessage(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "No resources are registered yet.")
}

func TestHomeCountErrorIsServerError(t *testing.T) {
	src := newMemSource(1)
	src.failWith = errBoom
	h := loggedIn(t, src)
	res := h.get("/")
	if res.status != http.StatusInternalServerError {
		t.Fatalf("status %d", res.status)
	}
	if strings.Contains(res.body, "boom") || strings.Contains(res.body, "secret") {
		t.Error("internal error text leaked to the client")
	}
}

func TestDashboardNavActiveOnlyOnDashboard(t *testing.T) {
	h := loggedIn(t, newMemSource(1))
	navOf := func(body string) string {
		start, end := strings.Index(body, "<nav"), strings.Index(body, "</nav>")
		return body[start:end]
	}
	home := navOf(h.get("/").body)
	if !strings.Contains(home, `aria-current="page">Dashboard`) {
		t.Errorf("dashboard nav not active on home: %s", home)
	}
	list := navOf(h.get("/products").body)
	if strings.Contains(list, `aria-current="page">Dashboard`) {
		t.Errorf("dashboard nav must not be active on a resource page: %s", list)
	}
	if !strings.Contains(list, `aria-current="page">Products`) {
		t.Errorf("products nav not active on its own page: %s", list)
	}
}

func TestDashboardCardOrderMatchesRegistration(t *testing.T) {
	cat := Resource[Note](nil).Source(newNoteSource()).
		Table(table.Text("Body")).
		Form(form.Text("Body").Required())
	prod := productResource(newMemSource(1))
	h := newHarness(t, Config{}, cat, prod)
	h.loginAdmin()
	body := h.get("/").body
	notesIdx, productsIdx := strings.Index(body, "Notes"), strings.Index(body, "Products")
	if notesIdx < 0 || productsIdx < 0 || notesIdx > productsIdx {
		t.Errorf("cards must appear in registration order (Notes then Products): %s", body)
	}
}

func TestDashboardCountsAreFreshPerRequest(t *testing.T) {
	src := newMemSource(1)
	h := loggedIn(t, src)
	first := h.get("/").body
	if !strings.Contains(first, ">1<") {
		t.Fatalf("expected a count of 1: %s", first)
	}
	src.items = append(src.items, Product{ID: 99, Name: "New"})
	second := h.get("/").body
	if !strings.Contains(second, ">2<") {
		t.Errorf("count did not refresh after the source changed: %s", second)
	}
}
```

`TestDashboardCardOrderMatchesRegistration` needs a second, trivial `DataSource` fixture, since every other test in this file uses `memSource` bound to `Product`. Add a minimal one right above it, following `relation_test.go`'s `relStubSource` convention exactly: only `List` does real work, since only `List` (through `countFn`) is ever exercised by this test, every other method panics rather than pretending to implement something never called:

```go
// noteSource is an in-memory DataSource[Note], used only to register a
// second resource alongside Product for the dashboard's card-order test.
// Only List is exercised (through countFn); everything else panics, the
// same relStubSource convention relation_test.go already uses.
type noteSource struct{ items []Note }

func newNoteSource() *noteSource { return &noteSource{items: []Note{{ID: 1, Body: "x"}}} }

func (s *noteSource) List(context.Context, ListQuery) (ListResult[Note], error) {
	return ListResult[Note]{Items: s.items, Total: int64(len(s.items))}, nil
}
func (s *noteSource) Find(context.Context, string) (*Note, error) { panic("unused") }
func (s *noteSource) Create(context.Context, *Note) error         { panic("unused") }
func (s *noteSource) Update(context.Context, *Note) error         { panic("unused") }
func (s *noteSource) Delete(context.Context, string) error        { panic("unused") }
func (s *noteSource) ID(*Note) string                              { panic("unused") }
```

`Note` is already declared in `gormsource_test.go` (`type Note struct { ID uint; Body string; DeletedAt gorm.DeletedAt }`), reused here rather than adding yet another fixture type.

Now add to `dashboard_test.go` the one end-to-end test that needs the real HTTP harness together with real Postgres:

```go
func TestDashboardPageRendersRealSeries(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	today := time.Now()
	db.Create(&dashActivity{Name: "a", CreatedAt: today})
	db.Create(&dashActivity{Name: "b", CreatedAt: today})
	db.Create(&dashActivity{Name: "c", CreatedAt: today.AddDate(0, 0, -10)})

	res := Resource[dashActivity](db).Slug("activity").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required())
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	result := h.get("/")
	if result.status != http.StatusOK {
		t.Fatalf("status %d", result.status)
	}
	body := result.body
	i := strings.Index(body, `data-series="`)
	if i < 0 {
		t.Fatal("missing data-series attribute")
	}
	rest := body[i+len(`data-series="`):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("unterminated data-series attribute")
	}
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(rest[:j]), &decoded); err != nil {
		t.Fatalf("decode %q: %v", rest[:j], err)
	}
	xs, ys := decoded[0], decoded[1]
	if len(xs) != 30 || len(ys) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(xs), len(ys))
	}
	if ys[29] != 2 {
		t.Errorf("today's count (index 29) = %d, want 2", ys[29])
	}
	if ys[19] != 1 {
		t.Errorf("10-days-ago count (index 19 = 29-10) = %d, want 1", ys[19])
	}
}

func TestHomeSeriesErrorIsServerError(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	res := Resource[dashActivity](db).Slug("activity").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required())
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	if err := db.Migrator().DropTable(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	result := h.get("/")
	if result.status != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", result.status)
	}
	if !strings.Contains(result.body, "Something went wrong on our side") {
		t.Errorf("expected the generic server-error message, got: %s", result.body)
	}
}
```

Add `"encoding/json"`, `"net/http"`, `"strings"`, and `"github.com/TechnoVizor/tellus/form"`, `"github.com/TechnoVizor/tellus/table"` to `dashboard_test.go`'s import block (`context`, `reflect`, `testing`, `time`, and `internal/testdb` are already there from Task 2).

- [ ] **Step 2: Run the tests to verify they fail for the right reason**

Run: `go test . -run "TestHome|TestDashboard" -v`
Expected: `TestHomeRendersDashboard` FAILs (still redirects, no "Dashboard" text yet). `TestHomeWithNoResourcesShowsMessage` PASSes already (unchanged behavior). `TestHomeCountErrorIsServerError` currently redirects instead of erroring (memSource's `failWith` is never reached, since `home` today never calls `List`), so it FAILs. `TestDashboardNavActiveOnlyOnDashboard` FAILs (no "Dashboard" nav entry exists yet). `TestDashboardCardOrderMatchesRegistration` FAILs to compile until `noteSource` is added, then fails on missing "Notes"/"Products" card text. `TestDashboardCountsAreFreshPerRequest` FAILs (still redirects). `TestDashboardPageRendersRealSeries` and `TestHomeSeriesErrorIsServerError` FAIL (no `data-series` attribute exists yet; status is a redirect, not 500).

- [ ] **Step 3: Wire dashboardCards into resource.go's register()**

`resource.go`'s import block currently reads:

```go
import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/table"
)
```

Add `"context"`:

```go
import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/table"
)
```

Its `register()` currently ends with:

```go
	p.slugs[rs.slug] = true
	p.nav = append(p.nav, navEntry{slug: rs.slug, label: rs.plural})
	p.mounters = append(p.mounters, rs.mount)
	return nil
}
```

Change to:

```go
	p.slugs[rs.slug] = true
	p.nav = append(p.nav, navEntry{slug: rs.slug, label: rs.plural})
	p.dashboardCards = append(p.dashboardCards, dashboardCard{
		label: rs.plural,
		href:  p.url("/" + rs.slug),
		countFn: func(ctx context.Context) (int64, error) {
			res, err := rs.source.List(ctx, ListQuery{PerPage: 1})
			if err != nil {
				return 0, err
			}
			return res.Total, nil
		},
		seriesFn: resolveTimeSeries(b.db, model),
	})
	p.mounters = append(p.mounters, rs.mount)
	return nil
}
```

- [ ] **Step 4: Add dashboardCards to Panel and rewrite shell() and home()**

In `panel.go`, the `Panel` struct currently reads:

```go
type Panel struct {
	cfg     Config
	prefix  string
	signer  *secure.Signer
	csrf    secure.CSRF
	limiter *secure.Limiter
	now     func() time.Time

	mu       sync.Mutex
	built    bool
	handler  http.Handler
	nav      []navEntry
	slugs    map[string]bool
	mounters []func(*http.ServeMux)
}
```

Add `dashboardCards`:

```go
type Panel struct {
	cfg     Config
	prefix  string
	signer  *secure.Signer
	csrf    secure.CSRF
	limiter *secure.Limiter
	now     func() time.Time

	mu             sync.Mutex
	built          bool
	handler        http.Handler
	nav            []navEntry
	slugs          map[string]bool
	mounters       []func(*http.ServeMux)
	dashboardCards []dashboardCard
}
```

The `const` block near the top of `panel.go` currently reads:

```go
const (
	sessionCookie       = "tellus_session"
	defaultMaxBodyBytes = 8 << 20 // comfortably fits one image upload plus its other fields
	loginAttempts       = 10
	loginWindow         = 10 * time.Minute
	defaultPrefix       = "/admin"
	defaultName         = "Tellus"
	defaultSession      = 12 * time.Hour
	defaultUploadDir    = "tellus-uploads"
	defaultUploadURL    = "/uploads"
)
```

Add `dashboardDays`:

```go
const (
	sessionCookie       = "tellus_session"
	defaultMaxBodyBytes = 8 << 20 // comfortably fits one image upload plus its other fields
	loginAttempts       = 10
	loginWindow         = 10 * time.Minute
	defaultPrefix       = "/admin"
	defaultName         = "Tellus"
	defaultSession      = 12 * time.Hour
	defaultUploadDir    = "tellus-uploads"
	defaultUploadURL    = "/uploads"
	dashboardDays       = 30
)
```

`shell()` currently reads:

```go
func (p *Panel) shell(r *http.Request, u User, title, activeSlug string) ui.Shell {
	nav := make([]ui.NavItem, 0, len(p.nav))
	for _, e := range p.nav {
		nav = append(nav, ui.NavItem{Label: e.label, Href: p.url("/" + e.slug), Active: e.slug == activeSlug})
	}
	return ui.Shell{
		Title:    title,
		Brand:    p.cfg.Name,
		Prefix:   p.prefix,
		Nav:      nav,
		UserName: u.DisplayName(),
		CSRF:     secure.CSRFToken(r.Context()),
	}
}
```

Change the nav-building lines:

```go
func (p *Panel) shell(r *http.Request, u User, title, activeSlug string) ui.Shell {
	nav := make([]ui.NavItem, 0, len(p.nav)+1)
	if len(p.dashboardCards) > 0 {
		nav = append(nav, ui.NavItem{Label: i18n.T("ui.dashboard"), Href: p.url("/"), Active: activeSlug == ""})
	}
	for _, e := range p.nav {
		nav = append(nav, ui.NavItem{Label: e.label, Href: p.url("/" + e.slug), Active: e.slug == activeSlug})
	}
	return ui.Shell{
		Title:    title,
		Brand:    p.cfg.Name,
		Prefix:   p.prefix,
		Nav:      nav,
		UserName: u.DisplayName(),
		CSRF:     secure.CSRFToken(r.Context()),
	}
}
```

(`len(p.dashboardCards)` is checked, not `len(p.nav)`, even though the two always have equal length, one appended per registered resource in the same `register()` call: the question here is "is there a dashboard to link to", which is what `p.dashboardCards` answers. `activeSlug == ""` already means "no resource is active": every resource page passes its own `rs.slug`; `renderMessage`, used for a 404 and for the empty-panel message, is the only other caller, always with `""`, so a 404 page will also highlight "Dashboard" in the sidebar. This is a minor, deliberate simplification carried over from the spec: `renderMessage` has no resource context to pass instead, and reworking its signature for this cosmetic nit on an already-rare page is not worth it.)

`home()` currently reads:

```go
func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.nav) > 0 {
		http.Redirect(w, r, p.url("/"+p.nav[0].slug), http.StatusSeeOther)
		return
	}
	p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
}
```

Change to:

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

- [ ] **Step 5: Add DashboardCardView and DashboardView to internal/ui/views.go**

Append, after the existing `FormView` struct:

```go
// DashboardCardView is one resource's stat card on the dashboard.
type DashboardCardView struct {
	Label      string
	Href       string
	Count      int64
	HasSeries  bool
	SeriesJSON string // uPlot's own data shape, [xs[], ys[]]; ignored when HasSeries is false
}

// DashboardView is the data of the dashboard home page.
type DashboardView struct {
	Shell   Shell
	Heading string
	Cards   []DashboardCardView
}
```

- [ ] **Step 6: Create internal/ui/dashboard.templ**

```templ
package ui

import "strconv"

templ DashboardPage(v DashboardView) {
	@Layout(v.Shell) {
		<div class="page-head">
			<h1>{ v.Heading }</h1>
		</div>
		<div class="dashboard-grid">
			for _, c := range v.Cards {
				<a class="card dashboard-card" href={ templ.SafeURL(c.Href) }>
					<div class="card-body">
						<div class="muted">{ c.Label }</div>
						<div class="stat-value">{ strconv.FormatInt(c.Count, 10) }</div>
						if c.HasSeries {
							<div class="dashboard-chart" data-series={ c.SeriesJSON }></div>
						}
					</div>
				</a>
			}
		</div>
	}
}
```

- [ ] **Step 7: Add the dashboard CSS**

In `internal/assets/static/app.css`, add near the existing `.card`/`.card-soft`/`.card-body` rules (inside the `@layer components` block):

```css
.dashboard-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 20px; }
.dashboard-card { display: block; text-decoration: none; transition: border-color 0.15s var(--ease); }
.dashboard-card:hover { border-color: var(--border-strong); }
.stat-value { font-size: 32px; font-weight: 700; letter-spacing: -0.02em; margin-top: 4px; }
.dashboard-chart { width: 100%; height: 60px; margin-top: 12px; }
```

- [ ] **Step 8: Add the ui.dashboard i18n key**

In `internal/i18n/en.go`, in the "layout" group alongside `ui.navigation`/`ui.theme`/`ui.toggle_theme`/`ui.sign_out`/`ui.no_resources`, add:

```go
"ui.dashboard": "Dashboard",
```

- [ ] **Step 9: Add the chart-drawing JS**

In `internal/assets/static/app.js`, append this function near the other top-level helper functions (after `weekdayLabels`, before `document.addEventListener("alpine:init", ...)`):

```js
function initDashboardCharts() {
  var ink = getComputedStyle(document.documentElement).getPropertyValue("--ink").trim();
  document.querySelectorAll(".dashboard-chart").forEach(function (el) {
    el.replaceChildren(); // clear a previous draw, so a theme change can redraw cleanly
    var data = JSON.parse(el.dataset.series);
    new uPlot(
      {
        width: el.clientWidth,
        height: 60,
        series: [{}, { stroke: ink, width: 2, fill: ink + "22" }],
        axes: [{ show: false }, { show: false }],
        legend: { show: false },
        cursor: { show: false },
      },
      data,
      el
    );
  });
}
document.addEventListener("DOMContentLoaded", initDashboardCharts);
```

In the same file, `Alpine.data("themeToggle", ...)`'s `toggle` function currently ends:

```js
      toggle: function () {
        var root = document.documentElement;
        var current =
          root.dataset.theme ||
          (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
        var next = current === "dark" ? "light" : "dark";
        root.dataset.theme = next;
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
      },
```

Add a call to redraw every chart in the new theme's ink color:

```js
      toggle: function () {
        var root = document.documentElement;
        var current =
          root.dataset.theme ||
          (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
        var next = current === "dark" ? "light" : "dark";
        root.dataset.theme = next;
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
        initDashboardCharts();
      },
```

- [ ] **Step 10: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test . -run "TestHome|TestDashboard" -v`
Expected: PASS (all seven tests: `TestHomeRendersDashboard`, `TestHomeWithNoResourcesShowsMessage`, `TestHomeCountErrorIsServerError`, `TestDashboardNavActiveOnlyOnDashboard`, `TestDashboardCardOrderMatchesRegistration`, `TestDashboardCountsAreFreshPerRequest`, `TestDashboardPageRendersRealSeries`, `TestHomeSeriesErrorIsServerError`).

- [ ] **Step 11: Run the whole project's tests, gofmt, and vet**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, no vet errors, every test passes except the one pre-existing unrelated failure, `TestLocalStorageRejectsFilenamesThatEscapeTheDirectory` (Windows-path handling on Linux); this task does not touch it and does not need to fix it.

- [ ] **Step 12: Commit**

```bash
git add resource.go panel.go internal/ui/views.go internal/ui/dashboard.templ internal/ui/dashboard_templ.go internal/assets/static/app.css internal/assets/static/app.js internal/i18n/en.go resource_test.go dashboard_test.go
git commit -m "feat: render an automatic dashboard as the panel's home page"
```

---

## After this plan

`examples/shop` still opens straight to the Products list (its own local navigation, not the panel's own root, since it has one resource today and the demo user goes to `/admin/products` directly). Once `Product`'s existing `CreatedAt` field and the new `Category` resource are both registered, restarting the example already exercises the real dashboard with no further code changes needed; a follow-up conversation can point the demo's own landing link at `/admin/` if that is wanted, that is not part of this plan.
