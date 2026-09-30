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
