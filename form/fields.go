package form

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
)

// timeType is the reflect.Type DateField binds to, since a plain
// reflect.Struct Kind check would also accept unrelated struct fields.
var timeType = reflect.TypeOf(time.Time{})

// TextField is a single-line or multi-line text input bound to a string field.
type TextField struct {
	name      string
	label     string
	required  bool
	maxLength int
	multiline bool
}

// Text returns a single-line text input for the named model field.
func Text(name string) *TextField {
	return &TextField{name: name, label: humanize.Name(name)}
}

// Textarea returns a multi-line text input for the named model field.
func Textarea(name string) *TextField {
	f := Text(name)
	f.multiline = true
	return f
}

// Label overrides the default label.
func (f *TextField) Label(label string) *TextField { f.label = label; return f }

// Required rejects empty or whitespace-only input.
func (f *TextField) Required() *TextField { f.required = true; return f }

// MaxLength rejects input longer than n characters.
func (f *TextField) MaxLength(n int) *TextField { f.maxLength = n; return f }

func (f *TextField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *TextField) Check(target reflect.Type) error {
	if target.Kind() != reflect.String {
		return fmt.Errorf("form.Text(%q) needs a string field, the model field is %s", f.name, target)
	}
	return nil
}

func (f *TextField) Format(v any) string { return fmt.Sprint(v) }

func (f *TextField) Parse(raw string, target reflect.Type) (any, string) {
	if !secure.ValidText(raw) {
		return nil, i18n.T("validation.invalid_text")
	}
	if f.required && strings.TrimSpace(raw) == "" {
		return nil, i18n.T("validation.required")
	}
	if f.maxLength > 0 && utf8.RuneCountInString(raw) > f.maxLength {
		return nil, fmt.Sprintf(i18n.T("validation.max_length"), f.maxLength)
	}
	out := reflect.New(target).Elem()
	out.SetString(raw)
	return out.Interface(), ""
}

func (f *TextField) Render(v Value) templ.Component {
	if f.multiline {
		return textareaInput(f, v)
	}
	return textInput(f, v)
}

// NumberField is a numeric input bound to an integer or float field.
type NumberField struct {
	name     string
	label    string
	required bool
}

// Number returns a numeric input for the named model field.
func Number(name string) *NumberField {
	return &NumberField{name: name, label: humanize.Name(name)}
}

// Label overrides the default label.
func (f *NumberField) Label(label string) *NumberField { f.label = label; return f }

// Required rejects empty input. Without it, empty input stores zero.
func (f *NumberField) Required() *NumberField { f.required = true; return f }

func (f *NumberField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *NumberField) Check(target reflect.Type) error {
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	}
	return fmt.Errorf("form.Number(%q) needs an integer or float field, the model field is %s", f.name, target)
}

func (f *NumberField) Format(v any) string {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, rv.Type().Bits())
	}
	return fmt.Sprint(v)
}

func (f *NumberField) Parse(raw string, target reflect.Type) (any, string) {
	raw = strings.TrimSpace(raw)
	out := reflect.New(target).Elem()
	if raw == "" {
		if f.required {
			return nil, i18n.T("validation.required")
		}
		return out.Interface(), ""
	}
	invalid := i18n.T("validation.number")
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, target.Bits())
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, invalid
		}
		out.SetFloat(n)
	}
	return out.Interface(), ""
}

func (f *NumberField) Render(v Value) templ.Component { return numberInput(f, v) }

// ToggleField is a checkbox bound to a bool field.
type ToggleField struct {
	name  string
	label string
}

// Toggle returns a checkbox for the named model field.
func Toggle(name string) *ToggleField {
	return &ToggleField{name: name, label: humanize.Name(name)}
}

// Label overrides the default label.
func (f *ToggleField) Label(label string) *ToggleField { f.label = label; return f }

func (f *ToggleField) Info() Info { return Info{Name: f.name, Label: f.label} }

func (f *ToggleField) Check(target reflect.Type) error {
	if target.Kind() != reflect.Bool {
		return fmt.Errorf("form.Toggle(%q) needs a bool field, the model field is %s", f.name, target)
	}
	return nil
}

// Format returns "true" for a checked toggle and "" otherwise.
func (f *ToggleField) Format(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Bool && rv.Bool() {
		return "true"
	}
	return ""
}

