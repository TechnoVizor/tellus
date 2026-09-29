# belongsTo Relations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `form.Select` field bound to a belongsTo relation (`form.Select("CategoryID").Relation("Category", "Name")`), populated from the related table and validated against it at registration time.

**Architecture:** `SelectField` (in `form`) is a dropdown bound to an int, uint, or string foreign key field, the same shape as `NumberField`/`TextField` but rendered as `<select>`. `.Relation(relationField, labelField)` is mandatory metadata a Select carries; the panel (root `tellus` package) resolves it once at `register()` time by reflecting on the model and parsing the related type's GORM schema through the same technique `NewGormSource` already uses, then builds a small per-request closure that loads the dropdown's rows. `form` never imports GORM; the closure and its GORM-specific resolution logic live in the root package, in a new `relation.go` file.

**Tech Stack:** Go 1.26+, GORM v1.31.2, templ v0.3.1020, Postgres 17 in Docker for integration tests (unchanged from the M0 plan).

**Spec:** `docs/superpowers/specs/2026-09-29-belongsto-relations-design.md` (the plan argues from it; read both).

## Global Constraints

- English only UI: any new user-facing string goes through `i18n.T("key")`, the key must exist in `internal/i18n/en.go`. This plan reuses existing keys (`validation.required`, `validation.number`, `error.internal`) and adds none.
- No em-dashes anywhere: not in code, comments, docs, copy or commit messages.
- Generated `*_templ.go` files are committed. Regenerate with `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate` after any `.templ` change, before running tests.
- Integration tests use the real Postgres from `docker-compose.yml` (host port 55432) through `TELLUS_TEST_DSN`, and skip themselves when it is unset (`internal/testdb.Open(t)` already does this).
- `form` package imports no GORM and no root `tellus` package (avoids a circular import); GORM-specific relation resolution lives in the root package only.
- Relations are GORM-only in this phase: `.Relation(...)` always resolves through the `*gorm.DB` passed to `Resource[T](db)`, never through a custom `DataSource[T]` supplied via `.Source(...)`.
- Commit messages follow Conventional Commits, no em-dashes.

## Review Focus

1. A related table with more than 500 rows: only the first 500 by label load, the form must not error or silently show every row (Task 3, `TestSelectOptionsAreCappedAndOrdered`).
2. Submitting the empty option on a non-required Select must store the foreign key's zero value and succeed, not fail validation or leave the previous value (Task 3, `TestSelectRendersSavesAndClearsRelation`).
3. Submitting the empty option on a required Select must fail with `validation.required` and must not touch the database (Task 3, `TestSelectRequiredEmptyFails`).
4. A foreign key whose kind family (int, uint, or string) does not match the related model's primary key kind must be rejected at registration, not silently save a value that could never come back out correctly (Task 2, `TestRelationRegistrationRejectsKindMismatch`).
5. A failure loading a relation's options (the related table is gone, a connection error) must render the standard generic 500 page, never the raw database error text (Task 3, `TestSelectOptionLoadFailureIsServerError`).

## Deliberately not in this plan

hasMany (a read-only list on the record page), manyToMany (a multi-select), a relation-free `.Options(...)` for a plain enum select, search or pagination inside the select itself, and any non-GORM `DataSource` support for relations. All deferred in `docs/superpowers/specs/2026-09-29-belongsto-relations-design.md` section 8.

---

### Task 1: form.SelectField

A dropdown field bound to an int, uint, or string model field, carrying a mandatory `.Relation(relationField, labelField)` declaration the panel resolves later. This task is self-contained inside the `form` package: no relation is actually resolved or queried here, only declared and validated for shape.

**Files:**
- Modify: `form/field.go` (add `Option`, `OptionsField`, `RelationInfo`, `RelationField`)
- Modify: `form/fields.go` (add `SelectField` and `Select`)
- Modify: `form/render.templ` (add `selectInput`)
- Modify: `form/render_templ.go` (regenerated, not hand-edited)
- Test: `form/fields_test.go`

