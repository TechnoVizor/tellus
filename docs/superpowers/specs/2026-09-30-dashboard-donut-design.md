# Dashboard widgets, part 2: a Donut chart widget

Status: approved, 2026-09-30. Source: design conversation with the project owner.

## 1. Goal

`docs/superpowers/specs/2026-09-29-dashboard-widgets-design.md` introduced the
`dashboard.Widget` interface and shipped its first implementation, `Stat`. It
named the donut chart as the next sub-project in the same decomposition. This
spec designs that widget: `dashboard.Donut`, a part-to-whole chart with up to
six labeled, colored segments, a legend, and a center total.

## 2. Scope

In scope: the `dashboard.Donut` builder (`Segments`, `Href`, `Validate`,
`Render`); vendoring Chart.js as the rendering engine; a six-slot categorical
color palette, validated for this project's actual light and dark surfaces;
folding a seventh-and-beyond segment into an "Other" bucket; a custom HTML
legend (not Chart.js's own); a center total; the wiring script that mounts
the chart the same way `initDashboardCharts` already mounts uPlot.

Out of scope: the embedded data table widget (separate spec, next in the
decomposition); any change to `Panel.Dashboard`, `home()`, or the `Widget`/
`Validated` interfaces, all already generic enough for a second
implementation to slot into unchanged; per-segment host-supplied colors
(colors stay assigned by slot position, not configurable, matching every
other color decision in this library); any chart form other than donut for
part-to-whole (considered and rejected here, see section 3); animated
transitions beyond whatever Chart.js does by default; a legend "toggle to
isolate a segment" interaction.

## 3. Why donut, and why a cap of six

The data-viz method this project follows (categorical color, contrast,
CVD-safety) treats donut/pie as a deprioritized chart form: its own
anti-pattern list calls out "a donut/pie for comparing close values" and
recommends a bar chart or the plain numbers instead, describing donut/pie as
acceptable only for "part-to-whole at a glance only, <= 6 segments." Past
roughly seven categories, adjacent color classes stop being distinguishable
at a glance regardless of the palette.

The owner confirmed donut anyway, matching the reference screenshots and the
prior spec's decomposition, with the six-segment cap this method requires.
`Segments` accepts any length; the widget itself folds the seventh and later
entries into a single "Other" slice (their values summed) rather than
rendering an unreadable eight-plus-color wheel or erroring on data shape the
host cannot always predict at registration time (see section 5).

## 4. Public API

```go
panel.Dashboard(
	dashboard.Donut("Orders by Status").
		Segments(func(ctx context.Context) ([]dashboard.Segment, error) {
			var rows []struct {
				Status string
				N      float64
			}
			db.Model(&Order{}).Select("status, count(*) as n").Group("status").Scan(&rows)
			segs := make([]dashboard.Segment, len(rows))
			for i, r := range rows {
				segs[i] = dashboard.Segment{Label: r.Status, Value: r.N}
			}
			return segs, nil
		}).
		Href("/admin/orders"),
)
```

```go
// package dashboard, alongside Widget, Validated, Stat.

// Segment is one labeled slice of a Donut's data. Value is assumed
// non-negative, matching the reference use (counts, sums, shares); a host
// returning a negative value gets an undefined visual result, not a
// validation error, the same trust boundary Value/Trend already have on
// Stat.
type Segment struct {
	Label string
	Value float64
}

func Donut(label string) *DonutWidget
func (w *DonutWidget) Segments(fn func(ctx context.Context) ([]Segment, error)) *DonutWidget
func (w *DonutWidget) Href(url string) *DonutWidget
func (w *DonutWidget) Validate() error
func (w *DonutWidget) Render(ctx context.Context) (templ.Component, error)
```

`Donut(label)` starts the widget; `label` is its heading, shown above the
chart the same place `Stat`'s label sits above its value. `Segments` is
required, checked by `Validate()` the same way `Stat.Value` is:
`fmt.Errorf("dashboard.Donut(%q) needs .Segments(...)", label)` when never
called. `Href` is optional, identical contract to `Stat.Href`: the whole card
becomes a link with a trailing chevron, no other visual change.

`DonutWidget` satisfies `Validated` (so `Widget`) exactly like `StatWidget`;
`Panel.Dashboard` needs no change; its existing `w.(dashboard.Validated)`
type-assertion already covers any widget that implements the interface.

## 5. Segment count: the six-slot cap and the Other bucket

`Render` sorts nothing (the host's order is the display and slot-assignment
order, same principle as `Stat`'s host-owned formatting) and takes the first
six entries of whatever `Segments` returns that request. A seventh-and-later
entry does not get its own color; instead, one synthetic final segment is
appended: label `i18n.T("dashboard.other")` ("Other"), value the sum of every
folded entry's `Value`. A `Segments` result of six or fewer is untouched, no
"Other" slice appears.

This check happens inside `Render`, once per request, not inside `Validate`:
segment count comes from the host's data, which can change between requests
(an eighth order status can appear tomorrow), unlike `Stat`'s `Value` being
set or not, which is fixed the moment `Dashboard(...)` is called. There is
nothing to reject at startup here, so `Validate()` for `Donut` only checks
that `Segments` itself was set, mirroring `Stat.Validate` exactly.

An empty result (zero segments, or every value zero, total `== 0`) renders a
single flat, muted ring with no legend rows and no center total, labeled
`i18n.T("dashboard.no_data")` ("No data") in place of the total, rather than
dividing by zero for each slice's percentage or drawing nothing. This mirrors
the automatic dashboard's own comfort with a zero count on a `Stat`-style
card; a `Donut` with nothing to show is not an error.

## 6. Color: a new six-slot categorical palette

`docs/spec.md` section 4.2 keeps this panel's colors fixed and
non-configurable; the only exception before this spec was the single
`--success`/`--danger` pair on `Stat`'s trend badge. A donut chart needs
several mutually distinguishable hues at once, which is a bigger exception
than one badge, so it gets its own validated palette rather than reusing or
extending the trend colors.

Six new tokens, `--chart-1` through `--chart-6`, added to `app.css` alongside
the existing token block, light and dark:

```css
/* light */
--chart-1: #2a78d6; /* blue */
--chart-2: #eb6834; /* orange */
--chart-3: #1baf7a; /* aqua */
--chart-4: #eda100; /* yellow */
--chart-5: #e87ba4; /* magenta */
--chart-6: #008300; /* green */
/* dark, both dark blocks */
--chart-1: #3987e5;
--chart-2: #d95926;
--chart-3: #199e70;
--chart-4: #c98500;
--chart-5: #d55181;
--chart-6: #008300;
```

These are six slots of the data-viz method's own validated reference
palette (its documented order, unchanged: blue, orange, aqua, yellow,
magenta, green), re-validated here against this project's actual surfaces
(`--canvas`/`--card` `#ffffff` light, `#151517` dark) with
`validate_palette.js`:

- Light (`--surface #ffffff`): lightness band, chroma floor, CVD separation
  (worst adjacent ΔE 9.1), and normal-vision floor (worst adjacent ΔE 19.6)
  all PASS. Contrast vs. surface WARNs for three of the six hues (aqua,
  yellow, magenta land under 3:1 on white). This is the method's documented
  "relief" band: legal only when color never carries meaning alone. Section
  7's mandatory legend (always visible, always paired with the numeric
  value) is that relief; nothing in this widget relies on a segment's color
  by itself to convey which segment is which.
- Dark (`--surface #151517`): every check, including contrast, PASSes
  outright.

The "Other" bucket (section 5) uses `--ink-muted`, already a token, not a
seventh chart color: it is a deliberate de-emphasis, not a seventh category
competing for attention, matching the method's own "de-emphasis / Other" as
a distinct color role from the categorical set.

## 7. Rendering: Chart.js, the legend, and the center total

### 7.1 Vendoring

Chart.js **4.5.1** (MIT, confirmed current), the UMD build,
`dist/chart.umd.min.js`, fetched once from
`https://cdn.jsdelivr.net/npm/chart.js@4.5.1/dist/chart.umd.min.js` the same
way uPlot was: committed to `internal/assets/static/chart.umd.min.js`,
license text added to `THIRD_PARTY_NOTICES.md` alongside uPlot, htmx,
Alpine, and Inter Variable. The file is about 204 KB minified, roughly 4x
uPlot's own footprint; the project already accepted a non-trivial vendored
file size for one chart engine, and there is no lighter well-maintained
option that draws a donut with a built-in accessible hover tooltip for free
(section 7.3), so this is an accepted, explicit tradeoff, not an oversight.
Chart.js ships no separate CSS file (unlike uPlot), so nothing parallel to
`uPlot.min.css` is needed.

### 7.2 Markup and data flow

`dashboard` gets its own `render.templ` addition (alongside `statWidget`),
following the same shape section 6 of the `Stat` spec already used:

```templ
templ donutWidget(w *DonutWidget, segs []resolvedSegment, total float64, href string) {
	if href != "" {
		<a class="card dashboard-card" href={ templ.SafeURL(href) }>
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
			<div class="donut-total">
				if total == 0 {
					{ i18n.T("dashboard.no_data") }
				} else {
					{ formatTotal(total) }
				}
			</div>
		</div>
		if total > 0 {
			<ul class="donut-legend">
				for _, s := range segs {
					<li>
						<span class="donut-swatch" style={ "background:" + s.color }></span>
						{ s.label }
						<span class="muted">{ formatTotal(s.value) }</span>
					</li>
				}
			</ul>
		}
	</div>
}
```

`resolvedSegment{label string, value float64, color string}` is a small,
package-private render-time struct built by `Render` from the host's
`[]Segment` plus the fixed six-slot palette (plus the "Other" fold, section
5); `segmentsJSON` marshals `[]{label, value, color}` for Chart.js to
consume, the same "compute in Go, hand JSON to the browser" split
`sparklineJSON` already uses for the automatic dashboard's chart. `color` is
read from the CSS custom property by name (`var(--chart-1)` etc.) at draw
time in JavaScript, not baked into the JSON as a literal hex, so a theme
change repaints with the right set without a server round-trip (section
7.4). `formatTotal` is `strconv.FormatFloat(v, 'f', -1, 64)`, the same
one-line formatting `Stat`'s `trendValue.text` already does; duplicated
rather than factored out; two call sites do not earn a shared helper yet.

Label and value in the legend and total render through templ's normal
escaping, the same as `Stat`'s label and value; nothing in this widget
touches `templ.Raw`, unlike `Stat`'s icon, so there is no equivalent escaping
hazard here.

### 7.3 Chart.js configuration and the legend split

Chart.js draws the ring only: `type: "doughnut"`, `plugins: { legend: {
display: false }, tooltip: { enabled: true } }`. Its own built-in legend is
turned off; the `<ul class="donut-legend">` markup above is the legend,
server-rendered HTML using the panel's own text tokens (`.muted`, body
font), not Chart.js's canvas-drawn text, so it matches every other list in
the panel instead of picking up Chart.js's default font and never
respecting `--ink`/dark mode on its own. The hover tooltip is left as Chart.
js's own default (dark pill, white text) for this first version: the data-
viz method calls hover a default expectation, not optional, and Chart.js's
unstyled tooltip already reads fine over the card background in both
themes; restyling it to match panel tokens exactly is a small follow-up, not
blocking this spec.

