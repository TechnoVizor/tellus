package table

import (
	"strings"
	"testing"
)

func TestImageColumnRendersThumbnail(t *testing.T) {
	got := cell(t, Image("Photo"), "/uploads/abc.png")
	if !strings.Contains(got, `src="/uploads/abc.png"`) {
		t.Errorf("missing image src: %s", got)
	}
	if !strings.Contains(got, "field-thumb") && !strings.Contains(got, "class=") {
		t.Errorf("missing thumbnail class: %s", got)
	}
}

func TestImageColumnEscapesTheURL(t *testing.T) {
	got := cell(t, Image("Photo"), `"><script>x</script>`)
	if strings.Contains(got, "<script>x</script>") {
		t.Fatal("value was not escaped")
	}
}

func TestImageColumnShowsPlaceholderWhenEmpty(t *testing.T) {
	for _, v := range []any{"", nil} {
		got := cell(t, Image("Photo"), v)
		if strings.Contains(got, "<img") {
			t.Errorf("value %#v: expected a placeholder, not an <img>: %s", v, got)
		}
	}
}

func TestImageColumnInfo(t *testing.T) {
	info := Image("Photo").Label("Picture").Info()
	if info.Name != "Photo" || info.Label != "Picture" {
		t.Errorf("unexpected info: %+v", info)
	}
}