**Interfaces:**
- Consumes: nothing new; follows the existing `Field` interface (`form/field.go`) and the `NumberField` pattern (`form/fields.go`) already in the repository.
- Produces:
  - `form.Option{Value, Label string}`
  - `form.OptionsField` interface: `Field` plus `RenderOptions(v Value, opts []Option) templ.Component`
  - `form.RelationInfo{Field, Label string}`
  - `form.RelationField` interface: `Field` plus `RelationInfo() (RelationInfo, bool)`
  - `form.Select(name string) *SelectField`
  - `(*SelectField) Label(string) *SelectField`
  - `(*SelectField) Required() *SelectField`
  - `(*SelectField) Relation(relationField, labelField string) *SelectField`
  - `(*SelectField) RelationInfo() (RelationInfo, bool)`
  - `(*SelectField) Check/Format/Parse/Info/Render/RenderOptions`, satisfying `Field` and `OptionsField` and `RelationField`

- [ ] **Step 1: Write the failing tests for Check, RelationInfo, Format and Parse**

Add to `form/fields_test.go`, right after the existing `TestDateFormat` (keeps the field-by-field grouping the file already uses):

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./form/... -run TestSelect`
Expected: FAIL, `undefined: Select` (and related undefined symbols)

- [ ] **Step 3: Add Option, OptionsField, RelationInfo, RelationField to form/field.go**

Append to the end of `form/field.go`:

```go
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
```

- [ ] **Step 4: Add SelectField to form/fields.go**

Append to the end of `form/fields.go`:

```go
// SelectField is a dropdown bound to an int, uint, or string field. In
// this phase it always needs Relation: a plain, relation-free select
// with static options is not part of this phase.
type SelectField struct {
	name     string
	label    string
	required bool
	relation RelationInfo
	hasRel   bool
}

// Select returns a dropdown for the named model field.
func Select(name string) *SelectField {
	return &SelectField{name: name, label: humanize.Name(name)}
}

// Label overrides the default label.
func (f *SelectField) Label(label string) *SelectField { f.label = label; return f }

// Required rejects an empty selection. Without it, an empty selection
// stores the field's zero value.
func (f *SelectField) Required() *SelectField { f.required = true; return f }

// Relation declares a belongsTo association: relationField is the Go
// struct field on the model holding the association (typically declared
// alongside the foreign key itself, for example "Category" next to a
// "CategoryID" field), labelField is a field name on the related model
// shown as each option's label (for example "Name").
func (f *SelectField) Relation(relationField, labelField string) *SelectField {
	f.relation = RelationInfo{Field: relationField, Label: labelField}
	f.hasRel = true
	return f
}

// RelationInfo reports the Relation(...) call. ok is false when it was
// never called.
func (f *SelectField) RelationInfo() (RelationInfo, bool) { return f.relation, f.hasRel }

func (f *SelectField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func isSelectableKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.String:
		return true
	}
	return false
}

func (f *SelectField) Check(target reflect.Type) error {
	if !f.hasRel {
		return fmt.Errorf("form.Select(%q) needs .Relation(relationField, labelField)", f.name)
	}
	if !isSelectableKind(target.Kind()) {
		return fmt.Errorf("form.Select(%q) needs an integer or string field, the model field is %s", f.name, target)
	}
	return nil
}

func (f *SelectField) Format(v any) string {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	}
	return fmt.Sprint(v)
}

func (f *SelectField) Parse(raw string, target reflect.Type) (any, string) {
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
	case reflect.String:
		out.SetString(raw)
	}
	return out.Interface(), ""
}

func (f *SelectField) Render(v Value) templ.Component { return f.RenderOptions(v, nil) }

func (f *SelectField) RenderOptions(v Value, opts []Option) templ.Component {
	return selectInput(f, v, opts)
}
```

`reflect`, `strconv`, `strings`, `fmt` are already imported at the top of `form/fields.go`; `humanize` and `i18n` are already imported there too.

- [ ] **Step 5: Add the selectInput template to form/render.templ**

Append to the end of `form/render.templ`:

```templ
templ selectInput(f *SelectField, v Value, opts []Option) {
	<div class="field">
		@fieldLabel(f.name, f.label, f.required)
		<select
			class="input"
			id={ "f-" + f.name }
			name={ f.name }
			required?={ f.required }
			if v.Error != "" {
				aria-invalid="true"
				aria-describedby={ "e-" + f.name }
			}
		>
			if !f.required {
				<option value=""></option>
			}
			for _, o := range opts {
				<option value={ o.Value } selected?={ o.Value == v.Raw }>{ o.Label }</option>
			}
		</select>
		@fieldError(f.name, v.Error)
	</div>
}
```

- [ ] **Step 6: Regenerate templ and run the tests**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test ./form/... -run TestSelect -v`
Expected: PASS (all four tests)

