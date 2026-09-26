# Tellus: design spec

Status: draft v0, 2026-09-26. Source: design interview (grill session) with the project owner.
Language: English (the repository is public, English docs come first).

## 1. Vision

Tellus is an admin panel framework for Go, in the spirit of Laravel Filament. A Go developer with an existing API and a Postgres database describes resources in Go and gets a modern, ready-to-use admin UI: lists, forms, relations, actions, auth and permissions.

Success looks like this: the project is used by its author on small real sites (the public demo site first), it is open source, and any Go developer can take it and use it the way a Laravel developer uses Filament.

## 2. Audience and non-goals

Primary audience: a Go developer who already has, or is building, a Go API on Postgres and wants an admin panel in about a day.

Non-goals for the first phases:
- Non-developer site builders, no-code editing of the schema.
- A standalone binary that introspects any database.
- A separate SPA admin with its own frontend build.
- Multi-tenancy.

## 3. Landscape (checked 2026-09-26)

- GoAdmin (GoAdminGroup/go-admin): about 9k stars, Apache-2.0, English and Chinese docs, 222 open issues. Mature, dated UI, Chinese origin.
- QOR Admin (qor/admin): about 900 stars, old.
- SublimeGo (bozz33/SublimeGo): Filament-inspired, Ent + templ + HTMX + Tailwind, standalone starter, 7 stars, 25 commits, no tagged release, license not stated.

The gap: no modern, well-documented, library-style panel with a Filament-grade developer experience. The place is held by user count, not by quality, so the edge has to be developer experience and visual quality.

## 4. Architecture

### 4.1 Integration model

Tellus is a library that is mounted into the host application and runs in the same binary as the host's API. It exposes an `http.Handler`, so it works with `net/http`, chi, gin, echo and others through their standard adapters.

- The mount prefix is configurable, default `/admin`. Developers can move it to a secret path. Every generated link, redirect and asset URL is built from the prefix. A hidden prefix reduces bot noise but is not a security control; login and CSRF protection do not depend on it.
- One binary, one deploy, direct access to the host's own types and business logic.

### 4.2 Frontend

- Server-rendered HTML with templ. Interactivity from htmx and Alpine.js.
- All JS and CSS are embedded with `embed`. No CDN, no Node in the user's build.
- Styling is modern CSS: custom properties and `@layer`, semantic component classes (`.btn`, `.card`, and so on). Tailwind is not part of the shipped artifact, so users need no CSS build step for their own components. Tailwind is still allowed in the demo site.
- One fixed, original design with a light theme and a dark theme. Colors and tokens are not user-configurable. The visual language follows the owner's Farhaven and mybee.lv sites: monochrome ink on white, hairline borders, Inter, large radii and pill buttons.
- Branding is limited to panel name, logo and favicon.
- No third-party UI kit in the public API (shadcn ports were evaluated and rejected: 0.x dependency in the public surface, and full control over htmx swaps and Alpine re-initialization is needed).

### 4.3 Internationalization

The UI is English only at first. All UI strings go through `t("key")` with a single English catalog, so adding a language later is cheap.

## 5. Data layer

- A `DataSource` interface is part of the core from day one.
- First adapter: GORM on Postgres. MySQL and SQLite come nearly free through GORM.
- Ent and sqlc adapters come later as separate modules.
- Models use a single-column primary key of any comparable type (uint, uuid).
- Lists are paginated on the server.

## 6. Resource API

Resources are described with generic builder chains. This is the only way to describe a resource in phase 1 (a struct-tag shortcut is deferred).

Illustrative sketch, not final. Package and identifier names are decided at implementation time.

```go
panel.Resource[Product](db).
    Table(
        table.Text("Name").Searchable().Sortable(),
        table.Money("Price"),
        table.Badge("Status"),
    ).
    Form(
        form.Text("Name").Required().MaxLength(100),
        form.Select("CategoryID").Relation("Category"),
        form.Image("Photo"),
    )
```

- Field names are strings and are validated at startup: the panel fails fast if a field does not exist on the model.
- Validation rules live in the builder (`Required()`, `MaxLength(n)`, and so on). No external validation library in the core.

## 7. Authentication and authorization

Modeled on Filament.

