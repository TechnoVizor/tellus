package form

import (
	"context"
	"io"
	"mime/multipart"
	"reflect"
)

// Storage is where an uploaded file's bytes are kept and how a browser reaches
// it afterwards. tellus.LocalStorage is the built-in implementation.
type Storage interface {
	// Save writes r under a name derived from filename and returns a path
	// unique to this upload.
	Save(ctx context.Context, filename string, r io.Reader) (path string, err error)
	// URL returns the address a browser fetches path from.
	URL(path string) string
}

// FileField is implemented by fields that read an uploaded file instead of a
// plain text value, such as Image. The panel calls ParseFile only when the
// request actually included a new file for this field; editing a record
// without replacing its file leaves the field's stored value untouched.
type FileField interface {
	Field
	ParseFile(ctx context.Context, fh *multipart.FileHeader, target reflect.Type, storage Storage) (value any, message string)
}