`cutout: "60%"` (a donut, not a filled pie) leaves room for the center
total, an absolutely-positioned `.donut-total` div centered over the
canvas's hole with plain CSS (`.donut-wrap { position: relative }`,
`.donut-total { position: absolute; inset: 0; display: grid;
place-items: center; }`), not a Chart.js plugin: the total is already known
server-side (section 5), so this is a CSS overlay, not something that needs
canvas drawing code.

### 7.4 Wiring into app.js

A second `initDashboardCharts`-style function, `initDonutCharts`, added to
`app.js` next to the existing one, called from the same `DOMContentLoaded`
and theme-toggle listeners:

```js
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
```

`s.color` in the JSON payload is the custom-property name (for example
`"--chart-1"`), resolved through `getComputedStyle` at draw time, so
`initDonutCharts` re-reads the right hex for whichever theme is active,
exactly parallel to how `initDashboardCharts` already re-reads `--ink` on
every call. The existing `themeToggle` Alpine component's `toggle()` already
calls `initDashboardCharts()` after flipping `data-theme`; it gains a second
call to `initDonutCharts()` right after it.

### 7.5 CSS

New rules in `internal/assets/static/app.css`'s components layer, alongside
the `Stat` rules already there:

```css
.donut-wrap { position: relative; width: 140px; height: 140px; margin: 12px auto 0; }
.donut-total { position: absolute; inset: 0; display: grid; place-items: center; pointer-events: none; font-size: 20px; font-weight: 700; }
.donut-legend { list-style: none; margin: 12px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
.donut-legend li { display: flex; align-items: center; gap: 8px; }
.donut-legend li .muted { margin-left: auto; }
.donut-swatch { display: inline-block; width: 10px; height: 10px; border-radius: 2px; flex-shrink: 0; }
```