- Built-in pages: login, logout, password reset, profile.
- The user comes from the host application: the host's user type implements `panel.User` (including `CanAccessPanel()`).
- An `Authenticator` interface, required in the config. `NewGormAuthenticator` is the ready-made built-in implementation (email and argon2id password in a `tellus_users` table, created explicitly with `Migrate`, so the panel never runs DDL on its own). A host with its own auth (for example JWT from its API) plugs in its own.
- Permissions are per-resource policies (`CanView`, `CanEdit`, and so on) that receive the user and, where relevant, the record.
- Two-factor auth comes later.

## 8. Phase 1 scope

Form fields: Text, Textarea, Number, Money, Toggle, Select, Radio, Date, DateTime, Image/File.

Table columns: Text, Badge, Boolean, Image, Date, Money.

Filters: text, select, boolean, date range, relation.

Tables: server-side pagination, sorting, search.

Relations:
- belongsTo, as a select field.
- hasMany, as a read-only list on the record page.
- manyToMany, as a multi-select.

Files: a `Storage` interface with a local disk implementation.

Actions: built-in create, edit, delete and bulk delete; custom actions on a record and on a table.

Extension points:
- Custom pages: `panel.Page("/reports", handler)`, rendered inside the panel layout.
- Custom actions.
- Custom field types through a `Field` interface. Note: this makes the `Field` interface public earlier than ideal. Accepted risk; expect it to change during `v0.x`.

Theming: light and dark, one fixed design.

## 9. Later phases

Phase 2: notifications, global search, S3 storage, rich text, markdown, JSON and color fields, inline editing of relations, struct-tag "zero config" registration, additional UI languages.

Phase 3: dashboard and widgets, multiple panels, a plugin and extension system.

Ent and sqlc adapters are scheduled independently of phases.

## 10. Milestone M0

The `Product` resource end to end: list, create, edit, delete on GORM and Postgres, login, light and dark theme.

Definition of done: the resource is assembled from a single builder description of fewer than 30 lines.

No calendar deadlines. Work is done at a relaxed pace, solo.

Implementation plan: `docs/superpowers/plans/2026-09-26-m0-product-resource.md`.

## 11. Demo site

A small public shop-like site ("storefront with orders"): categories, products with photos, customers, orders, order items, roles `admin` and `manager`. It exercises relations, file uploads, statuses, roles and bulk actions. It doubles as the public live demo of the panel and lives in this repository under `examples/shop` with its own `go.mod`, so CI builds both.

Hosting and abuse protection for the public demo are undecided and deferred. Working notes for when it is decided: isolated cheap VPS, public demo login, data reset from a seed on a schedule, uploads restricted to small images with type checking and re-encoding, request rate limits, a Postgres role without schema privileges.

## 12. Security defaults

- CSRF tokens on all state-changing requests.
- Passwords hashed with argon2id.
- Session cookies with `HttpOnly`, `Secure`, `SameSite`.
- HTML escaping by default through templ.
- Upload validation: type and size.
- Login attempt rate limiting.

## 13. Testing and documentation

- Regular Go tests.
- Integration tests against real Postgres in Docker with a published host port.
- End-to-end tests with Playwright later.
- Phase 1 docs: README and Markdown files in `docs/`. A dedicated docs site comes later.

## 14. Packaging

- Name: Tellus.
- Repository: the owner's personal GitHub account.
- License: Apache-2.0.
- Versioning: `v0.x` until the end of phase 2, semantic versioning after.
- Go: developed on the latest stable release. The module's `go` directive is 1.26.0, which the dependencies require; 1.26 and 1.27 are the supported releases.
- Layout: library at the repository root, demo site in `examples/shop` with a separate `go.mod`.

## 15. Open questions and risks

- Demo hosting and abuse protection: deferred by the owner.
- Scope: phase 1 is larger than the original estimate because dark theme, custom fields and actions moved into it. Original estimate: about 3 to 5 months of solo work to a public alpha, 9 to 15 months to a stable v1, and at a relaxed part-time pace this can double.
- Public `Field` interface in phase 1 will probably break at least once before `v1.0`.
- Name collisions and domain: `pkg.go.dev` search for "tellus" showed one unrelated package. `tellus.dev` and `tellus.io` are taken. Domain availability was not verified with a registrar.
- Not verified: how htmx swaps interact with Alpine re-initialization in a data-heavy table. Prototype this early in M0.
