package tellus

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

func loggedIn(t *testing.T, src *memSource, mods ...func(*ResourceBuilder[Product])) *harness {
	t.Helper()
	res := productResource(src)
	for _, m := range mods {
		m(res)
	}
	h := newHarness(t, Config{}, res)
	h.loginAdmin()
	return h
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in:\n%s", w, body)
		}
	}
}

// --- registration ---

func TestRegisterValidation(t *testing.T) {
	src := newMemSource(0)
	nameCol := table.Text("Name")
	nameField := form.Text("Name")

	cases := map[string]Registrable{
		"no columns": Resource[Product](nil).Source(src).Form(nameField),
		"no fields":  Resource[Product](nil).Source(src).Table(nameCol),
		"unknown column": Resource[Product](nil).Source(src).
			Table(table.Text("Nope")).Form(nameField),
		"unknown form field": Resource[Product](nil).Source(src).
			Table(nameCol).Form(form.Text("Nope")),
		"unexported column": Resource[Product](nil).Source(src).
			Table(table.Text("name")).Form(nameField),
		"text field on int": Resource[Product](nil).Source(src).
			Table(nameCol).Form(form.Text("Price")),
		"number field on string": Resource[Product](nil).Source(src).
			Table(nameCol).Form(form.Number("Name")),
		"toggle field on string": Resource[Product](nil).Source(src).
			Table(nameCol).Form(form.Toggle("Name")),
		"duplicate form field": Resource[Product](nil).Source(src).
			Table(nameCol).Form(nameField, form.Text("Name")),
		"reserved slug": Resource[Product](nil).Source(src).Slug("login").
			Table(nameCol).Form(nameField),
		"invalid slug": Resource[Product](nil).Source(src).Slug("Bad Slug").
			Table(nameCol).Form(nameField),
		"not a struct":     Resource[string](nil),
		"no db, no source": Resource[Product](nil).Table(nameCol).Form(nameField),
	}
	for name, res := range cases {
		p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
		if err := p.Register(res); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestRegisterRejectsDuplicateSlug(t *testing.T) {
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err := p.Register(productResource(newMemSource(0))); err != nil {
		t.Fatal(err)
	}
	if err := p.Register(productResource(newMemSource(0))); err == nil {
		t.Error("second resource with the same slug accepted")
	}
	if err := p.Register(productResource(newMemSource(0)).Slug("other-products")); err != nil {
		t.Errorf("a different slug must be accepted: %v", err)
	}
}

func TestHomeRendersDashboard(t *testing.T) {
	h := loggedIn(t, newMemSource(3))
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "Dashboard", "Product", "3")
	if strings.Contains(res.body, "data-series") {
		t.Error("a resource on a custom DataSource has no *gorm.DB, so it must have no chart")
	}
}

func TestHomeWithNoResourcesShowsMessage(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "No resources are registered yet.")
}

func TestHomeCountErrorIsServerError(t *testing.T) {
	src := newMemSource(1)
	src.failWith = errBoom
	h := loggedIn(t, src)
	res := h.get("/")
	if res.status != http.StatusInternalServerError {
		t.Fatalf("status %d", res.status)
	}
	if strings.Contains(res.body, "boom") || strings.Contains(res.body, "secret") {
		t.Error("internal error text leaked to the client")
	}
}

func TestDashboardNavActiveOnlyOnDashboard(t *testing.T) {
	h := loggedIn(t, newMemSource(1))
	navOf := func(body string) string {
		start, end := strings.Index(body, "<nav"), strings.Index(body, "</nav>")
		if start < 0 || end < 0 {
			return ""
		}
		return body[start:end]
	}
	home := navOf(h.get("/").body)
	if !strings.Contains(home, `aria-current="page">Dashboard`) {
		t.Errorf("dashboard nav not active on home: %s", home)
	}
	list := navOf(h.get("/products").body)
	if strings.Contains(list, `aria-current="page">Dashboard`) {
		t.Errorf("dashboard nav must not be active on a resource page: %s", list)
	}
	if !strings.Contains(list, `aria-current="page">Products`) {
		t.Errorf("products nav not active on its own page: %s", list)
	}
}

// noteSource is an in-memory DataSource[Note], used only to register a
// second resource alongside Product for the dashboard's card-order test.
// Only List is exercised (through countFn); everything else panics, the
// same relStubSource convention relation_test.go already uses.
type noteSource struct{ items []Note }

func newNoteSource() *noteSource { return &noteSource{items: []Note{{ID: 1, Body: "x"}}} }