// Parse treats an absent or empty checkbox as false, because browsers do not
// submit unchecked checkboxes.
func (f *ToggleField) Parse(raw string, target reflect.Type) (any, string) {
	out := reflect.New(target).Elem()
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "false", "0", "off":
	default:
		out.SetBool(true)
	}
	return out.Interface(), ""
}

func (f *ToggleField) Render(v Value) templ.Component { return toggleInput(f, v) }

// DateField is a date or date-and-time input bound to a time.Time field.
type DateField struct {
	name     string
	label    string
	required bool
	withTime bool
}

// Date returns a date-only input for the named time.Time model field.
func Date(name string) *DateField {
	return &DateField{name: name, label: humanize.Name(name)}
}

// DateTime returns a date-and-time input for the named time.Time model field.
func DateTime(name string) *DateField {
	f := Date(name)
	f.withTime = true
	return f
}

// Label overrides the default label.
func (f *DateField) Label(label string) *DateField { f.label = label; return f }

// Required rejects empty input.
func (f *DateField) Required() *DateField { f.required = true; return f }

func (f *DateField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *DateField) Check(target reflect.Type) error {
	if target != timeType {
		return fmt.Errorf("form.Date(%q) needs a time.Time field, the model field is %s", f.name, target)
	}
	return nil
}

// layout is the HTML date/datetime-local value format, which doubles as the
// time.Parse/Format layout since both use the same reference date.
func (f *DateField) layout() string {
	if f.withTime {
		return "2006-01-02T15:04"
	}
	return "2006-01-02"
}

func (f *DateField) Format(v any) string {
	t, ok := v.(time.Time)
	if !ok || t.IsZero() {
		return ""
	}
	return t.Format(f.layout())
}

func (f *DateField) Parse(raw string, target reflect.Type) (any, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if f.required {
			return nil, i18n.T("validation.required")
		}
		return time.Time{}, ""
	}
	t, err := time.Parse(f.layout(), raw)
	if err != nil {
		return nil, i18n.T("validation.date")
	}
	return t, ""
}

func (f *DateField) Render(v Value) templ.Component { return dateInput(f, v) }

// MoneyField is a currency amount bound to an integer field storing minor
// units (e.g. cents), never a float, to avoid floating-point rounding bugs.
type MoneyField struct {
	name     string
	label    string
	required bool
	currency string
}

// Money returns a currency input for the named integer model field.
func Money(name string) *MoneyField {
	return &MoneyField{name: name, label: humanize.Name(name), currency: "$"}
}

// Label overrides the default label.
func (f *MoneyField) Label(label string) *MoneyField { f.label = label; return f }

// Required rejects empty input. Without it, empty input stores zero.
func (f *MoneyField) Required() *MoneyField { f.required = true; return f }

// Currency sets the symbol shown and accepted alongside the amount. Default "$".
func (f *MoneyField) Currency(symbol string) *MoneyField { f.currency = symbol; return f }

func (f *MoneyField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *MoneyField) Check(target reflect.Type) error {
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return nil
	}
	return fmt.Errorf("form.Money(%q) needs an integer field storing minor units (e.g. cents), the model field is %s", f.name, target)
}

func (f *MoneyField) Format(v any) string {
	rv := reflect.ValueOf(v)
	var cents int64
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		cents = rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		cents = int64(rv.Uint())
	default:
		return fmt.Sprint(v)
	}
	return formatCents(cents, f.currency)
}

// moneyChars keeps only what strconv.ParseFloat needs, so a formatted
// "$1,234.56" and a bare "1234.56" both parse the same way.
func moneyChars(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatCents(cents int64, symbol string) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%s%d.%02d", sign, symbol, cents/100, cents%100)
}

func (f *MoneyField) Parse(raw string, target reflect.Type) (any, string) {
	out := reflect.New(target).Elem()
	if strings.TrimSpace(raw) == "" {
		if f.required {
			return nil, i18n.T("validation.required")
		}
		return out.Interface(), ""
	}
	invalid := i18n.T("validation.number")
	amount, err := strconv.ParseFloat(moneyChars(raw), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, invalid
	}
	cents := int64(math.Round(amount * 100))
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strconv.FormatInt(cents, 10), 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(strconv.FormatInt(cents, 10), 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetUint(n)
	}
	return out.Interface(), ""
}

func (f *MoneyField) Render(v Value) templ.Component { return moneyInput(f, v) }
