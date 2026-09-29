# Dashboard widgets, part 1: a Widget API and Stat cards

Status: approved, 2026-09-29. Source: design conversation with the project owner.

## 1. Goal

The dashboard shipped in `docs/superpowers/specs/2026-09-29-dashboard-design.md` is
fully automatic: one stat card per registered resource, zero host
configuration, nothing else. The owner wants a richer dashboard, shown two
reference CRM/analytics screenshots with icon+trend stat cards, a donut
chart, ranked lists, progress-bar lists, and data tables embedded directly
in the dashboard. That is too large for one spec (`docs/spec.md` section 9
already calls dashboard widgets and a plugin system later-phase work); it is
decomposed into an ordered sequence of sub-projects, each its own
spec/plan/implementation cycle:

1. **This spec:** a `dashboard.Widget` API and one concrete widget,
   `dashboard.Stat` (icon, a host-computed value, an optional host-computed
   trend, an optional link).
2. A donut chart widget (later spec).
3. An embedded, sortable/filterable data table widget (later spec).

Each later sub-project reuses the `Widget` interface this spec introduces;
none of them are designed here.

## 2. Scope

In scope: the `dashboard` package and its `Widget` interface; `dashboard.Stat`
with a host-supplied icon, value function, and trend function; wiring a new
`Panel.Dashboard(widgets ...dashboard.Widget) error` that, when called,
fully replaces the automatic per-resource dashboard from the previous spec;
registration-time validation (a `Stat` with no `Value` fails at `Dashboard()`,
not at first request); rendering on the existing `.card`/`.dashboard-grid`
tokens, plus one new, deliberate color exception (a colored trend badge, see
section 6).

Out of scope for this slice: the donut and table widgets (separate specs),
any change to the automatic dashboard's own behavior when `Dashboard(...)`
is never called (it keeps working exactly as shipped), computing a trend
automatically from a resource's own data (the host always supplies both the
value and the trend, see section 4), a built-in icon set (the host always
supplies raw SVG), and any drag-and-drop or grid-layout configuration for
widget placement (widgets render in the order passed to `Dashboard(...)`,
in the same auto-fit grid the automatic dashboard already uses).

## 3. Public API

```go
panel.Dashboard(
	dashboard.Stat("Email Sent").
		Icon(`<svg viewBox="0 0 16 16" fill="none">...</svg>`).
		Value(func(ctx context.Context) (string, error) {
			return "1,251 Mail", nil
		}).
		Trend(func(ctx context.Context) (percent float64, up bool, err error) {
			return 12, true, nil
		}).
		Href("/admin/mail"),
	dashboard.Stat("Ongoing Task").
		Value(func(ctx context.Context) (string, error) {
			var n int64
			db.Model(&Task{}).Where("done = ?", false).Count(&n)
			return fmt.Sprintf("%d Task", n), nil
		}),
)
```

`Stat(label string)` starts a widget. `Value` is required, a function the
host writes however it needs to, with no assumption it relates to a Tellus
`Resource` at all, matching the reference screenshots' own metrics ("Email
Sent" has no registered-resource table behind it). `Icon` takes raw SVG
markup, trusted verbatim: this is code the host author writes, the same
trust boundary as every other builder call in this library, not end-user
input at any point. `Trend` and `Href` are both optional. A `Stat` with no
`Icon` renders without one; a `Stat` with no `Href` renders as a plain card,
not a link, with no chevron.

`Panel.Dashboard(widgets ...dashboard.Widget) error` is a new method, called
the same way `Register` already is, before `Handler()`/`Mount()`. Calling it
at all switches the home page from the automatic per-resource dashboard to
exactly what was passed in, rendered in that order. A panel that never calls
`Dashboard` keeps today's automatic behavior unchanged, this is an additive,
opt-in escalation, not a breaking change to what already shipped.

## 4. The Widget interface

