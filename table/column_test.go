package table

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func cell(t *testing.T, c Column, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Cell(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTextColumnInfoAndDefaults(t *testing.T) {
	info := Text("CreatedAt").Searchable().Sortable().Info()
	if info.Name != "CreatedAt" || info.Label != "Created at" || !info.Searchable || !info.Sortable {
		t.Errorf("unexpected info: %+v", info)
	}
	plain := Text("Name").Info()
	if plain.Searchable || plain.Sortable {
		t.Errorf("flags must default to off: %+v", plain)
	}
	if got := Text("Name").Label("Title").Info().Label; got != "Title" {
		t.Errorf("label override: %q", got)
	}
}

func TestTextCellEscapesAndHandlesValues(t *testing.T) {
	col := Text("Name")
	if got := cell(t, col, "<b>x</b>"); strings.Contains(got, "<b>") {
		t.Errorf("cell was not escaped: %s", got)
	}
	if got := cell(t, col, 42); got != "42" {
		t.Errorf("int cell: %q", got)
	}
	s := "hello"
	if got := cell(t, col, &s); got != "hello" {
		t.Errorf("pointer cell: %q", got)
	}
	var nilPtr *string
	if got := cell(t, col, nilPtr); got != "" {
		t.Errorf("nil pointer cell: %q", got)
	}
	if got := cell(t, col, nil); got != "" {
		t.Errorf("nil cell: %q", got)
	}
}

func TestBooleanCell(t *testing.T) {
	col := Boolean("Active")
	if got := cell(t, col, true); !strings.Contains(got, "Yes") || !strings.Contains(got, "badge-on") {
		t.Errorf("true cell: %s", got)
	}
	if got := cell(t, col, false); !strings.Contains(got, "No") || !strings.Contains(got, "badge-off") {
		t.Errorf("false cell: %s", got)
	}
	if !Boolean("Active").Sortable().Info().Sortable {
		t.Error("Sortable flag lost")
	}
}
