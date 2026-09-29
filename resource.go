package tellus

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/table"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// reservedSlugs are panel routes a resource must not shadow.
var reservedSlugs = map[string]bool{"login": true, "logout": true, "assets": true}

// ResourceBuilder describes how a model appears in the panel. Create one with
// Resource and chain Table and Form.
type ResourceBuilder[T any] struct {
	db       *gorm.DB
	source   DataSource[T]
	slug     string
	singular string
	plural   string
	perPage  int
	columns  []table.Column
	fields   []form.Field
}

// Resource starts describing the model T. db backs the built-in GORM data
// source; it may be nil when Source is used.
func Resource[T any](db *gorm.DB) *ResourceBuilder[T] {
	return &ResourceBuilder[T]{db: db}
}

// Source replaces the built-in GORM data source.
func (b *ResourceBuilder[T]) Source(ds DataSource[T]) *ResourceBuilder[T] { b.source = ds; return b }

// Slug sets the URL segment. Default: the table name, for example "products".
func (b *ResourceBuilder[T]) Slug(slug string) *ResourceBuilder[T] { b.slug = slug; return b }

// Label sets the singular and plural names shown in the UI.
func (b *ResourceBuilder[T]) Label(singular, plural string) *ResourceBuilder[T] {
	b.singular, b.plural = singular, plural
	return b
}

// PerPage sets the page size of the list. Default 25, maximum 200.
func (b *ResourceBuilder[T]) PerPage(n int) *ResourceBuilder[T] { b.perPage = n; return b }

// Table sets the columns of the list.
func (b *ResourceBuilder[T]) Table(columns ...table.Column) *ResourceBuilder[T] {
	b.columns = columns
	return b
}

// Form sets the fields of the create and edit form.
func (b *ResourceBuilder[T]) Form(fields ...form.Field) *ResourceBuilder[T] {
	b.fields = fields
	return b
}

// resource is a validated ResourceBuilder bound to a panel.
type resource[T any] struct {
	panel    *Panel
	source   DataSource[T]
	slug     string
	singular string
	plural   string
	perPage  int

	columns  []table.Column
	colIndex [][]int // reflect field index path per column

	fields    []form.Field
	fieldIdx  [][]int
	fieldType []reflect.Type

	sortable     map[string]bool
	searchFields []string

	optionLoaders []optionLoader // parallel to fields; nil entry for a field with no relation
}

func (b *ResourceBuilder[T]) register(p *Panel) error {
	model := reflect.TypeFor[T]()
	if model.Kind() != reflect.Struct {
		return fmt.Errorf("tellus: Resource[%s] needs a struct type", model)
	}

	source, tableName := b.source, ""
	if source == nil {
		gs, err := NewGormSource[T](b.db)
		if err != nil {
			return err
		}
		source, tableName = gs, gs.schema.Table
	}
	if tableName == "" {
		tableName = strings.ToLower(model.Name()) + "s"
	}

	rs := &resource[T]{
		panel:    p,
		source:   source,
		slug:     b.slug,
		singular: b.singular,
		plural:   b.plural,
		perPage:  b.perPage,
		sortable: map[string]bool{},
	}
	if rs.slug == "" {
		rs.slug = strings.ReplaceAll(tableName, "_", "-")
	}
	if rs.singular == "" {
		rs.singular = humanize.Name(model.Name())
	}
	if rs.plural == "" {
		rs.plural = humanize.Name(tableName)
	}
	if rs.perPage <= 0 {
		rs.perPage = defaultPerPage
	}
	rs.perPage = min(rs.perPage, maxPerPage)

	if !slugPattern.MatchString(rs.slug) || reservedSlugs[rs.slug] {
		return fmt.Errorf("tellus: resource slug %q is invalid or reserved", rs.slug)
	}
	if p.slugs[rs.slug] {
		return fmt.Errorf("tellus: resource slug %q is already registered", rs.slug)
	}
	if len(b.columns) == 0 {
		return fmt.Errorf("tellus: resource %q needs at least one column, call Table(...)", rs.slug)
	}
	if len(b.fields) == 0 {
		return fmt.Errorf("tellus: resource %q needs at least one form field, call Form(...)", rs.slug)
	}

	for _, c := range b.columns {
		info := c.Info()
		sf, ok := model.FieldByName(info.Name)
		if !ok || !sf.IsExported() {
			return fmt.Errorf("tellus: resource %q: column %q is not an exported field of %s", rs.slug, info.Name, model)
		}
		rs.columns = append(rs.columns, c)
		rs.colIndex = append(rs.colIndex, sf.Index)
		if info.Sortable {
			rs.sortable[info.Name] = true
		}
		if info.Searchable {
			rs.searchFields = append(rs.searchFields, info.Name)
		}
	}

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
	if needsStorage {
		if err := p.ensureStorage(); err != nil {
			return fmt.Errorf("tellus: resource %q: %w", rs.slug, err)
		}
	}

	p.slugs[rs.slug] = true
	p.nav = append(p.nav, navEntry{slug: rs.slug, label: rs.plural})
	p.dashboardCards = append(p.dashboardCards, dashboardCard{
		label: rs.plural,
		href:  p.url("/" + rs.slug),
		countFn: func(ctx context.Context) (int64, error) {
			res, err := rs.source.List(ctx, ListQuery{PerPage: 1})
			if err != nil {
				return 0, err
			}
			return res.Total, nil
		},
		seriesFn: resolveTimeSeries(b.db, model),
	})
	p.mounters = append(p.mounters, rs.mount)
	return nil
}
