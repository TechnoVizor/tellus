package tellus

import (
	"context"
	"errors"
)

// ErrNotFound is returned by a DataSource when a record does not exist.
var ErrNotFound = errors.New("tellus: record not found")

// ListQuery describes one page of a resource list. Field names are Go struct
// field names of the model.
type ListQuery struct {
	Search       string   // text to look for; empty means no search
	SearchFields []string // fields the search looks at
	SortField    string   // empty means primary key order
	SortDesc     bool
	Page         int // 1-based
	PerPage      int
}

// ListResult is one page of records plus the total number of matches.
type ListResult[T any] struct {
	Items []T
	Total int64
}

// DataSource is where a resource reads and writes its records. GormSource is
// the built-in implementation. Ids travel as strings because they come from
// URLs.
type DataSource[T any] interface {
	List(ctx context.Context, q ListQuery) (ListResult[T], error)
	Find(ctx context.Context, id string) (*T, error) // ErrNotFound when missing
	Create(ctx context.Context, item *T) error
	Update(ctx context.Context, item *T) error // ErrNotFound when the record is gone
	Delete(ctx context.Context, id string) error
	ID(item *T) string
}