func (s *noteSource) List(context.Context, ListQuery) (ListResult[Note], error) {
	return ListResult[Note]{Items: s.items, Total: int64(len(s.items))}, nil
}
func (s *noteSource) Find(context.Context, string) (*Note, error) { panic("unused") }
func (s *noteSource) Create(context.Context, *Note) error         { panic("unused") }
func (s *noteSource) Update(context.Context, *Note) error         { panic("unused") }
func (s *noteSource) Delete(context.Context, string) error        { panic("unused") }
func (s *noteSource) ID(*Note) string                             { panic("unused") }

func TestDashboardCardOrderMatchesRegistration(t *testing.T) {
	cat := Resource[Note](nil).Source(newNoteSource()).
		Table(table.Text("Body")).
		Form(form.Text("Body").Required())
	prod := productResource(newMemSource(1))
	h := newHarness(t, Config{}, cat, prod)
	h.loginAdmin()
	body := h.get("/").body
	notesIdx, productsIdx := strings.Index(body, "Notes"), strings.Index(body, "Products")
	if notesIdx < 0 || productsIdx < 0 || notesIdx > productsIdx {
		t.Errorf("cards must appear in registration order (Notes then Products): %s", body)
	}
}

func TestDashboardCountsAreFreshPerRequest(t *testing.T) {
	src := newMemSource(1)
	h := loggedIn(t, src)
	first := h.get("/").body
	if !strings.Contains(first, ">1<") {
		t.Fatalf("expected a count of 1: %s", first)
	}
	src.items = append(src.items, Product{ID: 99, Name: "New"})
	second := h.get("/").body
	if !strings.Contains(second, ">2<") {
		t.Errorf("count did not refresh after the source changed: %s", second)
	}
}

// --- list ---

func TestListRendersRowsAndChrome(t *testing.T) {
	h := loggedIn(t, newMemSource(3))
	res := h.get("/products")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body,
		"<h1>Products</h1>", "Product 01", "Product 03",
		`href="/admin/products/new"`,
		`href="/admin/products/1/edit"`,
		`action="/admin/products/1/delete"`,
		`aria-current="page"`,
		"3 records, page 1 of 1",
		"badge-on", "badge-off",
		`name="q"`, "sort=Name", "sort=Price",
		"Ada", // signed-in user in the sidebar
	)
	if strings.Contains(res.body, "sort=Active") {
		t.Error("a column that is not sortable must not get a sort link")
	}
}

func TestListEmptyState(t *testing.T) {
	h := loggedIn(t, newMemSource(0))
	res := h.get("/products")
	mustContain(t, res.body, "Nothing here yet.", "0 records, page 1 of 1")
}

func TestListPagination(t *testing.T) {
	h := loggedIn(t, newMemSource(30), func(b *ResourceBuilder[Product]) { b.PerPage(10) })

	res := h.get("/products?page=2&q=pro&sort=Price&dir=desc")
	mustContain(t, res.body, "30 records, page 2 of 3", "Product 11", "page=3")
	if strings.Contains(res.body, "Product 10<") || strings.Contains(res.body, "Product 21") {
		t.Error("page 2 shows rows from other pages")
	}
	// Previous and next keep the search and sort.
	mustContain(t, res.body, "q=pro", "sort=Price", "dir=desc")

	first := h.get("/products?page=2")
	if !strings.Contains(first.body, `href="/admin/products"`) {
		t.Error("the previous link to page 1 should drop the page parameter")
	}
}

func TestListPassesOnlyWhitelistedParameters(t *testing.T) {
	src := newMemSource(60)
	h := loggedIn(t, src)

	h.get("/products?q=%20lamp%20&sort=Name&dir=desc&page=2")
	q := src.last()
	if q.Search != "lamp" || q.SortField != "Name" || !q.SortDesc || q.Page != 2 || q.PerPage != defaultPerPage {
		t.Errorf("valid params not passed through: %+v", q)
	}
	if !reflect.DeepEqual(q.SearchFields, []string{"Name"}) {
		t.Errorf("search fields must be the declared searchable columns: %v", q.SearchFields)
	}

	for _, target := range []string{
		"/products?sort=Active",                 // declared column, but not sortable
		"/products?sort=ID",                     // real field, not declared
		"/products?sort=Name%3B%20DROP%20TABLE", // hostile
		"/products?sort=name",                   // wrong case
	} {
		h.get(target)
		if got := src.last().SortField; got != "" {
			t.Errorf("%s: sort field %q reached the data source", target, got)
		}
	}

	h.get("/products?sort=Name&dir=sideways")
	if q := src.last(); q.SortField != "Name" || q.SortDesc {
		t.Errorf("unknown direction should mean ascending: %+v", q)
	}

	for _, target := range []string{"/products?page=abc", "/products?page=-4", "/products?page=0", "/products?page=99999999999999999999999"} {
		h.get(target)
		if got := src.last().Page; got != 1 {
			t.Errorf("%s: page %d", target, got)
		}
	}

	long := strings.Repeat("x", 500)
	h.get("/products?q=" + long)
	if got := len([]rune(src.last().Search)); got != maxSearchRunes {
		t.Errorf("search term was not capped: %d runes", got)
	}
}