## 8. i18n

Two new keys, alongside `dashboard.trend_up`/`dashboard.trend_down` in
`internal/i18n/en.go`:

```go
"dashboard.other":   "Other",
"dashboard.no_data": "No data",
```

## 9. Testing

- `dashboard` package: `Donut`'s builder chain sets internal state; `Validate`
  rejects a `Donut` with no `Segments` and accepts one that has it (mirrors
  `TestStatValidateRequiresValue`/`TestStatValidateErrorNamesTheWidget`);
  `Render` propagates a `Segments` error without rendering anything (mirrors
  `TestStatRenderPropagatesValueError`); a `Segments` result of six or fewer
  renders every label and value, escaped, with no "Other" row; a result of
  seven or more renders exactly six named segments plus one "Other" row
  whose value is the summed remainder; an empty or all-zero result renders
  the no-data state with no legend rows and no division-by-zero panic; a
  `Href` produces the link wrapper and chevron, no `Href` produces the plain
  card (mirrors `TestStatRenderChevronOnlyWithHref`).
- Root package, through the existing HTTP harness: `Panel.Dashboard(...)`
  with a well-formed `Donut` alongside a `Stat` renders both on `GET /`
  (proving `Widget` already composes across implementations with no `Panel`
  change); a `Donut` with no `Segments` returns a `Dashboard(...)` error
  naming the widget, the same shape `TestDashboardMethodRejectsMisconfiguredStat`
  already proves for `Stat`; a `Segments` function returning an error
  surfaces as the generic 500 page (mirrors
  `TestDashboardWidgetValueErrorIsServerError`).
- `internal/assets`: extend `TestVendoredLibrariesAreReallyEmbedded` with
  `/chart.umd.min.js` (well over the existing 10 KB threshold).

## 10. Deferred

The embedded data table widget (separate spec, next in the decomposition,
per the part-1 spec's section 1); per-segment host-supplied colors; any
chart form other than donut for part-to-whole; a styled/re-themed Chart.js
tooltip; a legend "click to isolate a segment" interaction; animating the
donut on data change (Chart.js's own defaults apply, untouched, unexamined
here); a table-view accessible fallback beyond the legend (the method calls
for one on larger charts; a six-slice dashboard card with a numeric legend
next to every slice already gives the same information a table would, so
one is not added here, revisit if a future widget's data is large enough
that this stops holding); any per-user or per-role widget visibility
(`Stat`'s spec already deferred this for the whole `Widget` system, still
true).
