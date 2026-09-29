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
