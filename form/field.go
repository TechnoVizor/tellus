// Package form holds the field builders used to describe a resource's create
// and edit form. Custom fields implement Field.
package form

import (
	"reflect"

	"github.com/a-h/templ"
)

// Info is the metadata every field exposes.
type Info struct {
	Name     string // Go struct field name on the model
	Label    string // human label shown next to the input
	Required bool
}

// Value is what a field needs to draw itself.
type Value struct {
	Raw   string // current value as text: submitted input on redraw, formatted model value otherwise
	Error string // validation message, empty when the value is valid
}

// Field is one input on a record form.
type Field interface {
	Info() Info
	// Check runs once at startup and rejects a field bound to a model field of
	// the wrong Go type.
	Check(target reflect.Type) error
	// Format renders the model's current value as the text shown in the input.
	Format(v any) string
	// Parse converts submitted text into a value of exactly the target type. A
	// non-empty message means the input is invalid and is shown to the user.
	Parse(raw string, target reflect.Type) (value any, message string)
	// Render draws the label, the input and the error message.
	Render(v Value) templ.Component
}