- [ ] **Step 7: Write the failing render tests**

Add to `form/fields_test.go`, right after the `render` helper near the top of the file:

```go
func renderOptions(t *testing.T, f *SelectField, v Value, opts []Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := f.RenderOptions(v, opts).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
```

Then, alongside the other render tests near the bottom of the file:

```go
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
```

- [ ] **Step 8: Run the render tests to verify they fail for the right reason, then pass**

Run: `go test ./form/... -run TestRenderSelect -v`
Expected first: FAIL only if Step 6 was skipped (it should already pass, since `selectInput` was added in Step 5); if the assertions themselves are wrong, adjust them, not the production code, and rerun. Confirm PASS before continuing.

- [ ] **Step 9: Run the whole form package's tests, gofmt, and vet**

Run: `gofmt -l form/ && go vet ./form/... && go test ./form/...`
Expected: no gofmt output, no vet errors, all tests pass

- [ ] **Step 10: Commit**

```bash
git add form/field.go form/fields.go form/fields_test.go form/render.templ form/render_templ.go
git commit -m "feat: add form.Select bound to a relation's foreign key"
```

---

### Task 2: Resolve relations at registration

Reflects on the model to find the relation field, parses the related type's GORM schema through `b.db`, validates it, and builds a per-field option-loading closure stored on `resource[T]`.

**Files:**
- Create: `relation.go`
- Modify: `resource.go`
- Test: `relation_test.go`

**Interfaces:**
- Consumes: `form.RelationField`, `form.RelationInfo`, `form.Option` (Task 1); `isIntKind`, `isUintKind` (already defined in `gormsource.go`); `ResourceBuilder[T].db`, `resource[T]`, `register()` (already in `resource.go`).
- Produces:
  - `type optionLoader func(ctx context.Context) ([]form.Option, error)`
  - `func resolveRelation(db *gorm.DB, model reflect.Type, fkType reflect.Type, rel form.RelationInfo) (optionLoader, error)`
  - `resource[T].optionLoaders []optionLoader`, indexed the same as `resource[T].fields`

- [ ] **Step 1: Write the failing registration tests**

Create `relation_test.go`:

```go
package tellus

import (
	"context"
	"testing"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/testdb"
	"github.com/TechnoVizor/tellus/table"
)

// relCategory and relProduct are fixtures dedicated to relation tests, kept
// separate from the shared Product/Note fixtures in gormsource_test.go so
// this feature's tests do not reach into a widely reused type.
type relCategory struct {
	ID   uint
	Name string
}

type relProduct struct {
	ID         uint
	Name       string
	CategoryID uint
	Category   relCategory
}

// relProductStringKey has a string foreign key against relCategory's uint
// primary key, for the kind-family mismatch test.
type relProductStringKey struct {
	ID         uint
	CategoryID string
	Category   relCategory
}

// relStubSource is a DataSource[relProduct] used only to exercise
// Resource(nil).Source(...) in a registration test; its methods are never
// called during Register.
type relStubSource struct{}

func (relStubSource) List(context.Context, ListQuery) (ListResult[relProduct], error) {
	panic("unused")
}
func (relStubSource) Find(context.Context, string) (*relProduct, error) { panic("unused") }
func (relStubSource) Create(context.Context, *relProduct) error         { panic("unused") }
func (relStubSource) Update(context.Context, *relProduct) error         { panic("unused") }
func (relStubSource) Delete(context.Context, string) error              { panic("unused") }
func (relStubSource) ID(*relProduct) string                             { panic("unused") }

func TestRelationRegistrationRejectsBadInput(t *testing.T) {
	db := testdb.Open(t)
	nameCol := table.Text("Name")
	cases := map[string]Registrable{
		"select without Relation": Resource[relProduct](db).
			Table(nameCol).Form(form.Select("CategoryID")),
		"unknown relation field": Resource[relProduct](db).
			Table(nameCol).Form(form.Select("CategoryID").Relation("Nope", "Name")),
		"relation field not a struct": Resource[relProduct](db).
			Table(nameCol).Form(form.Select("CategoryID").Relation("Name", "Name")),
		"unknown label field": Resource[relProduct](db).
			Table(nameCol).Form(form.Select("CategoryID").Relation("Category", "Nope")),
		"nil db, custom source": Resource[relProduct](nil).Source(relStubSource{}).
			Table(nameCol).Form(form.Select("CategoryID").Relation("Category", "Name")),
	}
	for name, res := range cases {
		p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
		if err := p.Register(res); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestRelationRegistrationRejectsKindMismatch(t *testing.T) {
	db := testdb.Open(t)
	res := Resource[relProductStringKey](db).
		Table(table.Text("CategoryID")).
		Form(form.Select("CategoryID").Relation("Category", "Name"))
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err := p.Register(res); err == nil {
		t.Error("a string foreign key against a uint primary key must be rejected")
	}
}

func TestRelationRegistrationSucceeds(t *testing.T) {
	db := testdb.Open(t)
	res := Resource[relProduct](db).
		Table(table.Text("Name")).
		Form(form.Text("Name"), form.Select("CategoryID").Relation("Category", "Name"))
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err := p.Register(res); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail for the right reason**

Run: `go test . -run TestRelationRegistration -v`
Expected: `TestRelationRegistrationSucceeds` PASSes already (registration of a well-formed `Select` was already permissive before this task). `TestRelationRegistrationRejectsKindMismatch` FAILs (nothing yet checks the foreign key's kind against the related model). `TestRelationRegistrationRejectsBadInput` FAILs, but not uniformly: its "select without Relation" case already reports an error today, since `SelectField.Check` alone (Task 1) already requires `.Relation(...)` to have been called; the other four cases ("unknown relation field", "relation field not a struct", "unknown label field", "nil db, custom source") report no error yet, since nothing today resolves `.Relation(...)` against the related model. The test as a whole still FAILs because of those four. Confirm this exact pattern, not "every case fails", before moving on.

- [ ] **Step 3: Create relation.go**

```go
package tellus

import (
	"context"
	"fmt"
	"reflect"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/form"
)

// optionLoader loads a Select field's dropdown options for one request.
// nil for a field that is not a form.RelationField.
type optionLoader func(ctx context.Context) ([]form.Option, error)

// optionsCap bounds how many rows a relation's dropdown loads, so a very
// large related table cannot make every form render slowly. A searchable,
// paginated select is future work if a project needs more than this.
const optionsCap = 500

// resolveRelation validates a Select field's Relation(...) declaration
// against model (the resource's own model type) and fkType (the Select
// field's own bound Go type, already validated by SelectField.Check), and
// builds the field's per-request option loader. db must not be nil.
func resolveRelation(db *gorm.DB, model reflect.Type, fkType reflect.Type, rel form.RelationInfo) (optionLoader, error) {
	if db == nil {
		return nil, fmt.Errorf("tellus: relation %q needs a *gorm.DB (Resource(nil) with a custom Source cannot resolve it)", rel.Field)
	}
	sf, ok := model.FieldByName(rel.Field)
	if !ok || !sf.IsExported() {
		return nil, fmt.Errorf("tellus: relation field %q is not an exported field of %s", rel.Field, model)
	}
	relatedType := sf.Type
	for relatedType.Kind() == reflect.Pointer {
		relatedType = relatedType.Elem()
	}
	if relatedType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tellus: relation field %q of %s is not a struct", rel.Field, model)
	}

	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(reflect.New(relatedType).Interface()); err != nil {
		return nil, fmt.Errorf("tellus: relation %q: parse %s: %w", rel.Field, relatedType, err)
	}
	s := stmt.Schema
	if len(s.PrimaryFields) != 1 {
		return nil, fmt.Errorf("tellus: relation %q: %s needs exactly one primary key column", rel.Field, relatedType)
	}
	pk := s.PrimaryFields[0]
	if !sameKindFamily(fkType.Kind(), pk.FieldType.Kind()) {
		return nil, fmt.Errorf("tellus: relation %q: foreign key is %s, %s's primary key is %s, their kinds must match", rel.Field, fkType, relatedType, pk.FieldType)
	}
	labelSF, ok := s.FieldsByName[rel.Label]
	if !ok || labelSF.DBName == "" {
		return nil, fmt.Errorf("tellus: relation %q: %s has no column for label field %q", rel.Field, relatedType, rel.Label)
	}

	table, pkCol, labelCol := s.Table, pk.DBName, labelSF.DBName
	loader := func(ctx context.Context) ([]form.Option, error) {
		var rows []map[string]any
		// pkCol and labelCol are schema column names resolved once above,
		// never request input, so building the query from them is safe.
		q := db.WithContext(ctx).Table(table).Select(pkCol + ", " + labelCol).Order(labelCol).Limit(optionsCap)
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		opts := make([]form.Option, len(rows))
		for i, row := range rows {
			opts[i] = form.Option{Value: fmt.Sprint(row[pkCol]), Label: fmt.Sprint(row[labelCol])}
		}
		return opts, nil
	}
	return loader, nil
}