func TestListClampsPagePastTheEnd(t *testing.T) {
	src := newMemSource(25)
	h := loggedIn(t, src, func(b *ResourceBuilder[Product]) { b.PerPage(10) })

	res := h.get("/products?page=50")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "25 records, page 3 of 3", "Product 21")
	if n := len(src.queries); n != 2 || src.last().Page != 3 {
		t.Errorf("expected a second query for the last page, got %d queries, last page %d", n, src.last().Page)
	}

	src.queries = nil
	h.get("/products?page=5000000")
	if src.queries[0].Page != maxPage {
		t.Errorf("huge page must be capped at %d, got %d", maxPage, src.queries[0].Page)
	}
}

func TestListHTMXReturnsOnlyTheRegion(t *testing.T) {
	h := loggedIn(t, newMemSource(3))

	partial := h.get("/products?q=pro", "HX-Request", "true")
	mustContain(t, partial.body, `id="records"`, "Product 01")
	if strings.Contains(partial.body, "<html") || strings.Contains(partial.body, "sidebar") {
		t.Error("htmx request must not get the whole page")
	}
	if !strings.Contains(partial.header.Get("Vary"), "HX-Request") {
		t.Errorf("Vary must include HX-Request, got %q", partial.header.Get("Vary"))
	}

	restore := h.get("/products", "HX-Request", "true", "HX-History-Restore-Request", "true")
	mustContain(t, restore.body, "<html", "sidebar")

	full := h.get("/products")
	mustContain(t, full.body, "<html", `id="records"`)
}

func TestNoticeIsWhitelisted(t *testing.T) {
	h := loggedIn(t, newMemSource(1))
	mustContain(t, h.get("/products?notice=created").body, "Record created.")
	mustContain(t, h.get("/products?notice=deleted").body, "Record deleted.")

	res := h.get("/products?notice=%3Cscript%3Ealert(1)%3C/script%3E")
	if strings.Contains(res.body, "alert(1)") || strings.Contains(res.body, `class="notice"`) {
		t.Error("an unknown notice value must not be shown")
	}
}

// --- create ---

func TestNewFormRendersFields(t *testing.T) {
	h := loggedIn(t, newMemSource(0))
	res := h.get("/products/new")
	mustContain(t, res.body,
		"<h1>New Product</h1>",
		`action="/admin/products"`,
		`name="Name"`, "maxlength=\"100\"",
		`name="Price"`, `type="number"`,
		`type="checkbox" name="Active"`,
		`name="_csrf"`,
		`href="/admin/products"`, // cancel
	)
	if !strings.Contains(res.body, h.csrfToken()) {
		t.Error("the form must carry the session's CSRF token")
	}
}

func TestCreateStoresRecordAndRedirects(t *testing.T) {
	src := newMemSource(0)
	h := loggedIn(t, src)

	res := h.post("/products", url.Values{"Name": {"Lamp"}, "Price": {"30"}, "Active": {"true"}})
	if res.status != http.StatusSeeOther || res.location() != "/admin/products?notice=created" {
		t.Fatalf("create: %d %q", res.status, res.location())
	}
	if len(src.items) != 1 || src.items[0].Name != "Lamp" || src.items[0].Price != 30 || !src.items[0].Active {
		t.Fatalf("stored: %+v", src.items)
	}
	mustContain(t, h.get("/products?notice=created").body, "Record created.", "Lamp")
}

func TestCreateValidationErrorsKeepInputAndStoreNothing(t *testing.T) {
	src := newMemSource(0)
	h := loggedIn(t, src)

	res := h.post("/products", url.Values{"Name": {"   "}, "Price": {"abc"}})
	if res.status != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body,
		"This field is required.", "Enter a valid number.", "Some fields need attention.",
		`value="abc"`, `aria-invalid="true"`)
	if len(src.items) != 0 {
		t.Fatalf("invalid input was stored: %+v", src.items)
	}

	res = h.post("/products", url.Values{"Name": {strings.Repeat("n", 101)}, "Price": {"1"}})
	if res.status != http.StatusUnprocessableEntity || !strings.Contains(res.body, "Must be at most 100 characters.") {
		t.Fatalf("over-long name: %d", res.status)
	}
}

