# belongsTo relations and form.Select: design

Status: approved, 2026-09-29. Source: design conversation with the project owner.

## 1. Goal

Phase 1 (`docs/spec.md` section 8) lists belongsTo as a select field and Select as a
form field. This spec covers that one relation kind: a foreign key field
(for example `Product.CategoryID`) rendered as a `<select>` populated from
the related table (`categories`), with the chosen row's primary key stored
back into the foreign key column on save.

hasMany and manyToMany, and a struct-tag-free `.Options(...)` for a plain
enum-like select with no relation, are out of scope here. Nothing in this
design blocks adding either later.

## 2. Public API

```go
form.Select("CategoryID").
    Relation("Category", "Name"). // Go field on the model, label field on the related model
    Required()
```

`Select(name)` binds to the named field on the model, the same as every
other field builder. `.Relation(relationField, labelField)` is required for
a `Select` in this phase; a `Select` without it fails `Check` at
registration, the same "reported at startup" guarantee the rest of the
builder API already gives.

- `relationField` ("Category") is the Go struct field on the model that
  holds the association, `Category Category` or `Category *Category`. Its
  type, pointer stripped, is the related model.
- `labelField` ("Name") is a field name on the related model, resolved by
  reflection, matching the existing convention (`table.Text("Name")` is
  already a bare field name string everywhere else in this API).

`SelectField.Check` accepts an int-family or string field, the same
families `NumberField` and `TextField` already accept, since a foreign key
is ordinarily one of those. Format and Parse work the same way regardless
of `.Relation` being an association or, later, a plain enum: they convert
between the field's Go type and the string that travels through
`form.Value` and the submitted form, exactly like `NumberField` does for
int/uint/float today.

## 3. Registration-time resolution (resource.go)

Resolved once in `register()`, the same place that already reflects over
`b.columns` and `b.fields` and fails fast on a bad field name:

1. Look up `relationField` on model `T` via `reflect.Type.FieldByName`,
   unwrap one pointer level. Not found, not exported, or not a struct:
   registration error.
2. Parse the related type's schema with `b.db` (the `*gorm.DB` passed to
   `Resource[T](db)`, independent of whether `.Source(...)` replaced the
   primary `DataSource[T]`), the same `gorm.Statement{DB: b.db}.Parse(...)`
   call `NewGormSource` already uses, given `reflect.New(relatedType).Interface()`
   instead of `new(T)`. This needs no generic type parameter for the
   related model, `Statement.Parse` takes `any`.
3. From that schema: the table name, the primary key field (name, DB
   column, Go kind), and `labelField` resolved to its own DB column. The
   related model must have exactly one primary key column, the same rule
   `NewGormSource` already enforces for the primary model. A missing
   label field, a label field with no DB column, or a primary key that
   is not exactly one column: registration error.
4. Confirm the `Select` field's own bound Go kind (see `Check` above) is in
   the same family as the related model's primary key kind, using the
   `isIntKind` / `isUintKind` helpers `gormsource.go` already defines (both
   int-family, both uint-family, or both string). Mismatched families:
   registration error, since a value from one could never round-trip into
   the other.
5. Build a closure, `func(ctx context.Context) ([]form.Option, error)`,
   closing over `b.db`, the resolved table name, and the primary key and
   label DB column names. Store it on `resource[T]` in a slice parallel to
   `rs.fields`, `nil` for a field that is not `form.OptionsField` (mirrors
   `rs.colIndex` / `rs.fieldIdx` / `rs.fieldType`, already parallel slices
   populated once at registration and read per request).

Relations are GORM-only for this phase: the closure always uses `b.db`
directly, never the resource's own `DataSource[T]`. A host that replaced
`DataSource[T]` via `.Source(...)` for the primary model still needs a
working `*gorm.DB` for `Relation` to resolve, consistent with `ImageField`
and `Storage` already being local-disk-only in phase 1 (`docs/spec.md`
section 9 lists further adapters as separate, later work).

## 4. Request-time option loading and rendering

New interface in `form`, no GORM import in that package:

```go
// Option is one entry of a select populated from outside the field itself,
// for example a belongsTo relation.
type Option struct{ Value, Label string }

// OptionsField is implemented by a field whose Render needs option data
// only the panel can supply (a relation's rows). The panel calls
// RenderOptions instead of Render for such a field.
type OptionsField interface {
	Field
	RenderOptions(v Value, opts []Option) templ.Component
}
```

`SelectField` implements both `Render` (delegates to `RenderOptions(v, nil)`,
so the field is still safely usable, if optionless, through the plain
`Field` interface in a direct unit test) and `RenderOptions`.

In `resource_handlers.go`, `renderForm` currently calls `f.Render(values[...])`
for every field unconditionally. It changes to: if `rs.optionLoaders[i]` is
set, call it with the request's context to get `[]form.Option`; a load
error goes through `rs.panel.serverError`, the same path every other
`DataSource`/GORM error in this file already takes; otherwise call
`of.RenderOptions(v, opts)` where `of` is the field asserted to
`form.OptionsField`. A field with no loader keeps calling plain `Render`.

The loader queries only the two needed columns, ordered by label, capped at
500 rows: `SELECT <pk>, <label> FROM <table> ORDER BY <label> LIMIT 500`,
scanned into `[]map[string]any` through `b.db.WithContext(ctx).Table(...).
Select(...).Order(...).Limit(500).Find(&rows)`. This needs no reflected
instance of the related Go type at request time, only the column names
resolved once at registration. Each row's two values become an `Option`
through `fmt.Sprint`, the same loose, kind-agnostic formatting
`table.TextColumn`'s `display` helper already uses, so the label field is
not restricted to a string Go kind. 500 rows is a starting cap for phase 1, a
searchable/paginated select is later work if a real project needs it
(`docs/spec.md` section 9 territory), not attempted here.

When the field is not `.Required()`, `RenderOptions` renders one leading
`<option value="">` before the loaded rows, so clearing the relation is
possible from the UI. A required field renders no such option; an empty
submission fails `Parse` with `validation.required`, the same as every
other required field.

## 5. Parse, Format, Check

Same shape as `NumberField`, widened to also accept a string kind:

- `Check(target reflect.Type)`: accepts int-family, uint-family, or string.
  Anything else is a registration error naming the field and its actual
  type, matching every other `Check` implementation's error message style.
- `Format(v any) string`: int/uint kinds format with `strconv`, exactly as
  `NumberField.Format` does today; a string kind is returned as-is.
- `Parse(raw string, target reflect.Type) (any, string)`: empty and not
  required stores the zero value; empty and required returns
  `validation.required`; otherwise converts `raw` to `target`'s kind the
  same way `NumberField.Parse` does for int/uint, or passes a string
  through unchanged. Parse never touches the database, it only converts
  the submitted primary key string into the foreign key field's Go type,
  the relation's rows are irrelevant to this step (a stale or malicious id
  that names no real row is a foreign key constraint concern for the
  database, not something `Parse` can or should check).

## 6. Error handling

An option-load failure (the related table's query errors) is treated as a
server error: `rs.panel.serverError(w, err)`, the same response every
existing `DataSource`/GORM failure already produces in `list`, `editForm`,
`create`, and `update`. It is not a validation message on the field, since
the field itself is not what failed.

## 7. Testing

- `form` package, TDD as with every other field: `Check` accepts/rejects
  the right kinds, `Format`/`Parse` round-trip int, uint, and string keys,
  empty/required behavior, `RenderOptions` renders the right `<option>`
  elements (escaped, `selected` on the current value, the leading empty
  option only when not required).
- Root package: an integration test alongside the existing Postgres-backed
  tests (`internal/testdb`), registering a resource with a real `Select`
  and `Relation`, covering registration-time rejection (bad relation field,
  bad label field, mismatched key kinds) and the full request cycle
  (option list renders, a chosen value saves, clearing an optional relation
  works, an option-load failure surfaces as a server error).

## 8. Deferred

hasMany (read-only list on the record page), manyToMany (multi-select), a
relation-free `.Options(...)` for a plain enum select, search or pagination
within the select itself, and any non-GORM `DataSource` support for
relations.
