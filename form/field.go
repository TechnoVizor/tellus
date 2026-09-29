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

// Option is one entry of a select whose choices come from outside the
// field itself, for example a belongsTo relation's rows.
type Option struct{ Value, Label string }

// OptionsField is implemented by a field whose Render needs option data
// only the panel can supply. The panel calls RenderOptions instead of
// Render for such a field.
type OptionsField interface {
	Field
	RenderOptions(v Value, opts []Option) templ.Component
}

// RelationInfo is what a field's Relation-style declaration recorded,
// read by the panel at registration time to resolve the related model
// and build the field's option loader.
type RelationInfo struct {
	Field string // Go struct field on the model holding the association, for example "Category"
	Label string // field name on the related model shown as each option's label, for example "Name"
}

// RelationField is implemented by a field that needs the panel to
// resolve a relation at registration time. SelectField is the only
// implementation in this phase.
type RelationField interface {
	Field
	// RelationInfo reports the field's Relation(...) call. ok is false
	// when it was never called.
	RelationInfo() (RelationInfo, bool)
}