```go
// package dashboard, a new top-level package, sibling to form and table.

// Widget is one piece of the dashboard's home page. A host builds a
// dashboard from a list of these; Stat is the only implementation in this
// phase, more follow in later specs.
type Widget interface {
	// Render draws the widget for one request. ctx carries the request's
	// context, so a widget's own data-fetching functions (for example
	// Stat's Value and Trend) can use it the same way a DataSource already
	// does.
	Render(ctx context.Context) (templ.Component, error)
}
```

`Panel.home` (`panel.go`), when `len(p.dashboardWidgets) > 0`, loops over
them, calls `Render(ctx)` on each, collects the resulting components, and
renders the page; the first error from any widget's `Render` goes through
`p.serverError`, the same generic-500 path every other page already uses,
exactly the contract the automatic dashboard's `countFn`/`seriesFn` already
follow. This keeps `Panel` itself ignorant of what a `Stat` widget is or
needs, the same separation `form.Field` already gives resource forms: the
panel only knows the interface, not the concrete type.

Validation happens once, inside `Dashboard(...)`, not at first request,
matching the "a typo is reported at startup" guarantee the rest of this
library already gives (`README.md`). `Widget` itself stays minimal (only
`Render`); a `Stat` that has no `Value` set is instead caught through an
optional interface, the same shape `form.FileField` already uses for
`ImageField` (`form/file.go`): the extra interface lives in the package that
defines it, `dashboard`, exported, embedding `Widget`, and the root package
type-asserts against it. A Go interface's unexported method names are
scoped to the package that declares them, so a check interface declared
inside the root `tellus` package could never actually be satisfied by a
method `dashboard.StatWidget` defines, that pairing would compile but the
assertion would silently always fail; exporting it in `dashboard`, exactly
like `FileField`, is not a style choice, it is the only version that works:

```go
// in package dashboard, alongside Widget:

// Validated is implemented by a widget with configuration Dashboard checks
// before the panel starts, the same "reported at startup" guarantee the
// rest of this library already gives. Stat implements it.
type Validated interface {
	Widget
	Validate() error
}
```

```go
// in the root tellus package, panel.go, mirroring exactly how
// resource.go already asserts f.(form.FileField):
func (p *Panel) Dashboard(widgets ...dashboard.Widget) error {
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

`dashboard.StatWidget.Validate()` returns
`fmt.Errorf("dashboard.Stat(%q) needs .Value(...)", label)` when `Value` was
never called, `nil` otherwise.

## 5. Panel changes

`Panel` gains `dashboardWidgets []dashboard.Widget`. `home()`'s existing
automatic-card branch is kept, wrapped in a check for the new field:

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
	// ...unchanged automatic-card rendering from the previous spec...
}
```

`internal/ui` gains `WidgetDashboardView{Shell, Heading, Widgets []templ.Component}`
and a `WidgetDashboardPage` templ, structurally identical to the existing
`DashboardPage`/`DashboardView` except it takes already-rendered components
instead of building `<option>`-style markup itself, since each widget now
owns its own rendering (section 6). The existing `DashboardView`/`DashboardPage`
(automatic mode) are untouched.

## 6. Rendering a Stat widget

`dashboard` owns its own `.templ` file, the same way `form` and `table`
already own `render.templ`, rather than putting widget markup in
`internal/ui`. `trendValue` is a small package-private struct holding what
`Render` resolved from the host's `Trend` function, already formatted, so
the template itself does no formatting:

```go
// trendValue is Trend's resolved, display-ready result: the sign is baked
// into text (for example "12%"), up says which arrow and color to use.
type trendValue struct {
	up   bool
	text string
}
```

```templ
// dashboard/render.templ
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
					if trend.up { ▲ } else { ▼ }
				</span>
				{ trend.text }
				<span class="sr-only">
					if trend.up { { i18n.T("dashboard.trend_up") } } else { { i18n.T("dashboard.trend_down") } }
				</span>
			</span>
		}
	</div>
}
```

`templ.Raw(w.icon)` is safe here specifically because the icon string is
Go code the host wrote, never request or database data; this is the one
place in this spec that intentionally bypasses templ's usual escaping, and
it is called out here so a future reader does not "fix" it into an escaped
`{ w.icon }` (which would show literal angle brackets instead of an icon).

