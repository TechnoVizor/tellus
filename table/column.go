// Package table holds the column builders used to describe a resource's list.
package table

import (
	"fmt"
	"reflect"
	"time"

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

// DateColumn shows a time.Time value formatted as a date, or a date and time.
type DateColumn struct {
	name     string
	label    string
	sortable bool
	withTime bool
}

// Date returns a date column for the named time.Time field.
func Date(name string) *DateColumn {
	return &DateColumn{name: name, label: humanize.Name(name)}
}

// DateTime returns a date-and-time column for the named time.Time field.
func DateTime(name string) *DateColumn {
	c := Date(name)
	c.withTime = true
	return c
}

// Label overrides the default heading.
func (c *DateColumn) Label(label string) *DateColumn { c.label = label; return c }

// Sortable makes the heading a sort link.
func (c *DateColumn) Sortable() *DateColumn { c.sortable = true; return c }

func (c *DateColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Sortable: c.sortable}
}

func (c *DateColumn) Cell(v any) templ.Component {
	t, ok := v.(time.Time)
	if !ok || t.IsZero() {
		return textCell("")
	}
	layout := "2006-01-02"
	if c.withTime {
		layout = "2006-01-02 15:04"
	}
	return textCell(t.Format(layout))
}

// MoneyColumn shows an integer minor-units value (e.g. cents) as a formatted
// currency amount.
type MoneyColumn struct {
	name     string
	label    string
	sortable bool
	currency string
}

// Money returns a currency column for the named integer field.
func Money(name string) *MoneyColumn {
	return &MoneyColumn{name: name, label: humanize.Name(name), currency: "$"}
}

// Label overrides the default heading.
func (c *MoneyColumn) Label(label string) *MoneyColumn { c.label = label; return c }

// Sortable makes the heading a sort link.
func (c *MoneyColumn) Sortable() *MoneyColumn { c.sortable = true; return c }

// Currency sets the symbol shown alongside the amount. Default "$".
func (c *MoneyColumn) Currency(symbol string) *MoneyColumn { c.currency = symbol; return c }

func (c *MoneyColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Sortable: c.sortable}
}

func (c *MoneyColumn) Cell(v any) templ.Component {
	rv := reflect.ValueOf(v)
	var cents int64
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		cents = rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		cents = int64(rv.Uint())
	default:
		return textCell("")
	}
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return textCell(fmt.Sprintf("%s%s%d.%02d", sign, c.currency, cents/100, cents%100))
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
