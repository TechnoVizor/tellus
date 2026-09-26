// Package table holds the column builders used to describe a resource's list.
package table

import (
	"fmt"
	"reflect"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
)

// Info is the metadata every column exposes.
type Info struct {
	Name       string // Go struct field name on the model
	Label      string // column heading
	Searchable bool   // included in the list's text search
	Sortable   bool   // heading is a sort link
}

// Column is one column of a resource list.
type Column interface {
	Info() Info
	// Cell draws the model field's value.
	Cell(v any) templ.Component
}

// TextColumn shows a value as plain text.
type TextColumn struct {
	name       string
	label      string
	searchable bool
	sortable   bool
}

// Text returns a plain-text column for the named model field.
func Text(name string) *TextColumn {
	return &TextColumn{name: name, label: humanize.Name(name)}
}

// Label overrides the default heading.
func (c *TextColumn) Label(label string) *TextColumn { c.label = label; return c }

// Searchable includes the column in the list's text search.
func (c *TextColumn) Searchable() *TextColumn { c.searchable = true; return c }

// Sortable makes the heading a sort link.
func (c *TextColumn) Sortable() *TextColumn { c.sortable = true; return c }

func (c *TextColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Searchable: c.searchable, Sortable: c.sortable}
}

func (c *TextColumn) Cell(v any) templ.Component { return textCell(display(v)) }

// BooleanColumn shows a bool as a Yes or No badge.
type BooleanColumn struct {
	name     string
	label    string
	sortable bool
}

// Boolean returns a Yes/No badge column for the named bool field.
func Boolean(name string) *BooleanColumn {
	return &BooleanColumn{name: name, label: humanize.Name(name)}
}

// Label overrides the default heading.
func (c *BooleanColumn) Label(label string) *BooleanColumn { c.label = label; return c }

// Sortable makes the heading a sort link.
func (c *BooleanColumn) Sortable() *BooleanColumn { c.sortable = true; return c }

func (c *BooleanColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Sortable: c.sortable}
}

func (c *BooleanColumn) Cell(v any) templ.Component {
	rv := reflect.ValueOf(v)
	return boolCell(rv.Kind() == reflect.Bool && rv.Bool())
}

// display turns a model value into text, following pointers.
func display(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return ""
	}
	return fmt.Sprint(rv.Interface())
}
