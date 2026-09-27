package form

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/internal/i18n"
)

// defaultImageMaxBytes is the upload size limit an ImageField uses unless
// MaxSize sets one, chosen to comfortably fit a phone photo.
const defaultImageMaxBytes = 4 << 20

// allowedImageTypes are the content types ImageField accepts, detected by
// sniffing the file's bytes, never trusted from the filename or the browser's
// declared Content-Type.
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// ImageField is a file upload bound to a string field that stores the
// uploaded image's URL. Unlike other fields, its value comes from
// ParseFile, not Parse: the panel calls ParseFile only when the request
// carries a new file, so editing a record without choosing a new image
// leaves the stored URL untouched.
type ImageField struct {
	name     string
	label    string
	required bool
	maxSize  int64
}

// Image returns an image upload field for the named model field, which must
// be a string that holds the stored image's URL.
func Image(name string) *ImageField {
	return &ImageField{name: name, label: humanize.Name(name), maxSize: defaultImageMaxBytes}
}

// Label overrides the default label.
func (f *ImageField) Label(label string) *ImageField { f.label = label; return f }

// Required rejects a record that has never had an image. It has no effect on
// whether a given edit must include a new file.
func (f *ImageField) Required() *ImageField { f.required = true; return f }

// MaxSize sets the largest accepted upload, in bytes. Default 4 MiB.
func (f *ImageField) MaxSize(bytes int64) *ImageField { f.maxSize = bytes; return f }

func (f *ImageField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *ImageField) Check(target reflect.Type) error {
	if target.Kind() != reflect.String {
		return fmt.Errorf("form.Image(%q) needs a string field to hold the image URL, the model field is %s", f.name, target)
	}
	return nil
}

func (f *ImageField) Format(v any) string { return fmt.Sprint(v) }

// Parse exists to satisfy Field; the panel never calls it for an ImageField,
// since apply() routes file fields through ParseFile instead. It passes raw
// through unchanged so a direct call is harmless rather than surprising.
func (f *ImageField) Parse(raw string, target reflect.Type) (any, string) {
	if f.required && raw == "" {
		return nil, i18n.T("validation.required")
	}
	out := reflect.New(target).Elem()
	out.SetString(raw)
	return out.Interface(), ""
}

// ParseFile validates fh by sniffing its content, not trusting its name or
// declared Content-Type, then saves it and returns the stored URL.
func (f *ImageField) ParseFile(ctx context.Context, fh *multipart.FileHeader, target reflect.Type, storage Storage) (any, string) {
	if f.maxSize > 0 && fh.Size > f.maxSize {
		return nil, fmt.Sprintf(i18n.T("validation.image_too_large"), f.maxSize/(1<<20))
	}
	file, err := fh.Open()
	if err != nil {
		return nil, i18n.T("error.internal")
	}
	defer file.Close()

	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, i18n.T("error.internal")
	}
	sniff = sniff[:n]
	contentType := http.DetectContentType(sniff)
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return nil, i18n.T("validation.image_type")
	}

	full := io.MultiReader(bytes.NewReader(sniff), file)
	path, err := storage.Save(ctx, "upload"+ext, full)
	if err != nil {
		return nil, i18n.T("error.internal")
	}

	url := storage.URL(path)
	out := reflect.New(target).Elem()
	out.SetString(url)
	return out.Interface(), ""
}

func (f *ImageField) Render(v Value) templ.Component { return imageInput(f, v) }
