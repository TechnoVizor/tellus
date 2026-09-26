package tellus

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

// --- fake data source ---

// memSource is an in-memory DataSource[Product]. It only paginates, and it
// records the last ListQuery so tests can check what the handler asked for.
type memSource struct {
	items     []Product
	nextID    uint
	queries   []ListQuery
	failWith  error
	updateErr error
}

func newMemSource(n int) *memSource {
	s := &memSource{nextID: 1}
	for i := 1; i <= n; i++ {
		s.items = append(s.items, Product{ID: s.nextID, Name: fmt.Sprintf("Product %02d", i), Price: i * 10, Active: i%2 == 0})
		s.nextID++
	}
	return s
}

func (s *memSource) last() ListQuery { return s.queries[len(s.queries)-1] }

func (s *memSource) List(_ context.Context, q ListQuery) (ListResult[Product], error) {
	s.queries = append(s.queries, q)
	if s.failWith != nil {
		return ListResult[Product]{}, s.failWith
	}
	per := q.PerPage
	if per < 1 {
		per = defaultPerPage
	}
	page := max(q.Page, 1)
	start := min((page-1)*per, len(s.items))
	end := min(start+per, len(s.items))
	return ListResult[Product]{Items: append([]Product(nil), s.items[start:end]...), Total: int64(len(s.items))}, nil
}

func (s *memSource) index(id string) int {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return -1
	}
	for i, p := range s.items {
		if uint64(p.ID) == n {
			return i
		}
	}
	return -1
}

func (s *memSource) Find(_ context.Context, id string) (*Product, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	i := s.index(id)
	if i < 0 {
		return nil, ErrNotFound
	}
	p := s.items[i]
	return &p, nil
}

func (s *memSource) Create(_ context.Context, p *Product) error {
	if s.failWith != nil {
		return s.failWith
	}
	p.ID = s.nextID
	s.nextID++
	s.items = append(s.items, *p)
	return nil
}

func (s *memSource) Update(_ context.Context, p *Product) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	i := s.index(fmt.Sprint(p.ID))
	if i < 0 {
		return ErrNotFound
	}
	s.items[i] = *p
	return nil
}

func (s *memSource) Delete(_ context.Context, id string) error {
	if s.failWith != nil {
		return s.failWith
	}
	if i := s.index(id); i >= 0 {
		s.items = append(s.items[:i], s.items[i+1:]...)
	}
	return nil
}

func (s *memSource) ID(p *Product) string { return fmt.Sprint(p.ID) }

var errBoom = errors.New("boom: secret database detail")

// productResource is the resource used by most tests.
func productResource(src DataSource[Product]) *ResourceBuilder[Product] {
	return Resource[Product](nil).Source(src).
		Table(
			table.Text("Name").Searchable().Sortable(),
			table.Text("Price").Sortable(),
			table.Boolean("Active"),
		).
		Form(
			form.Text("Name").Required().MaxLength(100),
			form.Number("Price").Required(),
			form.Toggle("Active"),
		)
}
