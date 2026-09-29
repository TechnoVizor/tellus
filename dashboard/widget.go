// Package dashboard holds the widget builders used to describe the panel's
// dashboard home page. Custom widgets implement Widget.
package dashboard

import (
	"context"

	"github.com/a-h/templ"
)

// Widget is one piece of the dashboard's home page. A host builds a
// dashboard from a list of these; Stat is the only implementation in this
// phase, more follow in later work.
type Widget interface {
	// Render draws the widget for one request. ctx carries the request's
	// context, so a widget's own data-fetching functions can use it the
	// same way a DataSource already does.
	Render(ctx context.Context) (templ.Component, error)
}

// Validated is implemented by a widget with configuration Dashboard checks
// before the panel starts, the same "reported at startup" guarantee the
// rest of this library already gives (a typo in a Table/Form field name is
// caught the same way). Stat implements it.
type Validated interface {
	Widget
	Validate() error
}
