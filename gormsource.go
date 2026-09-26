package tellus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const (
	defaultPerPage = 25
	maxPerPage     = 200
)

// GormSource is a DataSource backed by GORM. The model needs exactly one
// primary key column of an integer or string type.
//
// ponytail: text search uses Postgres ILIKE. Use LOWER(col) LIKE LOWER(?) when
// other databases are supported.
type GormSource[T any] struct {
	db     *gorm.DB
	schema *schema.Schema
	pk     *schema.Field
}

// NewGormSource parses the model T and returns a source for it.
func NewGormSource[T any](db *gorm.DB) (*GormSource[T], error) {
	if db == nil {
		return nil, errors.New("tellus: NewGormSource needs a *gorm.DB")
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(new(T)); err != nil {
		return nil, fmt.Errorf("tellus: parse model: %w", err)
	}
	s := stmt.Schema
	if len(s.PrimaryFields) != 1 {
		return nil, fmt.Errorf("tellus: model %s needs exactly one primary key column", s.Name)
	}
	pk := s.PrimaryFields[0]
	switch k := pk.FieldType.Kind(); {
	case isIntKind(k), isUintKind(k), k == reflect.String:
	default:
		return nil, fmt.Errorf("tellus: model %s has a %s primary key, only integer and string keys are supported", s.Name, pk.FieldType)
	}
	return &GormSource[T]{db: db, schema: s, pk: pk}, nil
}

func (s *GormSource[T]) List(ctx context.Context, q ListQuery) (ListResult[T], error) {
	page, perPage := q.Page, q.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	base := s.db.WithContext(ctx).Model(new(T))
	if q.Search != "" && len(q.SearchFields) > 0 {
		pattern := "%" + escapeLike(q.Search) + "%"
		conds := make([]string, 0, len(q.SearchFields))
		args := make([]any, 0, len(q.SearchFields))
		for _, name := range q.SearchFields {
			f, err := s.column(name)
			if err != nil {
				return ListResult[T]{}, err
			}
			conds = append(conds, "CAST("+s.db.Statement.Quote(f.DBName)+` AS text) ILIKE ? ESCAPE '\'`)
			args = append(args, pattern)
		}
		base = base.Where("("+strings.Join(conds, " OR ")+")", args...)
	}

	var res ListResult[T]
	if err := base.Session(&gorm.Session{}).Count(&res.Total).Error; err != nil {
		return ListResult[T]{}, err
	}

	rows := base.Session(&gorm.Session{})
	if q.SortField != "" {
		f, err := s.column(q.SortField)
		if err != nil {
			return ListResult[T]{}, err
		}
		rows = rows.Order(clause.OrderByColumn{Column: clause.Column{Name: f.DBName}, Desc: q.SortDesc})
	}
	// Primary key as a tiebreaker keeps pages stable when sort values repeat.
	rows = rows.Order(clause.OrderByColumn{Column: clause.Column{Name: s.pk.DBName}})
	if err := rows.Limit(perPage).Offset((page - 1) * perPage).Find(&res.Items).Error; err != nil {
		return ListResult[T]{}, err
	}
	return res, nil
}

func (s *GormSource[T]) Find(ctx context.Context, id string) (*T, error) {
	key, err := s.parseID(id)
	if err != nil {
		return nil, err
	}
	// Limit(1).Find rather than First: a missing record is normal here, and First
	// would log it as an error.
	var item T
	tx := s.db.WithContext(ctx).Where(clause.Eq{Column: clause.Column{Name: s.pk.DBName}, Value: key}).Limit(1).Find(&item)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &item, nil
}

func (s *GormSource[T]) Create(ctx context.Context, item *T) error {
	return s.db.WithContext(ctx).Create(item).Error
}

// Update writes every column of item, zero values included. It never inserts:
// a record deleted in the meantime yields ErrNotFound.
func (s *GormSource[T]) Update(ctx context.Context, item *T) error {
	tx := s.db.WithContext(ctx).Model(item).Select("*").Omit(s.pk.DBName).Updates(item)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the record. Deleting a missing record is not an error.
func (s *GormSource[T]) Delete(ctx context.Context, id string) error {
	key, err := s.parseID(id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where(clause.Eq{Column: clause.Column{Name: s.pk.DBName}, Value: key}).Delete(new(T)).Error
}

func (s *GormSource[T]) ID(item *T) string {
	v, _ := s.pk.ValueOf(context.Background(), reflect.ValueOf(item))
	return fmt.Sprint(v)
}

func (s *GormSource[T]) column(name string) (*schema.Field, error) {
	f, ok := s.schema.FieldsByName[name]
	if !ok || f.DBName == "" {
		return nil, fmt.Errorf("tellus: model %s has no column for field %q", s.schema.Name, name)
	}
	return f, nil
}

// parseID converts a URL id into the primary key's Go type. An id that cannot
// be that type cannot exist, so it reports ErrNotFound.
func (s *GormSource[T]) parseID(id string) (any, error) {
	switch k := s.pk.FieldType.Kind(); {
	case isIntKind(k):
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, ErrNotFound
		}
		return n, nil
	case isUintKind(k):
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n > math.MaxInt64 {
			return nil, ErrNotFound
		}
		return int64(n), nil
	default:
		return id, nil
	}
}

func isIntKind(k reflect.Kind) bool  { return k >= reflect.Int && k <= reflect.Int64 }
func isUintKind(k reflect.Kind) bool { return k >= reflect.Uint && k <= reflect.Uint64 }

// escapeLike escapes the characters that are special in a LIKE pattern so user
// input is matched literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
