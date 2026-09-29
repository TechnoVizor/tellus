package dashboard

import (
	"bytes"
	"context"
	"errors"
	"math"
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

func TestStatRenderTrendRoundsToOneDecimalPlace(t *testing.T) {
	w := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return 100.0 / 3, true, nil })
	out := render(t, w)
	if !strings.Contains(out, "33.3%") {
		t.Errorf("expected a trend rounded to one decimal place: %s", out)
	}
	if strings.Contains(out, "33.33") {
		t.Errorf("trend text was not rounded: %s", out)
	}
}

func TestStatRenderTrendShowsMagnitudeNotSign(t *testing.T) {
	w := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return -12, false, nil })
	out := render(t, w)
	if !strings.Contains(out, "trend-down") || !strings.Contains(out, "12%") {
		t.Errorf("expected the down arrow alone to carry direction: %s", out)
	}
	if strings.Contains(out, "-12%") {
		t.Errorf("a negative percent must not double up with the down arrow: %s", out)
	}
}

func TestStatRenderTrendHiddenWhenNaN(t *testing.T) {
	w := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return math.NaN(), true, nil })
	out := render(t, w)
	if strings.Contains(out, "trend-up") || strings.Contains(out, "trend-down") {
		t.Errorf("a non-finite trend value must render no badge: %s", out)
	}
}

func TestStatRenderTrendHiddenWhenInfinite(t *testing.T) {
	w := Stat("X").
		Value(func(context.Context) (string, error) { return "1", nil }).
		Trend(func(context.Context) (float64, bool, error) { return math.Inf(1), true, nil })
	out := render(t, w)
	if strings.Contains(out, "trend-up") || strings.Contains(out, "trend-down") {
		t.Errorf("a non-finite trend value must render no badge: %s", out)
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
