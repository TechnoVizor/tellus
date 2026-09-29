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
