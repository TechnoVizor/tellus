package table

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
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

func TestMoneyColumnCell(t *testing.T) {
	if got := cell(t, Money("Price"), 2500); got != "$25.00" {
		t.Errorf("positive cell: %q", got)
	}
	if got := cell(t, Money("Price"), -500); got != "-$5.00" {
		t.Errorf("negative cell: %q", got)
	}
	if got := cell(t, Money("Price").Currency("€"), 2500); got != "€25.00" {
		t.Errorf("custom currency cell: %q", got)
	}
	if got := cell(t, Money("Price"), "not money"); got != "" {
		t.Errorf("non-numeric cell must be empty, got %q", got)
	}
}

func TestMoneyColumnInfoAndDefaults(t *testing.T) {
	info := Money("Price").Sortable().Info()
	if info.Name != "Price" || info.Label != "Price" || !info.Sortable {
		t.Errorf("unexpected info: %+v", info)
	}
}

func TestDateColumnCell(t *testing.T) {
	in := time.Date(2026, 9, 29, 10, 15, 0, 0, time.UTC)
	if got := cell(t, Date("Published"), in); got != "2026-09-29" {
		t.Errorf("date cell: %q", got)
	}
	if got := cell(t, DateTime("PublishedAt"), in); got != "2026-09-29 10:15" {
		t.Errorf("datetime cell: %q", got)
	}
	if got := cell(t, Date("Published"), time.Time{}); got != "" {
		t.Errorf("zero time cell must be empty, got %q", got)
	}
	if got := cell(t, Date("Published"), "not a time"); got != "" {
		t.Errorf("wrong-typed value must render empty, got %q", got)
	}
}

func TestDateColumnInfoAndDefaults(t *testing.T) {
	info := Date("Published").Sortable().Info()
	if info.Name != "Published" || info.Label != "Published" || !info.Sortable {
		t.Errorf("unexpected info: %+v", info)
	}
}
