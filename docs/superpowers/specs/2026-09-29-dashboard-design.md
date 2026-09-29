# Dashboard home page: design

Status: approved, 2026-09-29. Source: design conversation with the project owner.

## 1. Goal

The panel currently has no home page: `GET /admin/` redirects to the first
registered resource's list, or shows a plain message when nothing is
registered (`panel.go`'s `home`). This spec replaces that redirect with an
automatic dashboard: a stat card per registered resource (a count, and where
the model supports it, a 30-day sparkline of new records), requiring zero
extra configuration from the host beyond the `Register(...)` calls already
made today. `docs/spec.md` section 9 lists "dashboard and widgets" as later
work; this is the first slice of it, deliberately narrow (see section 8).

## 2. Scope

In scope: the dashboard as the new home page, one stat card per registered
resource showing its row count, and a 30-day daily new-record sparkline for
a resource whose model has a `CreatedAt time.Time` field (GORM's own
timestamp convention). A "Dashboard" link in the sidebar, above the resource
links, so a user can navigate back to it.

Out of scope for this slice (see section 8 for the full list): host-defined
custom widgets or metrics, any aggregation other than a count (sum, average,
min, max), a configurable time range (30 days is fixed), per-resource
opt-out, drill-down from a chart point to a filtered list, and date bucketing
for a database other than Postgres.

## 3. Public API and routing

No new builder methods. `Panel.home` (`panel.go`) changes from:

```go
func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.nav) > 0 {
		http.Redirect(w, r, p.url("/"+p.nav[0].slug), http.StatusSeeOther)
		return
	}
	p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
}
```

to rendering the dashboard when at least one resource is registered, and
keeping today's message page when none are. The route itself
(`GET /{$}`, already wired to `p.authed(p.home)`) does not change.

**Sidebar nav.** `Panel.shell` (`panel.go`) builds `Shell.Nav` from `p.nav`,
which today holds only resource entries. It gains one more entry, prepended,
only when `len(p.nav) > 0`:

```go
nav := make([]ui.NavItem, 0, len(p.nav)+1)
if len(p.dashboardCards) > 0 {
	nav = append(nav, ui.NavItem{Label: i18n.T("ui.dashboard"), Href: p.url("/"), Active: activeSlug == ""})
}
for _, e := range p.nav {
	nav = append(nav, ui.NavItem{Label: e.label, Href: p.url("/" + e.slug), Active: e.slug == activeSlug})
}
```

(`p.dashboardCards` is checked here, not `p.nav`, even though the two
always have the same length, one appended per registered resource in the
same `register()` call: the question this guards is "is there a dashboard
to link to", which section 4 answers, not "are there resources".)

`activeSlug == ""` already means "no resource is active": every resource
page passes its own `rs.slug`, and `renderMessage` (used for a 404 and for
the empty-panel message) is the only other caller, always with `""`. A 404
page will therefore also highlight "Dashboard" in the sidebar. This is a
minor, deliberate simplification: `renderMessage` has no resource context to
pass instead, reworking its signature to carry one is not worth it for a
cosmetic nit on an already-rare page.

## 4. Per-resource data, gathered once at registration

A `resource[T].register()` already reflects over the model with its own
concrete `T`; a `dashboardCard` is where it hands the panel a plain,
non-generic way to ask for that resource's numbers later, the same pattern
`optionLoaders` already uses for a `Select` field's relation (`resource.go`,
`relation.go`):

```go
// dashboardCard is what one registered resource contributes to the
// dashboard home page. Built once at Register time, called fresh on every
// request to home, the same split ResourceBuilder already has between
// register-time reflection and request-time data access.
type dashboardCard struct {
	label    string // rs.plural
	href     string // the resource's list URL
	countFn  func(ctx context.Context) (int64, error)
	seriesFn func(ctx context.Context, days int) ([]dailyCount, error) // nil when unavailable
}

type dailyCount struct {
	Day   time.Time
	Count int64
}
```

`countFn` works for every resource, GORM-backed or not, by reusing the
`DataSource[T]` interface already in place:

```go
countFn: func(ctx context.Context) (int64, error) {
	res, err := rs.source.List(ctx, ListQuery{PerPage: 1})
	if err != nil {
		return 0, err
	}
	return res.Total, nil
},
```

`seriesFn` is GORM-only, resolved once in `register()` by a new
`resolveTimeSeries(db *gorm.DB, model reflect.Type) func(ctx context.Context, days int) ([]dailyCount, error)`
in a new `dashboard.go` (root package, alongside `relation.go`). It returns
`nil` when `db` is `nil` (a custom `Source` with no `*gorm.DB`, the same
GORM-only stance relations already take) or the model's schema has no
`CreatedAt` field of type `time.Time` with a real column. When available,
the returned closure runs one query, grouping by day and reusing
`Statement.Quote` for the column name exactly as `gormsource.go`'s `List`
already does for its own columns:

```go
func(ctx context.Context, days int) ([]dailyCount, error) {
	since := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	var rows []struct {
		Day   time.Time
		Count int64
	}
	err := db.WithContext(ctx).Table(table).
		Select("date_trunc('day', " + quotedCol + ") AS day, COUNT(*) AS count").
		Where(quotedCol+" >= ?", since).
		Group("day").Order("day").
		Find(&rows).Error
	...
}
```

`table` and `quotedCol` are resolved once from the schema at registration,
never from a request, so building the query string from them is safe, the
same argument already made for `relation.go`'s loader. `date_trunc` is
Postgres-specific; a `// ponytail:` comment notes this the same way
`gormsource.go` already flags its Postgres-only `ILIKE` search, since both
are the same "Postgres first" tradeoff (`docs/spec.md` section 5).

The result is sparse (only days with at least one row). Gap-filling into a
dense 30-point series happens later, in section 5, so this function stays
testable as pure SQL correctness, independent of presentation.

`resource[T].register()` builds and appends its card near the end of the
existing per-field loop:

```go
p.dashboardCards = append(p.dashboardCards, dashboardCard{
	label:    rs.plural,
	href:     p.url("/" + rs.slug),
	countFn:  ...,
	seriesFn: resolveTimeSeries(b.db, model),
})
```

`Panel` gains `dashboardCards []dashboardCard`, appended to in registration
order, the same order `p.nav` already uses.

## 5. The dashboard page

`Panel.home`, when `len(p.dashboardCards) > 0`, gathers fresh numbers on
every request (never cached, matching every other page in the panel) and
renders a new `ui.DashboardPage`:

```go
type DashboardCardView struct {
	Label      string
	Href       string
	Count      int64
	HasSeries  bool
	SeriesJSON string // uPlot's own data shape, [xs[], ys[]], or "" when HasSeries is false
}

type DashboardView struct {
	Shell   Shell
	Heading string
	Cards   []DashboardCardView
}
```

Gap-filling turns a sparse `[]dailyCount` into a dense 30-point series
before JSON-encoding, a pure function independent of the database:

```go
// sparklineJSON fills any day with no rows to zero, so the chart's x-axis
// spacing is even, then encodes uPlot's own [xs[], ys[]] shape.
func sparklineJSON(rows []dailyCount, days int) string
```

A load failure (`countFn` or `seriesFn` returning an error) goes through
`p.serverError`, the same generic-500 path every other page already uses
for a `DataSource`/GORM failure.

## 6. Rendering: layout and uPlot

The card grid reuses the existing token vocabulary, no new colors:

```css
.dashboard-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 20px; }
.stat-value { font-size: 32px; font-weight: 700; letter-spacing: -0.02em; }
.dashboard-chart { width: 100%; height: 60px; }
```

Charts render with **uPlot 1.6.32** (MIT, confirmed the current release),
chosen for its size (about 12 KB gzipped) and plain, unopinionated default
look, the closest fit to htmx and Alpine's own "small, does one thing"
footprint already embedded the same way. Two files, `uPlot.iife.min.js` and
its companion `uPlot.min.css`, are fetched once from
`https://unpkg.com/uplot@1.6.32/dist/`, committed under
`internal/assets/static/`, and served through the same `assets.Handler()`
every other embedded file already goes through. Both go in
`THIRD_PARTY_NOTICES.md` and the M0 plan's "Pinned versions" line, alongside
htmx 2.0.11, Alpine.js 3.17.4, and Inter Variable.

Each card carries its series as a `data-series` attribute (the same
data-attribute convention `datePicker` already uses for `data-value`). A
plain function in `app.js`, not an Alpine component (uPlot needs no
reactivity, just a one-time draw per element), initializes every chart on
the page:

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

`themeToggle` (already in `app.js`) calls `initDashboardCharts()` again
after it flips `data-theme`, so every chart redraws in the new theme's ink
color; redrawing a handful of 60px sparklines is cheap enough that a full
teardown and rebuild, rather than a more careful in-place color update, is
the right amount of engineering for this.

## 7. i18n

One new key, `ui.dashboard`, "Dashboard", used for both the nav label and
the page heading.

## 8. Deferred

Host-defined custom widgets or metrics, non-count aggregations, a
configurable time range, per-resource dashboard opt-out, drill-down from a
chart point to its filtered list, a real-time or auto-refreshing dashboard,
per-user dashboard permissions (every signed-in panel user already sees
every registered resource today, unchanged here), and date bucketing for a
database other than Postgres.

## 9. Testing

- Root package: `resolveTimeSeries` against real Postgres (`internal/testdb`)
  for a model with `CreatedAt`, confirming daily grouping and the 30-day
  window; `nil` returned for a model without `CreatedAt`, and for a `nil`
  db. `sparklineJSON`'s gap-filling is a pure unit test, no database.
- Root package, through the existing HTTP harness: registering one or more
  resources makes `GET /` render the dashboard (200, not a redirect) with
  each resource's label and count; registering none keeps today's message
  page; a `countFn`/`seriesFn` error surfaces as the generic 500 page, not
  raw error text, mirroring `TestSourceErrorsAreHiddenFrom500Pages`. The
  sidebar shows "Dashboard" first and marks it active only on the dashboard
  page itself, not on a resource's own list page.
