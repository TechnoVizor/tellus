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

// TestM0ProductResourceEndToEnd is the milestone check: one builder
// description, real Postgres, the built-in accounts, and the whole flow a
// person goes through in the browser.
func TestM0ProductResourceEndToEnd(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatal(err)
	}
	auth := NewGormAuthenticator(db)
	if err := auth.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := auth.CreateUser(context.Background(), "admin@example.com", "Ada", "correct horse"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 30; i++ {
		db.Create(&Product{Name: fmt.Sprintf("Item %02d", i), Price: i * 10, Active: i%2 == 0})
	}
	db.Create(&Product{Name: "100% cotton", Price: 5})

	// The resource description shown in the README. Keep it under 30 lines.
	products := Resource[Product](db).
		Table(
			table.Text("Name").Searchable().Sortable(),
			table.Text("Price").Sortable(),
			table.Boolean("Active"),
		).
		Form(
			form.Text("Name").Required().MaxLength(100),
			form.Number("Price").Required(),
			form.Toggle("Active"),
		).
		PerPage(10)

	h := newHarness(t, Config{Authenticator: auth}, products)

	// Not signed in: everything redirects to the login page.
	if res := h.get("/products"); res.status != http.StatusSeeOther {
		t.Fatalf("anonymous list: %d", res.status)
	}
	if res := h.login("admin@example.com", "wrong password"); res.status != http.StatusUnauthorized {
		t.Fatalf("bad login: %d", res.status)
	}
	h.loginAdmin()

	// List, pagination, sorting.
	res := h.get("/products")
	mustContain(t, res.body, "31 records, page 1 of 4", "Item 01", "Item 10")
	if strings.Contains(res.body, "Item 11") {
		t.Error("first page shows more than PerPage rows")
	}
	res = h.get("/products?sort=Price&dir=desc")
	if i, j := strings.Index(res.body, "Item 30"), strings.Index(res.body, "Item 29"); i < 0 || j < 0 || i > j {
		t.Errorf("descending price order wrong (Item 30 at %d, Item 29 at %d)", i, j)
	}
	res = h.get("/products?page=4")
	mustContain(t, res.body, "31 records, page 4 of 4", "100% cotton")

	// Search is case-insensitive and treats % literally.
	res = h.get("/products?q=item+07")
	mustContain(t, res.body, "1 records", "Item 07")
	res = h.get("/products?q=100%25")
	mustContain(t, res.body, "1 records", "100% cotton")
	res = h.get("/products?q=%25")
	mustContain(t, res.body, "1 records")

	// Create.
	res = h.post("/products", url.Values{"Name": {"Desk lamp"}, "Price": {"42"}, "Active": {"true"}})
	if res.status != http.StatusSeeOther {
		t.Fatalf("create: %d %s", res.status, res.body)
	}
	var lamp Product
	if err := db.Where("name = ?", "Desk lamp").First(&lamp).Error; err != nil {
		t.Fatal(err)
	}
	if lamp.Price != 42 || !lamp.Active || lamp.ID == 0 {
		t.Fatalf("stored product: %+v", lamp)
	}

	// Edit: the form is prefilled, an unchecked toggle saves false, ID is not
	// writable.
	id := fmt.Sprint(lamp.ID)
	mustContain(t, h.get("/products/"+id+"/edit").body, `value="Desk lamp"`, `value="42"`)
	res = h.post("/products/"+id, url.Values{"Name": {"Floor lamp"}, "Price": {"0"}, "ID": {"1"}})
	if res.status != http.StatusSeeOther {
		t.Fatalf("update: %d", res.status)
	}
	var after Product
	db.First(&after, lamp.ID)
	if after.Name != "Floor lamp" || after.Price != 0 || after.Active || after.ID != lamp.ID {
		t.Fatalf("after update: %+v", after)
	}

	// Validation errors do not touch the database.
	res = h.post("/products/"+id, url.Values{"Name": {""}, "Price": {"x"}})
	if res.status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid update: %d", res.status)
	}
	db.First(&after, lamp.ID)
	if after.Name != "Floor lamp" {
		t.Fatalf("invalid update changed the record: %+v", after)
	}

	// Delete, then a stale link is a 404, and deleting the last row of the last
	// page still renders a valid page.
	if res = h.post("/products/"+id+"/delete", nil); res.status != http.StatusSeeOther {
		t.Fatalf("delete: %d", res.status)
	}
	if res = h.get("/products/" + id + "/edit"); res.status != http.StatusNotFound {
		t.Errorf("edit after delete: %d", res.status)
	}
	res = h.get("/products?page=99")
	mustContain(t, res.body, "page 4 of 4")

	// Sign out.
	if res = h.post("/logout", nil); res.status != http.StatusSeeOther {
		t.Fatalf("logout: %d", res.status)
	}
	if res = h.get("/products"); res.status != http.StatusSeeOther {
		t.Errorf("list after logout: %d", res.status)
	}
}