func TestUserInputIsEscapedEverywhere(t *testing.T) {
	src := newMemSource(0)
	h := loggedIn(t, src)
	evil := `<img src=x onerror=alert(1)>`

	h.post("/products", url.Values{"Name": {evil}, "Price": {"1"}})
	if len(src.items) != 1 || src.items[0].Name != evil {
		t.Fatalf("value should be stored verbatim: %+v", src.items)
	}
	for _, page := range []string{"/products", "/products/1/edit"} {
		body := h.get(page).body
		if strings.Contains(body, "<img src=x") {
			t.Errorf("%s: unescaped user input in the page", page)
		}
		if !strings.Contains(body, "&lt;img") && !strings.Contains(body, "&#34;") && !strings.Contains(body, "&lt;") {
			t.Errorf("%s: expected the escaped form of the input", page)
		}
	}
	// A failed validation re-renders the submitted text.
	res := h.post("/products", url.Values{"Name": {evil}, "Price": {"nope"}})
	if strings.Contains(res.body, "<img src=x") {
		t.Error("re-rendered form reflected the input unescaped")
	}
}

func TestCreateIgnoresUndeclaredFields(t *testing.T) {
	src := newMemSource(2)
	h := loggedIn(t, src)
	h.post("/products", url.Values{"Name": {"Hack"}, "Price": {"1"}, "ID": {"999"}, "CreatedAt": {"2000-01-01T00:00:00Z"}})
	got := src.items[len(src.items)-1]
	if got.ID == 999 || !got.CreatedAt.IsZero() {
		t.Fatalf("undeclared fields were mass-assigned: %+v", got)
	}
}

// --- edit and update ---

func TestEditFormIsPrefilled(t *testing.T) {
	h := loggedIn(t, newMemSource(2))

	active := h.get("/products/2/edit")
	mustContain(t, active.body, "<h1>Edit Product</h1>", `action="/admin/products/2"`, `value="Product 02"`, `value="20"`)
	if !strings.Contains(active.body, `checked`) {
		t.Error("an active product must show a checked toggle")
	}
	inactive := h.get("/products/1/edit")
	if strings.Contains(inactive.body, "checked") {
		t.Error("an inactive product must show an unchecked toggle")
	}
}

func TestUpdateSavesAndUncheckedToggleMeansFalse(t *testing.T) {
	src := newMemSource(2)
	h := loggedIn(t, src)

	// Active is absent from the form, as browsers do for an unchecked box. The
	// posted ID must not change the record's identity.
	res := h.post("/products/2", url.Values{"Name": {"Renamed"}, "Price": {"5"}, "ID": {"999"}})
	if res.status != http.StatusSeeOther || res.location() != "/admin/products?notice=updated" {
		t.Fatalf("update: %d %q", res.status, res.location())
	}
	got := src.items[1]
	if got.ID != 2 || got.Name != "Renamed" || got.Price != 5 || got.Active {
		t.Fatalf("stored: %+v", got)
	}
	if src.items[0].Name != "Product 01" {
		t.Error("another record changed")
	}
}

func TestUpdateValidationErrorLeavesRecordUntouched(t *testing.T) {
	src := newMemSource(2)
	h := loggedIn(t, src)
	res := h.post("/products/2", url.Values{"Name": {""}, "Price": {"7"}})
	if res.status != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", res.status)
	}
	mustContain(t, res.body, "<h1>Edit Product</h1>", "This field is required.", `action="/admin/products/2"`)
	if src.items[1].Name != "Product 02" || src.items[1].Price != 20 {
		t.Fatalf("record changed despite the error: %+v", src.items[1])
	}
}

func TestMissingRecordsAre404(t *testing.T) {
	src := newMemSource(1)
	h := loggedIn(t, src)
	for _, id := range []string{"999", "abc", "0", "-1"} {
		if res := h.get("/products/" + id + "/edit"); res.status != http.StatusNotFound || !strings.Contains(res.body, "Not found.") {
			t.Errorf("GET edit %s: %d", id, res.status)
		}
		if res := h.post("/products/"+id, url.Values{"Name": {"x"}, "Price": {"1"}}); res.status != http.StatusNotFound {
			t.Errorf("POST update %s: %d", id, res.status)
		}
	}
	if len(src.items) != 1 {
		t.Error("data changed")
	}
}

func TestUpdateOfRecordDeletedMeanwhileIs404(t *testing.T) {
	src := newMemSource(1)
	src.updateErr = ErrNotFound
	h := loggedIn(t, src)
	if res := h.post("/products/1", url.Values{"Name": {"x"}, "Price": {"1"}}); res.status != http.StatusNotFound {
		t.Fatalf("status %d", res.status)
	}
}

