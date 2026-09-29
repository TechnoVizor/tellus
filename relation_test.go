package tellus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	// gorm:"-": keeps Category visible to reflection (what resolveRelation
	// needs) but invisible to GORM's own migration/association handling, so
	// AutoMigrate does not add a real FK constraint. A real constraint would
	// reject clearing the relation (storing the zero value, no category has
	// id 0), a database-level concern this feature's own registration-time
	// validation does not take over; the tests care about the panel's own
	// behavior.
	Category relCategory `gorm:"-"`
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
