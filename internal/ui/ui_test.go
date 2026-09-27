package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/assets"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in:\n%s", w, body)
		}
	}
}

func sampleShell() Shell {
	return Shell{
		Title:    "Products",
		Brand:    "Acme",
		Prefix:   "/admin",
		Nav:      []NavItem{{Label: "Products", Href: "/admin/products", Active: true}, {Label: "Orders", Href: "/admin/orders"}},
		UserName: "Ada",
		CSRF:     "tok123",
	}
}

func TestLayoutHasNavigationAssetsAndLogout(t *testing.T) {
	out := render(t, MessagePage(sampleShell(), "Hello", "A message"))
	v := assets.Version()
	mustContain(t, out,
		"<title>Products | Acme</title>",
		"<h1>Hello</h1>", "A message",
		`href="/admin/products"`, `href="/admin/orders"`,
		`action="/admin/logout"`, `name="_csrf" value="tok123"`,
		"Ada",
		`<script src="/admin/assets/theme-init.js?v=`+v+`"></script>`,
		`href="/admin/assets/app.css?v=`+v+`"`,
		`src="/admin/assets/htmx.min.js?v=`+v+`" defer`,
		`src="/admin/assets/alpine.min.js?v=`+v+`" defer`,
		`x-data="themeToggle"`,
		`content="noindex, nofollow"`,
	)
	if n := strings.Count(out, `aria-current="page"`); n != 1 {
		t.Errorf("exactly one nav item must be current, got %d", n)
	}
}

func TestLoginPageEscapesAndShowsError(t *testing.T) {
	out := render(t, LoginPage(LoginView{
		Brand: "Acme", Prefix: "/admin", Action: "/admin/login",
		Email: `"><script>x</script>`, Error: "Invalid email or password.", CSRF: "tok123",
	}))
	if strings.Contains(out, "<script>x</script>") {
		t.Fatal("email reflected unescaped")
	}
	mustContain(t, out, `role="alert"`, "Invalid email or password.", `action="/admin/login"`, `name="_csrf" value="tok123"`, `autocomplete="current-password"`)

	clean := render(t, LoginPage(LoginView{Brand: "Acme", Prefix: "/admin", Action: "/admin/login"}))
	if strings.Contains(clean, `role="alert"`) {
		t.Error("no error message expected")
	}
}

func sampleList() ListView {
	return ListView{
		Shell:      sampleShell(),
		Heading:    "Products",
		NewURL:     "/admin/products/new",
		ListURL:    "/admin/products",
		Searchable: true,
		Sort:       "Name", Dir: "asc",
		Columns: []ColumnView{
			{Label: "Name", Sortable: true, SortURL: "/admin/products?dir=desc&sort=Name", SortMark: "▲", AriaSort: "ascending"},
			{Label: "Price", Sortable: true, SortURL: "/admin/products?dir=asc&sort=Price"},
			{Label: "Active"},
		},
		Rows: []RowView{{
			Cells:     []templ.Component{templ.Raw("Lamp"), templ.Raw("30"), templ.Raw("Yes")},
			EditURL:   "/admin/products/7/edit",
			DeleteURL: "/admin/products/7/delete",
		}},
		Total: 31, Page: 2, TotalPages: 4,
		PrevURL: "/admin/products", NextURL: "/admin/products?page=3",
	}
}

func TestRecordsRegion(t *testing.T) {
	out := render(t, Records(sampleList()))
	mustContain(t, out,
		`id="records"`,
		// The search form lives outside the region, so these hidden inputs are tied
		// to it with the form attribute and refresh with every swap.
		`name="sort" value="Name"`, `name="dir" value="asc"`, `form="list-search"`,
		`aria-sort="ascending"`,
		"Lamp",
		`href="/admin/products/7/edit"`,
		`action="/admin/products/7/delete"`, `data-confirm="`, `x-data="confirmSubmit"`, `name="_csrf" value="tok123"`,
		"31 records, page 2 of 4",
		`hx-get="/admin/products"`, `hx-get="/admin/products?page=3"`,
	)
	if n := strings.Count(out, "aria-sort="); n != 1 {
		t.Errorf("only the active sort column may carry aria-sort, got %d", n)
	}
	if strings.Contains(out, "<html") {
		t.Error("the region must not include the page frame")
	}
	if strings.Contains(out, `role="search"`) || strings.Contains(out, `name="q"`) {
		t.Error("the search form must not be inside the swapped region: htmx would replace the focused input on every keystroke pause")
	}
}

func TestListPageKeepsSearchOutsideTheSwappedRegion(t *testing.T) {
	out := render(t, ListPage(sampleList()))
	search := strings.Index(out, `role="search"`)
	records := strings.Index(out, `id="records"`)
	if search < 0 || records < 0 || search > records {
		t.Fatalf("the search form must come before the #records region (search at %d, records at %d)", search, records)
	}
	if strings.Contains(out[records:], `role="search"`) {
		t.Error("a second search form leaked into the region")
	}
	mustContain(t, out,
		`id="list-search"`, `id="search-q"`, `name="q"`,
		`hx-target="#records"`, `hx-push-url="true"`, `hx-swap="outerHTML"`)
}

func TestLayoutTellsHtmxToReloadOnHistoryMiss(t *testing.T) {
	out := render(t, MessagePage(sampleShell(), "Hello", "A message"))
	mustContain(t, out, `name="htmx-config"`, "refreshOnHistoryMiss")
}

func TestLoginUsesTheSoftCard(t *testing.T) {
	out := render(t, LoginPage(LoginView{Brand: "Acme", Prefix: "/admin", Action: "/admin/login"}))
	mustContain(t, out, `class="card-soft"`)
}

func TestRecordsWithoutSearchEmptyAndFirstPage(t *testing.T) {
	v := sampleList()
	v.Searchable = false
	v.Rows = nil
	v.PrevURL = ""
	v.Page = 1
	out := render(t, Records(v))
	if strings.Contains(out, `role="search"`) {
		t.Error("no search box expected when nothing is searchable")
	}
	mustContain(t, out, "Nothing here yet.", `aria-disabled="true"`)
	if strings.Contains(out, "<table") {
		t.Error("an empty list should show the empty state, not a table")
	}
}

func TestListPageShowsNoticeAndNewButton(t *testing.T) {
	v := sampleList()
	v.Notice = "Record created."
	out := render(t, ListPage(v))
	mustContain(t, out, "<html", `role="status"`, "Record created.", `href="/admin/products/new"`, `id="records"`)
}

func TestFormPage(t *testing.T) {
	out := render(t, FormPage(FormView{
		Shell:     sampleShell(),
		Heading:   "New Product",
		Action:    "/admin/products",
		CancelURL: "/admin/products",
		Error:     "Some fields need attention.",
		Fields:    []templ.Component{templ.Raw(`<input name="Name">`)},
	}))
	mustContain(t, out,
		"<h1>New Product</h1>", `role="alert"`, "Some fields need attention.",
		`action="/admin/products"`, `name="_csrf" value="tok123"`, `<input name="Name">`,
		"Save", "Cancel", `novalidate`, `enctype="multipart/form-data"`)
}