func sameKindFamily(a, b reflect.Kind) bool {
	switch {
	case isIntKind(a) && isIntKind(b):
		return true
	case isUintKind(a) && isUintKind(b):
		return true
	case a == reflect.String && b == reflect.String:
		return true
	}
	return false
}
```

- [ ] **Step 4: Wire resolveRelation into resource.go's register()**

In `resource.go`, add a field to the `resource[T]` struct (it currently ends with `searchFields []string` around line 84):

```go
	sortable     map[string]bool
	searchFields []string

	optionLoaders []optionLoader // parallel to fields; nil entry for a field with no relation
```

In `register()`, the per-field loop currently reads (around lines 159-178):

```go
	seen := map[string]bool{}
	needsStorage := false
	for _, f := range b.fields {
		info := f.Info()
		if seen[info.Name] {
			return fmt.Errorf("tellus: resource %q: form field %q is used twice", rs.slug, info.Name)
		}
		seen[info.Name] = true
		sf, ok := model.FieldByName(info.Name)
		if !ok || !sf.IsExported() {
			return fmt.Errorf("tellus: resource %q: form field %q is not an exported field of %s", rs.slug, info.Name, model)
		}
		if err := f.Check(sf.Type); err != nil {
			return fmt.Errorf("tellus: resource %q: %w", rs.slug, err)
		}
		if _, ok := f.(form.FileField); ok {
			needsStorage = true
		}
		rs.fields = append(rs.fields, f)
		rs.fieldIdx = append(rs.fieldIdx, sf.Index)
		rs.fieldType = append(rs.fieldType, sf.Type)
	}