Reused unchanged: `.card`, `.card-body`, `.dashboard-card`, `.dashboard-grid`,
`.stat-value`, `.muted`, `.sr-only`. New, small additions to
`internal/assets/static/app.css`:

```css
.stat-icon { display: block; width: 20px; height: 20px; color: var(--ink-muted); margin-bottom: 8px; }
.stat-icon svg { width: 100%; height: 100%; }
.stat-chevron { position: absolute; top: 20px; right: 20px; color: var(--ink-muted); }
.trend { display: inline-flex; align-items: center; gap: 4px; margin-top: 8px; padding: 2px 8px; border-radius: var(--radius-pill); font-size: 12px; font-weight: 600; }
.trend-up { color: var(--success); background: color-mix(in srgb, var(--success) 12%, transparent); }
.trend-down { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
```

`dashboard.Stat` making a card `position: relative` (needed for
`.stat-chevron`'s absolute placement) is a small addition to `.dashboard-card`
itself, harmless for the automatic dashboard's own cards, which simply never
have a `.stat-chevron` child.

One deliberate, explicit exception to the panel's otherwise monochrome
design (`docs/spec.md` section 4.2, "Colors and tokens are not
user-configurable"): the owner asked for a colored trend badge, matching
the reference screenshots, rather than a monochrome arrow-only indicator.
This needs one new token pair, following the exact light/dark shape
`--danger` already has:

```css
:root { /* light, alongside the existing --danger: #c13515; */
	--success: #1a7d3a;
}
/* dark, in both places --danger: #f26d5b; already appears */
--success: #4ade80;
```

`--danger` is reused as-is for the "down" trend, so this spec adds exactly
one new token, not a new palette.

## 7. i18n

Two new keys, for the trend arrow's screen-reader text (the arrow glyph
itself is `aria-hidden`, decorative; the direction needs to reach assistive
technology some other way, the same reasoning `table.BooleanColumn` already
follows for its yes/no badge):

```go
"dashboard.trend_up":   "Trending up",
"dashboard.trend_down": "Trending down",
```

## 8. Testing

- `dashboard` package: `Stat`'s builder chain sets the right internal state;
  `Validate` rejects a `Stat` with no `Value` and accepts one that
  has it; `Render` calls `Value` (and `Trend`, when set) with the given
  context, propagates either function's error without rendering anything,
  and produces markup containing the resolved value, the icon's raw SVG
  verbatim (proving it is not escaped), the trend arrow and text with the
  right `trend-up`/`trend-down` class, and a `.stat-chevron` only when
  `Href` was set. A value or label containing HTML-special characters comes
  back escaped (unlike the icon), the same escaping guarantee every other
  field/column already gives.
- Root package, through the existing HTTP harness: `Panel.Dashboard(...)`
  with a well-formed `Stat` renders it on `GET /`, replacing the automatic
  cards; a panel that registers resources but never calls `Dashboard`
  renders exactly as the previous spec already proved (regression check, not
  new behavior); `Dashboard(...)` with a `Stat` missing `Value` returns an
  error mentioning the label, and the panel that would have used it never
  builds; a `Value` or `Trend` function returning an error surfaces as the
  generic 500 page, not raw text, mirroring the automatic dashboard's own
  `countFn`/`seriesFn` error tests.

## 9. Deferred

The donut and embedded-table widgets (separate specs, per section 1); a
built-in icon set; computing a trend automatically from a resource's own
data; a `dashboard.ResourceStat[T]` convenience constructor that would wire
a `Stat` to an existing `Resource[T]`'s count (a host can already write this
by hand in a few lines, and nothing here blocks adding the sugar later, it
is left out for now as unrequested scope); widget layout/ordering
configuration beyond registration order; and any per-user or per-role
control over which widgets a signed-in user sees, the panel has no
per-resource or per-widget authorization concept at all yet.
