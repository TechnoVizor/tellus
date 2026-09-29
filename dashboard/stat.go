package dashboard

import (
	"context"
	"fmt"
	"strconv"

	"github.com/a-h/templ"
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
