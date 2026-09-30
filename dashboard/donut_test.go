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