```

Change it to:

```go
	seen := map[string]bool{}
	needsStorage := false
	for _, f := range b.fields {
		info := f.Info()
		if seen[info.Name] {
			return fmt.Errorf("tellus: resource %q: form field %q is used twice", rs.slug, info.Name)
		}
		seen[info.Name] = true
		sf, ok := model.FieldByName(info.Name)
		if !ok || !sf.IsExported() {
			return fmt.Errorf("tellus: resource %q: form field %q is not an exported field of %s", rs.slug, info.Name, model)
		}
		if err := f.Check(sf.Type); err != nil {
			return fmt.Errorf("tellus: resource %q: %w", rs.slug, err)
		}
		if _, ok := f.(form.FileField); ok {
			needsStorage = true
		}
		var loader optionLoader
		if rf, ok := f.(form.RelationField); ok {
			rel, _ := rf.RelationInfo() // Check already required this to be set
			var relErr error
			loader, relErr = resolveRelation(b.db, model, sf.Type, rel)
			if relErr != nil {
				return fmt.Errorf("tellus: resource %q: %w", rs.slug, relErr)
			}
		}
		rs.fields = append(rs.fields, f)
		rs.fieldIdx = append(rs.fieldIdx, sf.Index)
		rs.fieldType = append(rs.fieldType, sf.Type)
		rs.optionLoaders = append(rs.optionLoaders, loader)
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate`
Run: `go test . -run TestRelationRegistration -v`
Expected: PASS (all three tests)

- [ ] **Step 6: Run the whole root package's tests, gofmt, and vet**

Run: `gofmt -l . && go vet ./... && go test .`
Expected: no gofmt output, no vet errors, all tests pass (the one pre-existing unrelated failure, `TestLocalStorageRejectsFilenamesThatEscapeTheDirectory` on a non-Windows machine, is not something this task touches or fixes)

- [ ] **Step 7: Commit**

```bash
git add relation.go relation_test.go resource.go
git commit -m "feat: resolve belongsTo relations at registration and load their options"
```

---

### Task 3: Wire option loading into rendering, and the end-to-end behavior

`renderForm` calls the resolved loader and renders through `RenderOptions` for a field that has one; a load failure is a server error like any other. This task also proves the whole feature end to end against real Postgres.

**Files:**
- Modify: `resource_handlers.go`
- Test: `relation_test.go`

**Interfaces:**
- Consumes: `resource[T].optionLoaders` (Task 2), `form.OptionsField` (Task 1), `rs.panel.serverError` (already in `panel.go`).
- Produces: nothing new; this task only wires existing pieces together and proves the result.

- [ ] **Step 1: Write the failing end-to-end tests**

Add to `relation_test.go`:

```go
func TestSelectRendersSavesAndClearsRelation(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategory{}, &relProduct{}); err != nil {
		t.Fatal(err)
	}
	books := relCategory{Name: "Books"}
	music := relCategory{Name: "Music"}
	db.Create(&books)
	db.Create(&music)

	res := Resource[relProduct](db).Slug("rel-products").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Name"))
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	body := h.get("/rel-products/new").body
	mustContain(t, body,
		`<option value=""></option>`,
		fmt.Sprintf(`value="%d"`, books.ID), "Books",
		fmt.Sprintf(`value="%d"`, music.ID), "Music",
	)
	if strings.Index(body, "Books") > strings.Index(body, "Music") {
		t.Error("options must be ordered by label")
	}

	res2 := h.post("/rel-products", url.Values{"Name": {"Atlas"}, "CategoryID": {fmt.Sprint(books.ID)}})
	if res2.status != http.StatusSeeOther {
		t.Fatalf("create: %d %s", res2.status, res2.body)
	}
	var saved relProduct
	if err := db.Where("name = ?", "Atlas").First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CategoryID != books.ID {
		t.Fatalf("stored CategoryID = %d, want %d", saved.CategoryID, books.ID)
	}

	editBody := h.get(fmt.Sprintf("/rel-products/%d/edit", saved.ID)).body
	mustContain(t, editBody, fmt.Sprintf(`value="%d" selected`, books.ID))

	res3 := h.post(fmt.Sprintf("/rel-products/%d", saved.ID), url.Values{"Name": {"Atlas"}, "CategoryID": {""}})
	if res3.status != http.StatusSeeOther {
		t.Fatalf("clear relation: %d %s", res3.status, res3.body)
	}
	var cleared relProduct
	db.First(&cleared, saved.ID)
	if cleared.CategoryID != 0 {
		t.Fatalf("CategoryID after clearing = %d, want 0", cleared.CategoryID)
	}
}

func TestSelectRequiredEmptyFails(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategory{}, &relProduct{}); err != nil {
		t.Fatal(err)
	}
	cat := relCategory{Name: "Books"}
	db.Create(&cat)

	res := Resource[relProduct](db).Slug("rel-products").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Name").Required())
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	res2 := h.post("/rel-products", url.Values{"Name": {"Atlas"}, "CategoryID": {""}})
	if res2.status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", res2.status)
	}
	var count int64
	db.Model(&relProduct{}).Count(&count)
	if count != 0 {
		t.Fatalf("record was saved despite the required relation being empty, count = %d", count)
	}
}

func TestSelectOptionsAreCappedAndOrdered(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategory{}, &relProduct{}); err != nil {
		t.Fatal(err)
	}
	cats := make([]relCategory, 501)
	for i := range cats {
		cats[i] = relCategory{Name: fmt.Sprintf("Category %03d", i)}
	}
	if err := db.CreateInBatches(&cats, 100).Error; err != nil {
		t.Fatal(err)
	}

	res := Resource[relProduct](db).Slug("rel-products").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Name"))
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	body := h.get("/rel-products/new").body
	if got := strings.Count(body, "<option"); got != 501 { // 500 capped rows plus the leading empty option
		t.Errorf("option count = %d, want 501", got)
	}
	if !strings.Contains(body, "Category 000") {
		t.Error("the alphabetically first category must be included")
	}
	if strings.Contains(body, "Category 500") {
		t.Error("the 501st category, past the cap, must not be loaded")
	}
}

