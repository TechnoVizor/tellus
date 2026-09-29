package form

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

var (
	tString  = reflect.TypeOf("")
	tInt     = reflect.TypeOf(0)
	tInt8    = reflect.TypeOf(int8(0))
	tUint    = reflect.TypeOf(uint(0))
	tFloat64 = reflect.TypeOf(0.0)
	tBool    = reflect.TypeOf(false)
	tTime    = reflect.TypeOf(time.Time{})
)

func render(t *testing.T, f Field, v Value) string {
	t.Helper()
	var buf bytes.Buffer
	if err := f.Render(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func renderOptions(t *testing.T, f *SelectField, v Value, opts []Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := f.RenderOptions(v, opts).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTextParse(t *testing.T) {
	f := Text("Name").Required().MaxLength(5)

	if _, msg := f.Parse("  ", tString); msg == "" {
		t.Error("whitespace-only input must fail Required")
	}
	if _, msg := f.Parse("toolong", tString); msg == "" {
		t.Error("input over MaxLength must fail")
	}
	// MaxLength counts characters, not bytes.
	if v, msg := f.Parse("héllo", tString); msg != "" || v != "héllo" {
		t.Errorf("5 characters must fit MaxLength(5): v=%v msg=%q", v, msg)
	}
	loose := Text("Name")
	if v, msg := loose.Parse("", tString); msg != "" || v != "" {
		t.Errorf("optional empty text: v=%v msg=%q", v, msg)
	}
}

func TestTextKeepsNamedStringType(t *testing.T) {
	type Slug string
	v, msg := Text("Slug").Parse("abc", reflect.TypeOf(Slug("")))
	if msg != "" {
		t.Fatal(msg)
	}
	if _, ok := v.(Slug); !ok {
		t.Fatalf("Parse must return the target type, got %T", v)
	}
}

func TestNumberParse(t *testing.T) {
	cases := []struct {
		name   string
		f      *NumberField
		raw    string
		target reflect.Type
		want   any
		bad    bool
	}{
		{"int", Number("P"), "42", tInt, 42, false},
		{"negative int", Number("P"), "-7", tInt, -7, false},
		{"int8 overflow", Number("P"), "300", tInt8, nil, true},
		{"uint negative", Number("P"), "-1", tUint, nil, true},
		{"float", Number("P"), "1.5", tFloat64, 1.5, false},
		{"not a number", Number("P"), "abc", tInt, nil, true},
		{"int given a decimal", Number("P"), "1.5", tInt, nil, true},
		{"NaN rejected", Number("P"), "NaN", tFloat64, nil, true},
		{"Inf rejected", Number("P"), "Inf", tFloat64, nil, true},
		{"empty optional is zero", Number("P"), "", tInt, 0, false},
		{"empty required fails", Number("P").Required(), "", tInt, nil, true},
		{"spaces trimmed", Number("P"), " 8 ", tInt, 8, false},
	}
	for _, tc := range cases {
		got, msg := tc.f.Parse(tc.raw, tc.target)
		if tc.bad {
			if msg == "" {
				t.Errorf("%s: expected a validation message", tc.name)
			}
			continue
		}
		if msg != "" || got != tc.want {
			t.Errorf("%s: got %v (%q), want %v", tc.name, got, msg, tc.want)
		}
	}
}

func TestNumberFormat(t *testing.T) {
	f := Number("P")
	for _, tc := range []struct {
		in   any
		want string
	}{{42, "42"}, {uint(7), "7"}, {1.5, "1.5"}, {1e21, "1000000000000000000000"}} {
		if got := f.Format(tc.in); got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestToggleParseAndFormat(t *testing.T) {
	f := Toggle("Active")
	for raw, want := range map[string]bool{"": false, "false": false, "0": false, "off": false, "true": true, "on": true} {
		got, msg := f.Parse(raw, tBool)
		if msg != "" || got != want {
			t.Errorf("Parse(%q) = %v (%q), want %v", raw, got, msg, want)
		}
	}
	if f.Format(true) != "true" || f.Format(false) != "" {
		t.Error("Format must return \"true\" for checked and \"\" otherwise")
	}
}

func TestCheckRejectsWrongModelType(t *testing.T) {
	if err := Text("X").Check(tInt); err == nil {
		t.Error("Text on an int field must be rejected")
	}
	if err := Number("X").Check(tString); err == nil {
		t.Error("Number on a string field must be rejected")
	}
	if err := Toggle("X").Check(tInt); err == nil {
		t.Error("Toggle on an int field must be rejected")
	}
	if err := Date("X").Check(tString); err == nil {
		t.Error("Date on a string field must be rejected")
	}
	if err := Text("X").Check(tString); err != nil {
		t.Error(err)
	}
	if err := Date("X").Check(tTime); err != nil {
		t.Error(err)
	}
}

func TestMoneyParse(t *testing.T) {
	cases := []struct {
		name   string
		f      *MoneyField
		raw    string
		target reflect.Type
		want   any
		bad    bool
	}{
		{"plain integer", Money("P"), "25", tInt, int64(2500), false},
		{"two decimals", Money("P"), "25.00", tInt, int64(2500), false},
		{"formatted with symbol and comma", Money("P"), "$1,234.56", tInt, int64(123456), false},
		{"negative", Money("P"), "-5.00", tInt, int64(-500), false},
		{"rounds to nearest cent", Money("P"), "10.999", tInt, int64(1100), false},
		{"not a number", Money("P"), "abc", tInt, nil, true},
		{"int8 overflow", Money("P"), "1000", tInt8, nil, true},
		{"uint negative", Money("P"), "-5.00", tUint, nil, true},
		{"empty optional is zero", Money("P"), "", tInt, int64(0), false},
		{"empty required fails", Money("P").Required(), "", tInt, nil, true},
		{"symbol with no digits is invalid", Money("P"), "$", tInt, nil, true},
	}
	for _, tc := range cases {
		got, msg := tc.f.Parse(tc.raw, tc.target)
		if tc.bad {
			if msg == "" {
				t.Errorf("%s: expected a validation message", tc.name)
			}
			continue
		}
		if msg != "" {
			t.Errorf("%s: unexpected message %q", tc.name, msg)
			continue
		}
		rv := reflect.ValueOf(got)
		var n int64
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n = rv.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n = int64(rv.Uint())
		}
		if n != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, n, tc.want)
		}
	}
}

func TestMoneyFormat(t *testing.T) {
	f := Money("P")
	if got := f.Format(2500); got != "$25.00" {
		t.Errorf("Format(2500) = %q", got)
	}
	if got := f.Format(-500); got != "-$5.00" {
		t.Errorf("Format(-500) = %q", got)
	}
	if got := f.Format(5); got != "$0.05" {
		t.Errorf("Format(5) = %q", got)
	}
	if got := Money("P").Currency("€").Format(2500); got != "€25.00" {
		t.Errorf("custom currency: %q", got)
	}
}

func TestMoneyCheckRejectsNonIntegerType(t *testing.T) {
	if err := Money("X").Check(tFloat64); err == nil {
		t.Error("Money on a float field must be rejected")
	}
	if err := Money("X").Check(tString); err == nil {
		t.Error("Money on a string field must be rejected")
	}
	if err := Money("X").Check(tInt); err != nil {
		t.Error(err)
	}
	if err := Money("X").Check(tUint); err != nil {
		t.Error(err)
	}
}

func TestRenderMoney(t *testing.T) {
	out := render(t, Money("Price"), Value{Raw: "$25.00"})
	if !strings.Contains(out, `type="text"`) || !strings.Contains(out, `inputmode="decimal"`) || !strings.Contains(out, `value="$25.00"`) {
		t.Errorf("money render: %s", out)
	}
}

func TestDateParse(t *testing.T) {
	f := Date("Published")
	got, msg := f.Parse("2026-09-29", tTime)
	if msg != "" {
		t.Fatal(msg)
	}
	want := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	if !got.(time.Time).Equal(want) {
		t.Errorf("Parse date = %v, want %v", got, want)
	}
	if _, msg := f.Parse("not-a-date", tTime); msg == "" {
		t.Error("malformed date must fail")
	}
	if _, msg := f.Parse("2026-09-29T10:00", tTime); msg == "" {
		t.Error("a datetime-local value must be rejected by the date-only layout")
	}
	if v, msg := f.Parse("", tTime); msg != "" || !v.(time.Time).IsZero() {
		t.Errorf("optional empty date: v=%v msg=%q", v, msg)
	}
	if _, msg := f.Required().Parse("", tTime); msg == "" {
		t.Error("empty required date must fail")
	}
}

func TestDateTimeParse(t *testing.T) {
	f := DateTime("PublishedAt")
	got, msg := f.Parse("2026-09-29T10:15", tTime)
	if msg != "" {
		t.Fatal(msg)
	}
	want := time.Date(2026, 9, 29, 10, 15, 0, 0, time.UTC)
	if !got.(time.Time).Equal(want) {
		t.Errorf("Parse date-time = %v, want %v", got, want)
	}
	if _, msg := f.Parse("2026-09-29", tTime); msg == "" {
		t.Error("a date-only value must be rejected by the date-time layout")
	}
}

func TestDateFormat(t *testing.T) {
	in := time.Date(2026, 9, 29, 10, 15, 0, 0, time.UTC)
	if got := Date("P").Format(in); got != "2026-09-29" {
		t.Errorf("Date Format = %q", got)
	}
	if got := DateTime("P").Format(in); got != "2026-09-29T10:15" {
		t.Errorf("DateTime Format = %q", got)
	}
	if got := Date("P").Format(time.Time{}); got != "" {
		t.Errorf("zero time must format empty, got %q", got)
	}
}

func TestSelectCheckRequiresRelationAndRightKind(t *testing.T) {
	if err := Select("X").Check(tUint); err == nil {
		t.Error("Select without Relation must be rejected")
	}
	if err := Select("X").Relation("Category", "Name").Check(tBool); err == nil {
		t.Error("Select on a bool field must be rejected")
	}
	if err := Select("X").Relation("Category", "Name").Check(tUint); err != nil {
		t.Error(err)
	}
	if err := Select("X").Relation("Category", "Name").Check(tString); err != nil {
		t.Error(err)
	}
}

func TestSelectRelationInfo(t *testing.T) {
	if _, ok := Select("CategoryID").RelationInfo(); ok {
		t.Error("RelationInfo must report false before Relation is called")
	}
	f := Select("CategoryID").Relation("Category", "Name")
	rel, ok := f.RelationInfo()
	if !ok || rel.Field != "Category" || rel.Label != "Name" {
		t.Errorf("RelationInfo() = %+v, %v", rel, ok)
	}
}

func TestSelectFormat(t *testing.T) {
	f := Select("CategoryID").Relation("Category", "Name")
	if got := f.Format(uint(3)); got != "3" {
		t.Errorf("Format(uint(3)) = %q", got)
	}
	if got := f.Format("abc"); got != "abc" {
		t.Errorf("Format(%q) = %q", "abc", got)
	}
}

func TestSelectParse(t *testing.T) {
	f := Select("CategoryID").Relation("Category", "Name")
	if got, msg := f.Parse("3", tUint); msg != "" || got != uint(3) {
		t.Errorf("Parse(\"3\") = %v, %q", got, msg)
	}
	if got, msg := f.Parse("", tUint); msg != "" || got != uint(0) {
		t.Errorf("optional empty select: %v, %q", got, msg)
	}
	if _, msg := f.Required().Parse("", tUint); msg == "" {
		t.Error("empty required select must fail")
	}
	if _, msg := f.Parse("abc", tUint); msg == "" {
		t.Error("non-numeric selection on a uint field must fail")
	}
	if got, msg := Select("Slug").Relation("Category", "Name").Parse("cat-1", tString); msg != "" || got != "cat-1" {
		t.Errorf("string-keyed select: %v, %q", got, msg)
	}
}

func TestDefaultLabelAndOverride(t *testing.T) {
	if got := Text("CreatedAt").Info().Label; got != "Created at" {
		t.Errorf("default label: %q", got)
	}
	if got := Text("CreatedAt").Label("Added").Info().Label; got != "Added" {
		t.Errorf("override label: %q", got)
	}
}

func TestRenderTextEscapesValueAndShowsError(t *testing.T) {
	out := render(t, Text("Name").Required().MaxLength(10), Value{
		Raw:   `"><script>alert(1)</script>`,
		Error: "Bad value",
	})
	if strings.Contains(out, "<script>") {
		t.Fatalf("value was not escaped: %s", out)
	}
	for _, want := range []string{`name="Name"`, `maxlength="10"`, `required`, `aria-invalid="true"`, "Bad value"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestRenderToggleChecked(t *testing.T) {
	checked := render(t, Toggle("Active"), Value{Raw: "true"})
	if !strings.Contains(checked, "checked") {
		t.Errorf("expected checked: %s", checked)
	}
	unchecked := render(t, Toggle("Active"), Value{})
	if strings.Contains(unchecked, "checked") {
		t.Errorf("did not expect checked: %s", unchecked)
	}
}

func TestRenderTextareaAndNumber(t *testing.T) {
	area := render(t, Textarea("Description"), Value{Raw: "line1\nline2"})
	if !strings.Contains(area, "<textarea") || !strings.Contains(area, "line1\nline2") {
		t.Errorf("textarea render: %s", area)
	}
	num := render(t, Number("Price"), Value{Raw: "12"})
	if !strings.Contains(num, `type="number"`) || !strings.Contains(num, `value="12"`) {
		t.Errorf("number render: %s", num)
	}
}

func TestRenderDateAndDateTime(t *testing.T) {
	date := render(t, Date("Published"), Value{Raw: "2026-09-29"})
	if !strings.Contains(date, `data-value="2026-09-29"`) || !strings.Contains(date, `name="Published"`) {
		t.Errorf("date render: %s", date)
	}
	if strings.Contains(date, `type="time"`) {
		t.Errorf("a date-only field must not render a time input: %s", date)
	}
	dt := render(t, DateTime("PublishedAt"), Value{Raw: "2026-09-29T10:15"})
	if !strings.Contains(dt, `data-value="2026-09-29T10:15"`) || !strings.Contains(dt, `type="time"`) {
		t.Errorf("datetime render: %s", dt)
	}
}

func TestRenderDateEscapesValue(t *testing.T) {
	out := render(t, Date("Published"), Value{Raw: `"><script>alert(1)</script>`, Error: "Bad value"})
	if strings.Contains(out, "<script>") {
		t.Fatalf("value was not escaped: %s", out)
	}
	if !strings.Contains(out, "Bad value") || !strings.Contains(out, `aria-invalid="true"`) {
		t.Errorf("missing error wiring: %s", out)
	}
}

func TestRenderSelectOptions(t *testing.T) {
	f := Select("CategoryID").Relation("Category", "Name").Required()
	opts := []Option{{Value: "1", Label: "Books"}, {Value: "2", Label: "Music"}}
	out := renderOptions(t, f, Value{Raw: "2"}, opts)
	if !strings.Contains(out, `value="1"`) || !strings.Contains(out, "Books") {
		t.Errorf("missing option 1: %s", out)
	}
	if !strings.Contains(out, `value="2" selected`) || !strings.Contains(out, "Music") {
		t.Errorf("option 2 must be selected: %s", out)
	}
	if strings.Contains(out, `value=""`) {
		t.Errorf("a required select must not render an empty option: %s", out)
	}
}

func TestRenderSelectEmptyOptionWhenNotRequired(t *testing.T) {
	f := Select("CategoryID").Relation("Category", "Name")
	out := renderOptions(t, f, Value{}, nil)
	if !strings.Contains(out, `<option value=""></option>`) {
		t.Errorf("an optional select must render a leading empty option: %s", out)
	}
}

func TestRenderSelectEscapesLabel(t *testing.T) {
	f := Select("CategoryID").Relation("Category", "Name")
	out := renderOptions(t, f, Value{}, []Option{{Value: "1", Label: "\"><script>alert(1)</script>"}})
	if strings.Contains(out, "<script>") {
		t.Fatalf("label was not escaped: %s", out)
	}
}
