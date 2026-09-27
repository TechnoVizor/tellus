package table

import (
	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
)

// ImageColumn shows a small thumbnail, or a placeholder when the record has
// none.
type ImageColumn struct {
	name  string
	label string
}

// Image returns a thumbnail column for the named model field, which holds an
// image URL such as the one form.Image stores.
func Image(name string) *ImageColumn {
	return &ImageColumn{name: name, label: humanize.Name(name)}
}

// Label overrides the default heading.
func (c *ImageColumn) Label(label string) *ImageColumn { c.label = label; return c }

func (c *ImageColumn) Info() Info { return Info{Name: c.name, Label: c.label} }

func (c *ImageColumn) Cell(v any) templ.Component { return imageCell(display(v)) }
