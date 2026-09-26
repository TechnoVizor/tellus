# Tellus

An admin panel framework for Go, in the spirit of Laravel Filament. Describe a resource in Go and get a ready-to-use admin UI: list with search, sorting and pagination, create and edit forms, sign-in, light and dark themes.

Status: pre-alpha (milestone M0). The API will change.

Tellus is a library. It mounts into your existing Go application and runs in the same binary as your API. The UI is rendered on the server with [templ](https://templ.guide), enhanced with htmx and Alpine.js, and everything ships embedded, so there is no CDN and no frontend build in your project.

## Quick start

```go
db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})

// Built-in accounts (a tellus_users table). Bring your own by implementing
// tellus.Authenticator.
auth := tellus.NewGormAuthenticator(db)
_ = auth.Migrate()
_ = auth.CreateUser(ctx, "admin@example.com", "Admin", "a-long-password")

panel, err := tellus.New(tellus.Config{
	Authenticator: auth,
	SessionSecret: secret, // at least 32 random bytes
	Prefix:        "/admin", // any path, for example a secret one
	Name:          "Shop admin",
})
if err != nil {
	log.Fatal(err)
}

err = panel.Register(
	tellus.Resource[Product](db).
		Table(
			table.Text("Name").Searchable().Sortable(),
			table.Text("Price").Sortable(),
			table.Boolean("Active"),
		).
		Form(
			form.Text("Name").Required().MaxLength(100),
			form.Number("Price").Required(),
			form.Toggle("Active"),
		),
)
if err != nil { // a typo in a field name is reported here, at startup
	log.Fatal(err)
}

mux := http.NewServeMux()
panel.Mount(mux) // serves the panel at panel.Prefix()
log.Fatal(http.ListenAndServe(":8080", mux))
```

Sign-in cookies are `Secure` by default, so a browser drops them over plain http (only `localhost` is exempt). For local development on another host name, set `InsecureCookies: true`.

`Panel.Handler()` returns a plain `http.Handler` for routers other than `net/http`; mount it at `panel.Prefix() + "/"`.

Behind a reverse proxy, set `r.RemoteAddr` from your trusted forwarding header before requests reach the panel (for example with chi's `RealIP` middleware). The sign-in rate limit is keyed on it.

## What works in M0

- Resources on GORM and Postgres with an in-memory-testable `DataSource` interface.
- List with text search, sortable columns and server-side pagination, updated in place with htmx.
- Create, edit and delete with validation errors shown next to the fields.
- Field types: text, textarea, number, toggle. Columns: text, boolean.
- Sign-in with argon2id passwords, signed session cookies, CSRF protection, sign-in rate limiting.
- One fixed design with light and dark themes.

Planned next: relations, more field types, filters, actions and bulk actions, file uploads, policies. See `docs/spec.md`.

## Development

Requirements: Go 1.26 or newer, Docker.

```bash
docker compose up -d --wait
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test ./...
```

Integration tests need the Postgres from `docker-compose.yml` (published on host port 55432) and skip themselves when `TELLUS_TEST_DSN` is not set.

The `.templ` files are compiled to `*_templ.go`, and the generated files are committed. After editing a `.templ` file:

```bash
go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

Run the example shop admin at http://localhost:8091/admin (demo login `admin@example.com` / `demo-password-123`, local development only):

```bash
cd examples/shop
go run .
```

## License

Apache-2.0. Third-party components embedded in the assets are listed in `THIRD_PARTY_NOTICES.md`.
