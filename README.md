<div align="center">

# Tellus

### A Filament-style admin panel for Go

Describe a resource in Go. Get a list with search, sorting and pagination, create and edit forms, sign-in, and a light and dark theme, all in your own binary. No Node, no CDN, no separate frontend to deploy.

[![Go Reference](https://pkg.go.dev/badge/github.com/TechnoVizor/tellus.svg)](https://pkg.go.dev/github.com/TechnoVizor/tellus)
[![Go Report Card](https://goreportcard.com/badge/github.com/TechnoVizor/tellus)](https://goreportcard.com/report/github.com/TechnoVizor/tellus)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
![Status](https://img.shields.io/badge/status-pre--alpha-orange)

<img src="docs/assets/screenshot-light.jpg#gh-light-mode-only" width="820" alt="Tellus admin panel showing a product list, light theme">
<img src="docs/assets/screenshot-dark.jpg#gh-dark-mode-only" width="820" alt="Tellus admin panel showing a product list, dark theme">

</div>

## Why Tellus

Laravel has Filament. Go has nothing quite like it. The closest things are [GoAdmin](https://github.com/GoAdminGroup/go-admin), mature but dated in look and developer experience, and a couple of very young, unreleased attempts. There is room for a panel with a modern look, a small typed API, and no dependency on Node or a separate frontend build.

Tellus is a library, not a service. It mounts into your existing Go application and runs in the same binary as your API, behind whatever router you already use.

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

## Try it locally

The screenshot above is `examples/shop`, a small storefront admin with a seeded product catalog.

```bash
docker compose up -d --wait
cd examples/shop
go run .
```

Open http://localhost:8091/admin and sign in with `admin@example.com` / `demo-password-123`. Local development only; see [Development](#development) below.

## What works today

- Resources described with a typed builder: `Resource[T](db).Table(...).Form(...)`, validated at startup, not at the first request.
- List view: text search, sortable columns, server-side pagination, all updated in place with htmx, no full page reloads.
- Create, edit and delete, with validation errors shown next to the field that failed.
- Field types: text, textarea, number, toggle. Column types: text, boolean.
- Built-in accounts with argon2id password hashing, signed session cookies, CSRF protection on every state-changing request, and a sign-in rate limiter.
- A `DataSource` interface behind every resource; the built-in one is GORM on Postgres, and it is straightforward to swap in your own for testing.
- One deliberate, fixed design with a light and a dark theme, shipped as plain CSS with no build step.

## Roadmap

Tellus is at milestone M0: one resource, end to end, on real infrastructure. The full plan lives in [`docs/spec.md`](docs/spec.md). Next up, in order:

- **Phase 1:** relations (belongs-to, has-many, many-to-many), filters, more field and column types, actions and bulk actions, file uploads, custom pages and fields.
- **Phase 2:** notifications, global search, S3 storage, rich text and other field types, additional UI languages.
- **Phase 3:** dashboard widgets, multiple panels, a plugin system.

The public API will change before v1.0. Pin a commit if you depend on it today.

## How it compares

| | Tellus | GoAdmin | Filament (PHP, for reference) |
|---|---|---|---|
| Language | Go | Go | PHP |
| Rendering | Server-rendered templ + htmx | Server-rendered Go templates | Server-rendered Livewire |
| Frontend build | None, everything embedded | None | Node, Tailwind |
| Resource definition | Typed generic builder | Struct tags and config | PHP classes |
| Status | Pre-alpha | Mature, ~9k stars | Mature |

Filament is the inspiration, not a competitor: it is a Laravel package and this is a Go one. GoAdmin is the closest existing alternative in Go; the gap Tellus tries to close is developer experience and a modern default look.

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

## Contributing

Tellus is early and the API is still moving, so the most useful contribution right now is a pointed issue: a rough edge you hit following the quick start, a design decision you disagree with, a feature from the roadmap you need sooner than planned. Pull requests are welcome; for anything larger than a small fix, open an issue first so the direction is agreed before the work is.

If Tellus is useful to you, a star helps other Go developers find it.

## Acknowledgments

Filament, for showing what an admin panel's developer experience can look like. [htmx](https://htmx.org), [Alpine.js](https://alpinejs.dev) and [Inter](https://rsms.me/inter/) are embedded in the panel's assets; see [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) for their licenses.

## License

Apache-2.0. Third-party components embedded in the assets are listed in `THIRD_PARTY_NOTICES.md`.
