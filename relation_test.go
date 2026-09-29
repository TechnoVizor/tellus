package tellus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gorm.io/gorm"

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

// relProduct's CategoryID is a *uint (not uint): AutoMigrate creates a real
// foreign key constraint from the CategoryID+Category naming convention, and
// a real constraint rejects a plain zero value (no category has id 0) when
// the relation is optional. A pointer stores NULL instead, which the
// constraint accepts.
type relProduct struct {
	ID         uint
	Name       string
	CategoryID *uint
	Category   relCategory
}

// relProductStringKey has a string foreign key against relCategory's uint
// primary key, for the kind-family mismatch test.
type relProductStringKey struct {
	ID         uint
	CategoryID string
	Category   relCategory
}

// relCategorySoft and relProductSoft prove a soft-deleted related row does
// not appear in the dropdown.
type relCategorySoft struct {
	ID        uint
	Name      string
	DeletedAt gorm.DeletedAt
}

type relProductSoft struct {
	ID         uint
	Name       string
	CategoryID *uint
	Category   relCategorySoft
}

// relCategoryReserved and relProductReserved prove a label field whose
// column name is a SQL reserved word does not crash the options query.
type relCategoryReserved struct {
	ID    uint
	Order string
}

type relProductReserved struct {
	ID         uint
	Name       string
	CategoryID *uint
	Category   relCategoryReserved
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
	if saved.CategoryID == nil || *saved.CategoryID != books.ID {
		t.Fatalf("stored CategoryID = %v, want %d", saved.CategoryID, books.ID)
	}

	editBody := h.get(fmt.Sprintf("/rel-products/%d/edit", saved.ID)).body
	mustContain(t, editBody, fmt.Sprintf(`value="%d" selected`, books.ID))

	res3 := h.post(fmt.Sprintf("/rel-products/%d", saved.ID), url.Values{"Name": {"Atlas"}, "CategoryID": {""}})
	if res3.status != http.StatusSeeOther {
		t.Fatalf("clear relation: %d %s", res3.status, res3.body)
	}
	var cleared relProduct
	db.First(&cleared, saved.ID)
	if cleared.CategoryID != nil {
		t.Fatalf("CategoryID after clearing = %v, want nil", *cleared.CategoryID)
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
	// Inserted in reverse label order, so the assertions below only pass if
	// the loader really orders by label: insertion/primary-key order alone
	// would surface "Category 500" (inserted first) and drop "Category 000"
	// (inserted last, past the cap).
	cats := make([]relCategory, 501)
	for i := range cats {
		cats[i] = relCategory{Name: fmt.Sprintf("Category %03d", 500-i)}
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

func TestSelectOptionsExcludeSoftDeletedRelatedRows(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategorySoft{}, &relProductSoft{}); err != nil {
		t.Fatal(err)
	}
	keep := relCategorySoft{Name: "Keep"}
	gone := relCategorySoft{Name: "Gone"}
	db.Create(&keep)
	db.Create(&gone)
	if err := db.Delete(&gone).Error; err != nil {
		t.Fatal(err)
	}

	res := Resource[relProductSoft](db).Slug("rel-products-soft").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Name"))
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	body := h.get("/rel-products-soft/new").body
	if !strings.Contains(body, "Keep") {
		t.Error("a non-deleted category must be offered")
	}
	if strings.Contains(body, "Gone") {
		t.Error("a soft-deleted category must not be offered")
	}
}

func TestSelectOptionsHandleReservedWordLabelColumn(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&relCategoryReserved{}, &relProductReserved{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&relCategoryReserved{Order: "First"})

	res := Resource[relProductReserved](db).Slug("rel-products-reserved").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Select("CategoryID").Relation("Category", "Order"))
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	res2 := h.get("/rel-products-reserved/new")
	if res2.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res2.status, res2.body)
	}
	if !strings.Contains(res2.body, "First") {
		t.Errorf("missing option label: %s", res2.body)
	}
}