// --- delete ---

func TestDelete(t *testing.T) {
	src := newMemSource(3)
	h := loggedIn(t, src)

	// Needs a CSRF token.
	if res := h.postRaw("/products/1/delete", url.Values{}); res.status != http.StatusForbidden || len(src.items) != 3 {
		t.Fatalf("delete without CSRF token: %d, %d items", res.status, len(src.items))
	}
	// A link click (GET) must never delete.
	if res := h.get("/products/1/delete"); res.status != http.StatusMethodNotAllowed && res.status != http.StatusNotFound || len(src.items) != 3 {
		t.Fatalf("GET on the delete URL: %d, %d items", res.status, len(src.items))
	}

	res := h.post("/products/1/delete", nil)
	if res.status != http.StatusSeeOther || res.location() != "/admin/products?notice=deleted" {
		t.Fatalf("delete: %d %q", res.status, res.location())
	}
	if len(src.items) != 2 || src.items[0].ID != 2 {
		t.Fatalf("items after delete: %+v", src.items)
	}
	// Deleting again is harmless.
	if res := h.post("/products/1/delete", nil); res.status != http.StatusSeeOther {
		t.Errorf("repeat delete: %d", res.status)
	}
}

// --- failures and access ---

func TestSourceErrorsAreHiddenFrom500Pages(t *testing.T) {
	src := newMemSource(1)
	src.failWith = errBoom
	h := loggedIn(t, src)

	for name, res := range map[string]result{
		"list":   h.get("/products"),
		"edit":   h.get("/products/1/edit"),
		"create": h.post("/products", url.Values{"Name": {"x"}, "Price": {"1"}}),
		"delete": h.post("/products/1/delete", nil),
	} {
		if res.status != http.StatusInternalServerError {
			t.Errorf("%s: status %d", name, res.status)
		}
		if strings.Contains(res.body, "boom") || strings.Contains(res.body, "secret") {
			t.Errorf("%s: internal error text leaked to the client", name)
		}
	}
}

func TestResourceRoutesRequireLogin(t *testing.T) {
	src := newMemSource(2)
	h := newHarness(t, Config{}, productResource(src)) // not signed in

	gets := []string{"/products", "/products/new", "/products/1/edit"}
	for _, path := range gets {
		if res := h.get(path); res.status != http.StatusSeeOther || res.location() != "/admin/login" {
			t.Errorf("GET %s: %d %q", path, res.status, res.location())
		}
	}
	posts := []string{"/products", "/products/1", "/products/1/delete"}
	for _, path := range posts {
		res := h.post(path, url.Values{"Name": {"x"}, "Price": {"1"}})
		if res.status != http.StatusSeeOther || res.location() != "/admin/login" {
			t.Errorf("POST %s: %d %q", path, res.status, res.location())
		}
	}
	if len(src.items) != 2 || src.items[0].Name != "Product 01" {
		t.Fatalf("anonymous requests changed data: %+v", src.items)
	}
}

// badField returns a value of the wrong type from Parse.
type badField struct{}

func (badField) Info() form.Info                          { return form.Info{Name: "Price", Label: "Price"} }
func (badField) Check(reflect.Type) error                 { return nil }
func (badField) Format(any) string                        { return "" }
func (badField) Parse(string, reflect.Type) (any, string) { return "not an int", "" }
func (badField) Render(form.Value) templ.Component {
	return templ.ComponentFunc(func(context.Context, io.Writer) error { return nil })
}

func TestCustomFieldReturningWrongTypeIsAnErrorNotAPanic(t *testing.T) {
	src := newMemSource(0)
	res := productResource(src).Form(form.Text("Name"), badField{})
	h := newHarness(t, Config{}, res)
	h.loginAdmin()
	if r := h.post("/products", url.Values{"Name": {"x"}}); r.status != http.StatusInternalServerError {
		t.Fatalf("status %d", r.status)
	}
	if len(src.items) != 0 {
		t.Error("record stored despite the bad field")
	}
}

func TestCustomLabelsAndSlug(t *testing.T) {
	h := loggedIn(t, newMemSource(1), func(b *ResourceBuilder[Product]) {
		b.Slug("goods").Label("Good", "Goods")
	})
	res := h.get("/goods")
	mustContain(t, res.body, "<h1>Goods</h1>", `href="/admin/goods/new"`)
	mustContain(t, h.get("/goods/new").body, "<h1>New Good</h1>")
	if r := h.get("/products"); r.status != http.StatusNotFound {
		t.Errorf("old slug still answers: %d", r.status)
	}
}
