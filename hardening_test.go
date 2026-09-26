package tellus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/testdb"
	"github.com/TechnoVizor/tellus/table"
)

// Tests added after the whole-branch review.

func TestListDropsSearchTextPostgresCannotStore(t *testing.T) {
	src := newMemSource(3)
	h := loggedIn(t, src)
	for _, target := range []string{"/products?q=%00", "/products?q=%FF", "/products?q=a%00b", "/products?q=%C3"} {
		if res := h.get(target); res.status != http.StatusOK {
			t.Errorf("%s: status %d", target, res.status)
		}
		if q := src.last().Search; !utf8.ValidString(q) || strings.ContainsRune(q, 0) {
			t.Errorf("%s: search text %q would break a Postgres query", target, q)
		}
	}
}

func TestHostileTextNeverBreaksTheRealDatabase(t *testing.T) {
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
	db.Create(&Product{Name: "Lamp", Price: 1})

	products := Resource[Product](db).
		Table(table.Text("Name").Searchable().Sortable(), table.Text("Price").Sortable(), table.Boolean("Active")).
		Form(form.Text("Name").Required().MaxLength(100), form.Number("Price").Required(), form.Toggle("Active"))
	h := newHarness(t, Config{Authenticator: auth}, products)

	// Sign-in with an email Postgres cannot store is an ordinary failed sign-in.
	for _, email := range []string{"a\x00b@x.co", "\xff@x.co"} {
		if res := h.login(email, "whatever"); res.status != http.StatusUnauthorized {
			t.Errorf("sign-in with %q: status %d, want 401", email, res.status)
		}
	}
	h.loginAdmin()

	for _, q := range []string{"%00", "%FF", "a%00b", "%C3"} {
		if res := h.get("/products?q=" + q); res.status != http.StatusOK {
			t.Errorf("search %s: status %d, want 200", q, res.status)
		}
	}

	for _, name := range []string{"a\x00b", "a\xffb"} {
		res := h.post("/products", url.Values{"Name": {name}, "Price": {"5"}})
		if res.status != http.StatusUnprocessableEntity || !strings.Contains(res.body, "not allowed") {
			t.Errorf("create with %q: status %d, want a 422 validation message", name, res.status)
		}
	}
	var n int64
	db.Model(&Product{}).Count(&n)
	if n != 1 {
		t.Errorf("hostile input created rows: %d products", n)
	}

	for _, id := range []string{"a%00b", "%FF"} {
		if res := h.get("/products/" + id + "/edit"); res.status != http.StatusNotFound {
			t.Errorf("edit %s: status %d, want 404", id, res.status)
		}
	}
}

func TestPostedIDNeverReachesTheDatabase(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatal(err)
	}
	products := Resource[Product](db).
		Table(table.Text("Name")).
		Form(form.Text("Name").Required(), form.Number("Price"))
	h := newHarness(t, Config{}, products)
	h.loginAdmin()

	res := h.post("/products", url.Values{"Name": {"Fresh"}, "Price": {"5"}, "ID": {"999"}})
	if res.status != http.StatusSeeOther {
		t.Fatalf("create: %d", res.status)
	}
	var p Product
	if err := db.Where("name = ?", "Fresh").First(&p).Error; err != nil {
		t.Fatal(err)
	}
	if p.ID == 999 {
		t.Fatal("a posted ID was written to the database")
	}
}

func TestClientKeyGroupsIPv6By64(t *testing.T) {
	key := func(addr string) string { return clientIP(&http.Request{RemoteAddr: addr}) }

	if got := key("203.0.113.7:5555"); got != "203.0.113.7" {
		t.Errorf("IPv4 key: %q", got)
	}
	a := key("[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:80")
	b := key("[2001:db8:1:2:1:2:3:4]:81")
	c := key("[2001:db8:1:3::1]:82")
	if a != b {
		t.Errorf("one /64 must share a key, got %q and %q", a, b)
	}
	if a == c {
		t.Error("different /64 networks must not share a key")
	}
	if got := key("[::ffff:203.0.113.7]:9"); got != "203.0.113.7" {
		t.Errorf("IPv4-mapped address: %q", got)
	}
	if got := key("garbage"); got != "garbage" {
		t.Errorf("unparsable address should be used as is, got %q", got)
	}
}

func TestSuccessfulSignInResetsTheAttemptCounter(t *testing.T) {
	h := newHarness(t, Config{})
	for i := 0; i < loginAttempts-1; i++ {
		h.login("admin@example.com", "wrong")
	}
	h.loginAdmin()
	h.post("/logout", nil)

	// Without the reset, these would run into the limit: 9 + 1 + 9 attempts.
	for i := 0; i < loginAttempts-1; i++ {
		if res := h.login("admin@example.com", "wrong"); res.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d after a successful sign-in: status %d", i+1, res.status)
		}
	}
}

func TestPrefixEdgeRedirectsStayInsideThePrefix(t *testing.T) {
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	h := p.Handler()

	cases := map[string]string{
		"/admin":                   "/admin/",
		"/admin//login":            "/admin/login",
		"/admin/products/../login": "/admin/login",
	}
	for target, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code < 300 || rec.Code >= 400 || rec.Header().Get("Location") != want {
			t.Errorf("%s: status %d, Location %q, want a redirect to %q", target, rec.Code, rec.Header().Get("Location"), want)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/login", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("a clean path must not redirect: %d", rec.Code)
	}
}

func TestErrorPagesUseTheCatalog(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.postRaw("/login", url.Values{})
	if res.status != http.StatusForbidden || !strings.Contains(res.body, i18n.T("error.csrf")) {
		t.Errorf("CSRF failure: %d %q", res.status, res.body)
	}

	src := newMemSource(1)
	src.failWith = errBoom
	h = loggedIn(t, src)
	res = h.get("/products")
	if res.status != http.StatusInternalServerError || !strings.Contains(res.body, i18n.T("error.internal")) {
		t.Errorf("500 page: %d %q", res.status, res.body)
	}
}