func TestSelectOptionLoadFailureIsServerError(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategory{}, &relProduct{}); err != nil {
		t.Fatal(err)
	}
	res := Resource[relProduct](db).Slug("rel-products").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Name"))
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	if err := db.Migrator().DropTable(&relCategory{}); err != nil {
		t.Fatal(err)
	}
	res2 := h.get("/rel-products/new")
	if res2.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res2.status)
	}
	if !strings.Contains(res2.body, "Something went wrong on our side") {
		t.Errorf("expected the generic server-error message, got: %s", res2.body)
	}
}
```

Add `"net/http"`, `"net/url"`, `"strings"`, `"fmt"` to `relation_test.go`'s import block (`context`, `testing`, and the three `github.com/TechnoVizor/tellus/...` imports are already there from Task 2).

- [ ] **Step 2: Run the tests to verify they fail for the right reason**

Run: `go test . -run TestSelect -v`
Expected: FAIL. `TestSelectRendersSavesAndClearsRelation` and `TestSelectOptionsAreCappedAndOrdered` fail because `renderForm` still calls plain `Render` (options never load, so `<option>` rows for the relation are missing). `TestSelectOptionLoadFailureIsServerError` currently returns 200 with no options rather than 500, since nothing loads them yet. `TestSelectRequiredEmptyFails` may already pass, since `SelectField.Parse` alone (Task 1) already rejects an empty required submission; confirm it does and move on regardless.

- [ ] **Step 3: Wire option loading into renderForm**

In `resource_handlers.go`, `renderForm` currently reads:

```go
func (rs *resource[T]) renderForm(w http.ResponseWriter, r *http.Request, u User, status int, heading, action string, values map[string]form.Value, summary string) {
	comps := make([]templ.Component, len(rs.fields))
	for i, f := range rs.fields {
		comps[i] = f.Render(values[f.Info().Name])
	}
	rs.panel.render(w, r, status, ui.FormPage(ui.FormView{
		Shell:     rs.panel.shell(r, u, heading, rs.slug),
		Heading:   heading,
		Action:    action,
		CancelURL: rs.listURL(listParams{}),
		Error:     summary,
		Fields:    comps,
	}))
}
```

Change it to:

```go
func (rs *resource[T]) renderForm(w http.ResponseWriter, r *http.Request, u User, status int, heading, action string, values map[string]form.Value, summary string) {
	comps := make([]templ.Component, len(rs.fields))
	for i, f := range rs.fields {
		v := values[f.Info().Name]
		if loader := rs.optionLoaders[i]; loader != nil {
			opts, err := loader(r.Context())
			if err != nil {
				rs.panel.serverError(w, err)
				return
			}
			comps[i] = f.(form.OptionsField).RenderOptions(v, opts)
			continue
		}
		comps[i] = f.Render(v)
	}
	rs.panel.render(w, r, status, ui.FormPage(ui.FormView{
		Shell:     rs.panel.shell(r, u, heading, rs.slug),
		Heading:   heading,
		Action:    action,
		CancelURL: rs.listURL(listParams{}),
		Error:     summary,
		Fields:    comps,
	}))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run TestSelect -v`
Expected: PASS (all four tests)

- [ ] **Step 5: Run the whole project's tests, gofmt, and vet**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, no vet errors, every test passes except the one pre-existing unrelated failure noted in Task 2 Step 6

- [ ] **Step 6: Commit**

```bash
git add relation_test.go resource_handlers.go
git commit -m "test: cover the belongsTo relation end to end"
```

---

## After this plan

`examples/shop` still has one `Product` resource with no relations. Wiring a `Category` into the demo (a `Product.CategoryID` plus `form.Select("CategoryID").Relation("Category", "Name")`) is a natural next step but is not part of this plan; do it as a small follow-up once this lands, the way Date and Money were each wired into the example after their own features shipped.
