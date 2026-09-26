// Package ui holds the templ pages of the panel and the plain view structs
// they render. It knows nothing about models or databases.
package ui

import (
	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/assets"
)

// Shell is the data every page with the panel layout needs.
type Shell struct {
	Title    string
	Brand    string
	Prefix   string // panel URL prefix without a trailing slash
	Nav      []NavItem
	UserName string
	CSRF     string
}

// NavItem is one sidebar link.
type NavItem struct {
	Label  string
	Href   string
	Active bool
}

func (s Shell) asset(name string) string {
	return s.Prefix + "/assets/" + name + "?v=" + assets.Version()
}

// LoginView is the data of the sign-in page.
type LoginView struct {
	Brand  string
	Prefix string
	Action string
	Email  string
	Error  string
	CSRF   string
}

func (v LoginView) asset(name string) string {
	return v.Prefix + "/assets/" + name + "?v=" + assets.Version()
}

// ColumnView is one list heading.
type ColumnView struct {
	Label    string
	Sortable bool
	SortURL  string // link that sorts by this column
	SortMark string // "▲", "▼" or "" for the active sort
	AriaSort string // "ascending", "descending" or "" when not the active sort
}

// RowView is one list row.
type RowView struct {
	Cells     []templ.Component
	EditURL   string
	DeleteURL string
}

// ListView is the data of a resource list page and of its #records region.
type ListView struct {
	Shell      Shell
	Heading    string
	Notice     string
	NewURL     string
	Search     string
	Searchable bool
	ListURL    string // base URL of the list, used by the search form
	Sort       string // current sort field name, "" for none
	Dir        string // "asc" or "desc"
	Columns    []ColumnView
	Rows       []RowView
	Total      int64
	Page       int
	TotalPages int
	PrevURL    string // empty when there is no previous page
	NextURL    string // empty when there is no next page
}

// FormView is the data of the create and edit page.
type FormView struct {
	Shell     Shell
	Heading   string
	Action    string
	CancelURL string
	Error     string // summary shown above the fields, empty when none
	Fields    []templ.Component
}
