# Tellus M0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build milestone M0 of Tellus: a `Product` resource end to end (list with search, sorting and pagination, create, edit, delete, sign-in, light and dark theme) on GORM and Postgres, assembled from a builder description of fewer than 30 lines.

**Architecture:** Tellus is a library mounted into a host Go application as an `http.Handler`. Pages are rendered on the server with templ; htmx swaps the list region in place and Alpine handles small interactions. A `DataSource` interface separates the panel from GORM. Small internal packages hold the security primitives (`secure`), the pages (`ui`), the embedded assets (`assets`) and the string catalog (`i18n`). Public packages `form` and `table` hold the field and column builders.

**Tech Stack:** Go 1.26+, templ v0.3.1020, GORM v1.31.2 with the Postgres driver v1.6.3, golang.org/x/crypto v0.57.0 (argon2id), htmx 2.0.11, Alpine.js 3.17.4, Inter Variable 5.3.0, plain modern CSS (custom properties and `@layer`), Postgres 17 in Docker for tests.

**Spec:** `docs/spec.md` (the plan argues from it; read both).

## Global Constraints

- Module path `github.com/TechnoVizor/tellus`. The `go` directive is `1.26.0` (the dependencies need it; spec section 14 says latest stable Go, which is 1.27 for development).
- License Apache-2.0. Third-party embedded files are listed in `THIRD_PARTY_NOTICES.md`.
- English only UI. Every user-facing string goes through `i18n.T("key")` and the key must exist in `internal/i18n/en.go`.
- No em-dashes anywhere: not in code, comments, docs, copy or commit messages.
- No CDN and no Node in a user's build: all JS, CSS and fonts are embedded with `embed`.
- Plain modern CSS with custom properties and `@layer`. No Tailwind and no third-party UI kit in the public API. One fixed design with light and dark themes; the look follows the Farhaven and mybee.lv sites.
- Postgres first through GORM, behind the `DataSource` interface. Integration tests use the real Postgres from `docker-compose.yml`, published on host port 55432, through `TELLUS_TEST_DSN`.
- Security defaults: CSRF tokens on all state-changing requests, argon2id passwords, session and CSRF cookies with `HttpOnly`, `Secure` (unless `InsecureCookies`) and `SameSite=Lax`, HTML escaping through templ, sign-in rate limiting, request bodies capped at 1 MiB.
- The panel prefix is configurable, default `/admin`. Every generated link, redirect and cookie path is built from it.
- Generated `*_templ.go` files are committed. Regenerate with `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate` after any `.templ` change.
- Pinned versions: htmx 2.0.11, Alpine.js 3.17.4, Inter Variable 5.3.0 (fontsource), templ v0.3.1020, gorm.io/gorm v1.31.2, gorm.io/driver/postgres v1.6.3, golang.org/x/crypto v0.57.0, Postgres image `postgres:17-alpine`.
- Commands are bash (Git Bash on Windows) and run from the repository root `D:/My repos/tellus` unless stated. Docker must be running.
- Commit messages follow Conventional Commits.

## Review Focus

Inputs the spec implies but a happy-path build would miss. Each one is pinned by a named test in the task that owns the code.

1. A session that expires while htmx is refreshing the list region must navigate the whole page to sign-in, not swap the sign-in form into the table (Task 13, `TestUnauthenticatedRequestsGoToLogin`).
2. Hand-edited list URLs (`page=0`, `page=99999999999999999999`, an unknown `sort`, `dir=sideways`, a 500 character `q`) must fall back to safe defaults or the last page, never a 500 (Task 14, `TestListPassesOnlyWhitelistedParameters`, `TestListClampsPagePastTheEnd`).
3. Form posts carrying fields the form does not declare (`ID`, `CreatedAt`) must be ignored, and an unchecked toggle (absent from the post) must save `false` (Task 14, `TestCreateIgnoresUndeclaredFields`, `TestUpdateSavesAndUncheckedToggleMeansFalse`).
4. Search terms with `%`, `_` and SQL quote characters must match literally and never break the query (Task 9, `TestGormSourceSearch`).
5. Stored text containing HTML or script must come back escaped in the list, the edit form and a re-rendered error form (Task 14, `TestUserInputIsEscapedEverywhere`).

## Deliberately not in this plan

Relations, filters, more field and column types, actions and bulk actions, file uploads, per-resource policies, logo and favicon branding, additional database adapters and the public demo hosting. Known limits carried forward: sessions are stateless (sign-out does not revoke a stolen cookie), the login limiter is in-memory per process, the CSRF cookie is not bound to the session, Alpine's standard build needs `unsafe-eval` so no strict Content-Security-Policy yet, model fields that are pointers are not supported by the built-in fields, `uuid.UUID` primary keys are not supported (use a string key), and text search uses Postgres `ILIKE`.

---

### Task 1: Repository scaffold and test database

A git repository with a Go module, the license, a Postgres for integration tests, and a helper that gives each test an isolated schema.

**Files:**
- Create: `.gitignore`
- Create: `docker-compose.yml`
- Create: `docker/initdb/01-dev-db.sql`
- Create: `internal/testdb/testdb.go`
- Create: `go.mod`, `go.sum`, `LICENSE` (through commands below)
- Test: `internal/testdb/testdb_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `testdb.Open(t testing.TB) *gorm.DB`. Skips the test when `TELLUS_TEST_DSN` is unset. Otherwise returns a GORM handle bound to a fresh schema (`t_<random>`) that is dropped when the test ends, so packages can run in parallel against one database. The DSN must be URL form.

- [ ] **Step 1: Initialise the repository and the module**

```bash
cd "D:/My repos/tellus"
git init -b main
go mod init github.com/TechnoVizor/tellus
go mod edit -go=1.26.0
curl -sSL -o LICENSE https://www.apache.org/licenses/LICENSE-2.0.txt
go get gorm.io/gorm@v1.31.2 gorm.io/driver/postgres@v1.6.3
```

The `go` directive is set to 1.26.0 on purpose. The spec asks for the latest stable Go; the dependencies require 1.26, and 1.26 and 1.27 are the two supported releases. Do not raise it further.

- [ ] **Step 2: Write the failing test**

Create `docker-compose.yml`:

````yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: tellus
      POSTGRES_PASSWORD: tellus
      POSTGRES_DB: tellus_test
    ports:
      - "55432:5432"
    volumes:
      - ./docker/initdb:/docker-entrypoint-initdb.d:ro
      - tellus-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U tellus -d tellus_test"]
      interval: 2s
      timeout: 3s
      retries: 20

volumes:
  tellus-pgdata:
````

Create `docker/initdb/01-dev-db.sql`:

````sql
-- Dev database for examples/shop. Integration tests use tellus_test (POSTGRES_DB).
CREATE DATABASE tellus_dev;
````

Create `.gitignore`:

````gitignore
# build output
*.exe
*.test
*.out

# local config and logs
.env
.env.*
*.log

# editors
.idea/
.vscode/
````

Create `internal/testdb/testdb_test.go`:

````go
package testdb

import "testing"

func TestOpenGivesIsolatedSchema(t *testing.T) {
	db := Open(t)

	var schema string
	if err := db.Raw(`SELECT current_schema()`).Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	if len(schema) < 3 || schema[:2] != "t_" {
		t.Fatalf("expected a t_* schema, got %q", schema)
	}
	if err := db.Exec(`CREATE TABLE probe (id int)`).Error; err != nil {
		t.Fatalf("table create in isolated schema failed: %v", err)
	}
}
````

`docker-compose.yml`, `.gitignore` and the init script are infrastructure, not tested logic. `testdb_test.go` is the test that fails first.

- [ ] **Step 3: Start Postgres**

```bash
docker compose up -d --wait
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
```

`TELLUS_TEST_DSN` must be exported in every shell that runs tests. Port 55432 is published on the host on purpose. Do not use 5432: another project's database may already hold it.

- [ ] **Step 4: Run to verify it fails**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test ./internal/testdb/ -v
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Open` or `no non-test Go files`).

- [ ] **Step 5: Write the implementation**

Create `internal/testdb/testdb.go`:

````go
// Package testdb gives integration tests an isolated Postgres schema.
package testdb

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open returns a GORM handle bound to a fresh schema that is dropped when the
// test ends. The test is skipped when TELLUS_TEST_DSN is not set. The DSN must
// be in URL form, for example
// postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TELLUS_TEST_DSN")
	if dsn == "" {
		t.Skip("TELLUS_TEST_DSN not set; run `docker compose up -d --wait` and export it")
	}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	admin, err := gorm.Open(postgres.Open(dsn), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(buf)
	if err := admin.Exec(`CREATE SCHEMA ` + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), cfg)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`).Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
````

- [ ] **Step 6: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/testdb/ -v
```

Expected: PASS, `--- PASS: TestOpenGivesIsolatedSchema` and `ok  github.com/TechnoVizor/tellus/internal/testdb`.

- [ ] **Step 7: Commit**

```bash
git add docs LICENSE
git commit -m "docs: add design spec, M0 plan and Apache-2.0 license"
git add go.mod go.sum .gitignore docker-compose.yml docker internal/testdb
git commit -m "chore: add module scaffold and isolated-schema test database helper"
```

### Task 2: Label helper and English string catalog

`humanize.Name` turns Go identifiers into labels. `i18n.T` looks up UI strings, and the catalog holds every string the later tasks show.

**Files:**
- Create: `internal/humanize/humanize.go`
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/en.go`
- Test: `internal/humanize/humanize_test.go`
- Test: `internal/i18n/i18n_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `humanize.Name(s string) string` (`CreatedAt` becomes `Created at`, `CategoryID` becomes `Category ID`, `order_items` becomes `Order items`). `i18n.T(key string) string` returns the English text or the key itself when unknown. The catalog in `en.go` already contains the keys used by later tasks (`validation.*`, `common.*`, `ui.*`, `login.*`, `list.*`, `form.*`, `notice.*`).

- [ ] **Step 1: Write the failing tests**

Create `internal/humanize/humanize_test.go`:

````go
package humanize

import "testing"

func TestName(t *testing.T) {
	cases := map[string]string{
		"Name":        "Name",
		"name":        "Name",
		"CreatedAt":   "Created at",
		"ID":          "ID",
		"CategoryID":  "Category ID",
		"HTMLBody":    "HTML body",
		"order_items": "Order items",
		"products":    "Products",
		"Line1Text":   "Line1 text",
		"":            "",
	}
	for in, want := range cases {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}
````

Create `internal/i18n/i18n_test.go`:

````go
package i18n

import "testing"

func TestTKnownAndUnknownKeys(t *testing.T) {
	if got := T("common.yes"); got != "Yes" {
		t.Errorf("known key: got %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key should echo the key, got %q", got)
	}
}

func TestCatalogHasNoEmptyValues(t *testing.T) {
	for k, v := range en {
		if v == "" {
			t.Errorf("key %q has an empty value", k)
		}
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/humanize/ ./internal/i18n/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Name` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `internal/humanize/humanize.go`:

````go
// Package humanize turns Go identifiers and table names into readable labels.
package humanize

import (
	"strings"
	"unicode"
)

// Name converts an identifier into a label:
// "CreatedAt" becomes "Created at", "CategoryID" becomes "Category ID",
// "order_items" becomes "Order items".
func Name(s string) string {
	var words []string
	for _, part := range strings.Fields(strings.ReplaceAll(s, "_", " ")) {
		words = append(words, splitCamel(part)...)
	}
	for i, w := range words {
		switch {
		case isAcronym(w):
			// keep ID, HTML, URL as written
		case i == 0:
			words[i] = upperFirst(strings.ToLower(w))
		default:
			words[i] = strings.ToLower(w)
		}
	}
	return strings.Join(words, " ")
}

// splitCamel splits "HTMLBody" into "HTML", "Body" and "CreatedAt" into
// "Created", "At".
func splitCamel(s string) []string {
	runes := []rune(s)
	var words []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		lowerToUpper := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur)
		acronymEnd := unicode.IsUpper(prev) && unicode.IsUpper(cur) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if lowerToUpper || acronymEnd {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	return append(words, string(runes[start:]))
}

func isAcronym(w string) bool {
	if len([]rune(w)) < 2 {
		return false
	}
	for _, r := range w {
		if unicode.IsLower(r) {
			return false
		}
	}
	return true
}

func upperFirst(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
````

Create `internal/i18n/i18n.go`:

````go
// Package i18n looks up UI strings. Only English exists for now, but every
// user-facing string goes through T so adding a language later is cheap.
package i18n

// T returns the English text for key, or the key itself when it is unknown.
func T(key string) string {
	if s, ok := en[key]; ok {
		return s
	}
	return key
}
````

Create `internal/i18n/en.go`:

````go
package i18n

var en = map[string]string{
	// validation
	"validation.required":   "This field is required.",
	"validation.max_length": "Must be at most %d characters.",
	"validation.number":     "Enter a valid number.",

	// common
	"common.yes":       "Yes",
	"common.no":        "No",
	"common.not_found": "Not found.",

	// layout
	"ui.navigation":   "Main navigation",
	"ui.theme":        "Theme",
	"ui.toggle_theme": "Switch between light and dark theme",
	"ui.sign_out":     "Sign out",
	"ui.no_resources": "No resources are registered yet.",

	// login
	"login.title":     "Sign in",
	"login.lede":      "Sign in to continue.",
	"login.email":     "Email",
	"login.password":  "Password",
	"login.submit":    "Sign in",
	"login.invalid":   "Invalid email or password.",
	"login.too_many":  "Too many attempts. Try again in a few minutes.",
	"login.forbidden": "This account cannot access the panel.",

	// list
	"list.new":            "New",
	"list.search":         "Search",
	"list.empty":          "Nothing here yet.",
	"list.actions":        "Actions",
	"list.edit":           "Edit",
	"list.delete":         "Delete",
	"list.confirm_delete": "Delete this record? This cannot be undone.",
	"list.summary":        "%d records, page %d of %d",
	"list.previous":       "Previous",
	"list.next":           "Next",

	// form
	"form.new_title":     "New %s",
	"form.edit_title":    "Edit %s",
	"form.save":          "Save",
	"form.cancel":        "Cancel",
	"form.error_summary": "Some fields need attention.",

	// notices shown after a redirect
	"notice.created": "Record created.",
	"notice.updated": "Record saved.",
	"notice.deleted": "Record deleted.",
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/humanize/ ./internal/i18n/
```

Expected: PASS, `ok` for both packages.

- [ ] **Step 5: Commit**

```bash
git add internal/humanize internal/i18n
git commit -m "feat: add label humanizer and English string catalog"
```

### Task 3: Password hashing

argon2id password hashing with a constant-time dummy check for unknown accounts.

**Files:**
- Create: `internal/secure/password.go`
- Test: `internal/secure/password_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `secure.HashPassword(password string) (string, error)` returns a PHC string `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>`. `secure.VerifyPassword(password, encoded string) (bool, error)` returns `secure.ErrBadHash` for a malformed hash. `secure.DummyVerify(password string)` spends the cost of one verification.

- [ ] **Step 1: Prepare**

```bash
go get golang.org/x/crypto@v0.57.0
```

- [ ] **Step 2: Write the failing test**

Create `internal/secure/password_test.go`:

````go
package secure

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	ok, err := VerifyPassword("correct horse", hash)
	if err != nil || !ok {
		t.Fatalf("right password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plaintext", "$argon2id$v=19$m=1,t=1,p=1$!!$!!", "$bcrypt$x$y$z$w"} {
		if _, err := VerifyPassword("x", bad); err != ErrBadHash {
			t.Errorf("hash %q: want ErrBadHash, got %v", bad, err)
		}
	}
}

func TestDummyVerifyDoesNotPanic(t *testing.T) {
	DummyVerify("anything")
}
````

- [ ] **Step 3: Run to verify it fails**

```bash
go test ./internal/secure/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: HashPassword` or `no non-test Go files`).

- [ ] **Step 4: Write the implementation**

Create `internal/secure/password.go`:

````go
// Package secure holds the small security primitives the panel is built on:
// password hashing, signed tokens, CSRF protection and a login rate limiter.
package secure

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters: OWASP password storage cheat sheet minimum profile.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrBadHash means the stored password hash could not be parsed.
var ErrBadHash = errors.New("secure: malformed password hash")

// HashPassword returns a PHC-formatted argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches a hash made by HashPassword.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrBadHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, ErrBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, ErrBadHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// DummyVerify spends the same time as a real VerifyPassword. Call it when the
// account does not exist so response time does not reveal which emails are
// registered.
func DummyVerify(password string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("tellus-dummy-password") })
	_, _ = VerifyPassword(password, dummyHash)
}
````

- [ ] **Step 5: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/secure/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/internal/secure`.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/secure
git commit -m "feat: add argon2id password hashing"
```

### Task 4: Signed session tokens

Tamper-proof, expiring tokens for the session cookie.

**Files:**
- Create: `internal/secure/token.go`
- Test: `internal/secure/token_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `secure.NewSigner(secret []byte) (*Signer, error)` (secret of at least 32 bytes). `(*Signer).Sign(value string, ttl time.Duration, now time.Time) string`. `(*Signer).Verify(token string, now time.Time) (value string, ok bool)`. Tokens are `base64url(expiry|value).base64url(HMAC-SHA256)`.

- [ ] **Step 1: Write the failing test**

Create `internal/secure/token_test.go`:

````go
package secure

import (
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignerRoundTrip(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	got, ok := s.Verify(token, now.Add(time.Minute))
	if !ok || got != "42" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestSignerKeepsPipeInValue(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	got, ok := s.Verify(s.Sign("a|b|c", time.Hour, now), now)
	if !ok || got != "a|b|c" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestSignerRejectsExpired(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	if _, ok := s.Verify(token, now.Add(time.Hour)); ok {
		t.Fatal("token accepted at its expiry instant")
	}
}

func TestSignerRejectsTampering(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	body, sig, _ := strings.Cut(token, ".")

	other, _ := NewSigner([]byte("ffffffffffffffffffffffffffffffff"))
	cases := map[string]string{
		"flipped body":  "A" + body[1:] + "." + sig,
		"flipped sig":   body + "." + "A" + sig[1:],
		"no separator":  body,
		"empty":         "",
		"garbage":       "%%%.%%%",
		"other secret":  other.Sign("42", time.Hour, now),
		"signature cut": body + ".",
	}
	for name, tok := range cases {
		if _, ok := s.Verify(tok, now); ok {
			t.Errorf("%s: token accepted", name)
		}
	}
}

func TestNewSignerRejectsShortSecret(t *testing.T) {
	if _, err := NewSigner([]byte("short")); err == nil {
		t.Fatal("expected error for short secret")
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/secure/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: NewSigner` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `internal/secure/token.go`:

````go
package secure

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Signer issues and checks tamper-proof tokens:
// base64url(expiry|value) + "." + base64url(HMAC-SHA256(secret, expiry|value)).
type Signer struct{ secret []byte }

// NewSigner requires a secret of at least 32 bytes.
func NewSigner(secret []byte) (*Signer, error) {
	if len(secret) < 32 {
		return nil, errors.New("secure: signing secret must be at least 32 bytes")
	}
	return &Signer{secret: append([]byte(nil), secret...)}, nil
}

// Sign returns a token carrying value that expires ttl after now.
func (s *Signer) Sign(value string, ttl time.Duration, now time.Time) string {
	payload := strconv.FormatInt(now.Add(ttl).Unix(), 10) + "|" + value
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return body + "." + base64.RawURLEncoding.EncodeToString(s.mac(body))
}

// Verify returns the value of a valid, unexpired token.
func (s *Signer) Verify(token string, now time.Time) (string, bool) {
	body, sig, found := strings.Cut(token, ".")
	if !found {
		return "", false
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, s.mac(body)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	expiry, value, found := strings.Cut(string(raw), "|")
	if !found {
		return "", false
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	return value, true
}

func (s *Signer) mac(body string) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(body))
	return m.Sum(nil)
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/secure/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/internal/secure`.

- [ ] **Step 5: Commit**

```bash
git add internal/secure
git commit -m "feat: add signed expiring tokens"
```

### Task 5: CSRF middleware

Double-submit cookie protection for every state-changing request.

**Files:**
- Create: `internal/secure/csrf.go`
- Test: `internal/secure/csrf_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: Constants `secure.CSRFCookie = "tellus_csrf"`, `secure.CSRFHeader = "X-CSRF-Token"`, `secure.CSRFField = "_csrf"`. `secure.CSRF{Path string; Secure bool}` with `Middleware(next http.Handler) http.Handler`. `secure.CSRFToken(ctx context.Context) string` returns the current request's token for forms. Unsafe methods (anything but GET, HEAD, OPTIONS) without a matching header or form token get 403.

- [ ] **Step 1: Write the failing test**

Create `internal/secure/csrf_test.go`:

````go
package secure

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func csrfHandler(seen *string) http.Handler {
	return CSRF{Path: "/admin"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = CSRFToken(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
}

func issueToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookie {
			return c.Value
		}
	}
	t.Fatal("no CSRF cookie issued on GET")
	return ""
}

func TestCSRFIssuesCookieOnGet(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("cookie not set")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/admin" {
		t.Fatalf("bad cookie attributes: %+v", cookie)
	}
	if seen != cookie.Value {
		t.Fatalf("context token %q != cookie %q", seen, cookie.Value)
	}
}

func TestCSRFKeepsExistingToken(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	token := issueToken(t, h)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("cookie was re-issued although a valid one was sent")
	}
	if seen != token {
		t.Fatal("token in context differs from cookie")
	}
}

func TestCSRFRejectsUnsafeRequests(t *testing.T) {
	var seen string
	h := csrfHandler(&seen)
	token := issueToken(t, h)
	other := strings.Repeat("a", 64)

	form := func(tok string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{CSRFField: {tok}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	withCookie := func(r *http.Request, v string) *http.Request {
		r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: v})
		return r
	}
	noToken := httptest.NewRequest(http.MethodPost, "/", nil)
	header := httptest.NewRequest(http.MethodDelete, "/", nil)
	header.Header.Set(CSRFHeader, token)

	cases := []struct {
		name string
		req  *http.Request
		want int
	}{
		{"no cookie no token", noToken, http.StatusForbidden},
		{"cookie but no token", withCookie(httptest.NewRequest(http.MethodPost, "/", nil), token), http.StatusForbidden},
		{"token but no cookie", form(token), http.StatusForbidden},
		{"mismatch", withCookie(form(other), token), http.StatusForbidden},
		{"valid form field", withCookie(form(token), token), http.StatusNoContent},
		{"valid header", withCookie(header, token), http.StatusNoContent},
		{"malformed cookie", withCookie(form("short"), "short"), http.StatusForbidden},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, tc.req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/secure/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: CSRF` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `internal/secure/csrf.go`:

````go
package secure

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

// CSRF cookie, request header and form field names.
const (
	CSRFCookie = "tellus_csrf"
	CSRFHeader = "X-CSRF-Token"
	CSRFField  = "_csrf"
)

type csrfKey struct{}

// CSRF protects state-changing requests with the double-submit cookie pattern:
// the token in the cookie must be echoed in the X-CSRF-Token header or the
// _csrf form field. Forms and htmx requests get the token from CSRFToken.
//
// ponytail: the cookie is not bound to the session, so a sibling subdomain that
// can set cookies could fix a known token. Bind it to the session if that
// threat matters.
type CSRF struct {
	Path   string // cookie path, the panel prefix (empty means "/")
	Secure bool   // set the Secure cookie attribute
}

// Middleware issues the cookie on first contact, stores the token in the
// request context and rejects unsafe requests without a matching token.
func (c CSRF) Middleware(next http.Handler) http.Handler {
	path := c.Path
	if path == "" {
		path = "/"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if ck, err := r.Cookie(CSRFCookie); err == nil && validCSRFToken(ck.Value) {
			token = ck.Value
		}
		if !isSafeMethod(r.Method) {
			sent := r.Header.Get(CSRFHeader)
			if sent == "" {
				sent = r.PostFormValue(CSRFField)
			}
			if token == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		if token == "" {
			token = newCSRFToken()
			http.SetCookie(w, &http.Cookie{
				Name:     CSRFCookie,
				Value:    token,
				Path:     path,
				HttpOnly: true,
				Secure:   c.Secure,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, token)))
	})
}

// CSRFToken returns the token of the current request, for embedding in forms
// and htmx headers.
func CSRFToken(ctx context.Context) string {
	token, _ := ctx.Value(csrfKey{}).(string)
	return token
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return hex.EncodeToString(b)
}

func validCSRFToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/secure/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/internal/secure`.

- [ ] **Step 5: Commit**

```bash
git add internal/secure
git commit -m "feat: add double-submit CSRF middleware"
```

### Task 6: Login rate limiter

A per-key sliding-window limiter for sign-in attempts.

**Files:**
- Create: `internal/secure/limiter.go`
- Test: `internal/secure/limiter_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `secure.NewLimiter(max int, window time.Duration) *Limiter` and `(*Limiter).Allow(key string) bool`. Every call counts as an event.

- [ ] **Step 1: Write the failing test**

Create `internal/secure/limiter_test.go`:

````go
package secure

import (
	"testing"
	"time"
)

func TestLimiterBlocksAfterMax(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("attempt %d blocked too early", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th attempt inside the window was allowed")
	}
	if !l.Allow("other-ip") {
		t.Fatal("limit leaked across keys")
	}
}

func TestLimiterWindowSlides(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	l.Allow("ip")
	l.Allow("ip")
	if l.Allow("ip") {
		t.Fatal("expected block")
	}
	now = now.Add(time.Minute + time.Second)
	if !l.Allow("ip") {
		t.Fatal("still blocked after the window passed")
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/secure/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: NewLimiter` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `internal/secure/limiter.go`:

````go
package secure

import (
	"sync"
	"time"
)

// Limiter allows at most max events per window for each key.
//
// ponytail: in-memory and per process. Use a shared store if the panel runs on
// several instances and the limit must hold across them.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	now    func() time.Time
}

// NewLimiter returns a limiter allowing max events per window per key.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string][]time.Time{}, now: time.Now}
}

// Allow records an event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if len(l.hits) > 10_000 { // bound memory when many distinct keys show up
		for k := range l.hits {
			l.prune(k, now)
		}
	}
	l.prune(key, now)
	if len(l.hits[key]) >= l.max {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}

func (l *Limiter) prune(key string, now time.Time) {
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = kept
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/secure/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/internal/secure`.

- [ ] **Step 5: Commit**

```bash
git add internal/secure
git commit -m "feat: add sliding-window rate limiter"
```

### Task 7: Form fields

The `form` package: `Text`, `Textarea`, `Number` and `Toggle` fields with parsing, validation and templ rendering.

**Files:**
- Create: `form/field.go`
- Create: `form/fields.go`
- Create: `form/render.templ`
- Test: `form/fields_test.go`

**Interfaces:**
- Consumes: `humanize.Name`, `i18n.T` keys `validation.required`, `validation.max_length`, `validation.number`.
- Produces: `form.Info{Name, Label string; Required bool}`, `form.Value{Raw, Error string}`, and the `form.Field` interface with `Info() Info`, `Check(target reflect.Type) error`, `Format(v any) string`, `Parse(raw string, target reflect.Type) (value any, message string)`, `Render(v Value) templ.Component`. Builders: `form.Text(name)` and `form.Textarea(name)` return `*TextField` (`Label`, `Required`, `MaxLength`), `form.Number(name)` returns `*NumberField` (`Label`, `Required`), `form.Toggle(name)` returns `*ToggleField` (`Label`). `Parse` returns a value of exactly the target type, so callers can `reflect.Value.Set` it.

- [ ] **Step 1: Prepare**

```bash
go get github.com/a-h/templ@v0.3.1020
```

- [ ] **Step 2: Write the failing test**

Create `form/fields_test.go`:

````go
package form

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

var (
	tString  = reflect.TypeOf("")
	tInt     = reflect.TypeOf(0)
	tInt8    = reflect.TypeOf(int8(0))
	tUint    = reflect.TypeOf(uint(0))
	tFloat64 = reflect.TypeOf(0.0)
	tBool    = reflect.TypeOf(false)
)

func render(t *testing.T, f Field, v Value) string {
	t.Helper()
	var buf bytes.Buffer
	if err := f.Render(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTextParse(t *testing.T) {
	f := Text("Name").Required().MaxLength(5)

	if _, msg := f.Parse("  ", tString); msg == "" {
		t.Error("whitespace-only input must fail Required")
	}
	if _, msg := f.Parse("toolong", tString); msg == "" {
		t.Error("input over MaxLength must fail")
	}
	// MaxLength counts characters, not bytes.
	if v, msg := f.Parse("héllo", tString); msg != "" || v != "héllo" {
		t.Errorf("5 characters must fit MaxLength(5): v=%v msg=%q", v, msg)
	}
	loose := Text("Name")
	if v, msg := loose.Parse("", tString); msg != "" || v != "" {
		t.Errorf("optional empty text: v=%v msg=%q", v, msg)
	}
}

func TestTextKeepsNamedStringType(t *testing.T) {
	type Slug string
	v, msg := Text("Slug").Parse("abc", reflect.TypeOf(Slug("")))
	if msg != "" {
		t.Fatal(msg)
	}
	if _, ok := v.(Slug); !ok {
		t.Fatalf("Parse must return the target type, got %T", v)
	}
}

func TestNumberParse(t *testing.T) {
	cases := []struct {
		name   string
		f      *NumberField
		raw    string
		target reflect.Type
		want   any
		bad    bool
	}{
		{"int", Number("P"), "42", tInt, 42, false},
		{"negative int", Number("P"), "-7", tInt, -7, false},
		{"int8 overflow", Number("P"), "300", tInt8, nil, true},
		{"uint negative", Number("P"), "-1", tUint, nil, true},
		{"float", Number("P"), "1.5", tFloat64, 1.5, false},
		{"not a number", Number("P"), "abc", tInt, nil, true},
		{"int given a decimal", Number("P"), "1.5", tInt, nil, true},
		{"NaN rejected", Number("P"), "NaN", tFloat64, nil, true},
		{"Inf rejected", Number("P"), "Inf", tFloat64, nil, true},
		{"empty optional is zero", Number("P"), "", tInt, 0, false},
		{"empty required fails", Number("P").Required(), "", tInt, nil, true},
		{"spaces trimmed", Number("P"), " 8 ", tInt, 8, false},
	}
	for _, tc := range cases {
		got, msg := tc.f.Parse(tc.raw, tc.target)
		if tc.bad {
			if msg == "" {
				t.Errorf("%s: expected a validation message", tc.name)
			}
			continue
		}
		if msg != "" || got != tc.want {
			t.Errorf("%s: got %v (%q), want %v", tc.name, got, msg, tc.want)
		}
	}
}

func TestNumberFormat(t *testing.T) {
	f := Number("P")
	for _, tc := range []struct {
		in   any
		want string
	}{{42, "42"}, {uint(7), "7"}, {1.5, "1.5"}, {1e21, "1000000000000000000000"}} {
		if got := f.Format(tc.in); got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestToggleParseAndFormat(t *testing.T) {
	f := Toggle("Active")
	for raw, want := range map[string]bool{"": false, "false": false, "0": false, "off": false, "true": true, "on": true} {
		got, msg := f.Parse(raw, tBool)
		if msg != "" || got != want {
			t.Errorf("Parse(%q) = %v (%q), want %v", raw, got, msg, want)
		}
	}
	if f.Format(true) != "true" || f.Format(false) != "" {
		t.Error("Format must return \"true\" for checked and \"\" otherwise")
	}
}

func TestCheckRejectsWrongModelType(t *testing.T) {
	if err := Text("X").Check(tInt); err == nil {
		t.Error("Text on an int field must be rejected")
	}
	if err := Number("X").Check(tString); err == nil {
		t.Error("Number on a string field must be rejected")
	}
	if err := Toggle("X").Check(tInt); err == nil {
		t.Error("Toggle on an int field must be rejected")
	}
	if err := Text("X").Check(tString); err != nil {
		t.Error(err)
	}
}

func TestDefaultLabelAndOverride(t *testing.T) {
	if got := Text("CreatedAt").Info().Label; got != "Created at" {
		t.Errorf("default label: %q", got)
	}
	if got := Text("CreatedAt").Label("Added").Info().Label; got != "Added" {
		t.Errorf("override label: %q", got)
	}
}

func TestRenderTextEscapesValueAndShowsError(t *testing.T) {
	out := render(t, Text("Name").Required().MaxLength(10), Value{
		Raw:   `"><script>alert(1)</script>`,
		Error: "Bad value",
	})
	if strings.Contains(out, "<script>") {
		t.Fatalf("value was not escaped: %s", out)
	}
	for _, want := range []string{`name="Name"`, `maxlength="10"`, `required`, `aria-invalid="true"`, "Bad value"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestRenderToggleChecked(t *testing.T) {
	checked := render(t, Toggle("Active"), Value{Raw: "true"})
	if !strings.Contains(checked, "checked") {
		t.Errorf("expected checked: %s", checked)
	}
	unchecked := render(t, Toggle("Active"), Value{})
	if strings.Contains(unchecked, "checked") {
		t.Errorf("did not expect checked: %s", unchecked)
	}
}

func TestRenderTextareaAndNumber(t *testing.T) {
	area := render(t, Textarea("Description"), Value{Raw: "line1\nline2"})
	if !strings.Contains(area, "<textarea") || !strings.Contains(area, "line1\nline2") {
		t.Errorf("textarea render: %s", area)
	}
	num := render(t, Number("Price"), Value{Raw: "12"})
	if !strings.Contains(num, `type="number"`) || !strings.Contains(num, `value="12"`) {
		t.Errorf("number render: %s", num)
	}
}
````

- [ ] **Step 3: Run to verify it fails**

```bash
go test ./form/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Text` or `no non-test Go files`).

- [ ] **Step 4: Write the implementation**

Create `form/field.go`:

````go
// Package form holds the field builders used to describe a resource's create
// and edit form. Custom fields implement Field.
package form

import (
	"reflect"

	"github.com/a-h/templ"
)

// Info is the metadata every field exposes.
type Info struct {
	Name     string // Go struct field name on the model
	Label    string // human label shown next to the input
	Required bool
}

// Value is what a field needs to draw itself.
type Value struct {
	Raw   string // current value as text: submitted input on redraw, formatted model value otherwise
	Error string // validation message, empty when the value is valid
}

// Field is one input on a record form.
type Field interface {
	Info() Info
	// Check runs once at startup and rejects a field bound to a model field of
	// the wrong Go type.
	Check(target reflect.Type) error
	// Format renders the model's current value as the text shown in the input.
	Format(v any) string
	// Parse converts submitted text into a value of exactly the target type. A
	// non-empty message means the input is invalid and is shown to the user.
	Parse(raw string, target reflect.Type) (value any, message string)
	// Render draws the label, the input and the error message.
	Render(v Value) templ.Component
}
````

Create `form/fields.go`:

````go
package form

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/internal/i18n"
)

// TextField is a single-line or multi-line text input bound to a string field.
type TextField struct {
	name      string
	label     string
	required  bool
	maxLength int
	multiline bool
}

// Text returns a single-line text input for the named model field.
func Text(name string) *TextField {
	return &TextField{name: name, label: humanize.Name(name)}
}

// Textarea returns a multi-line text input for the named model field.
func Textarea(name string) *TextField {
	f := Text(name)
	f.multiline = true
	return f
}

// Label overrides the default label.
func (f *TextField) Label(label string) *TextField { f.label = label; return f }

// Required rejects empty or whitespace-only input.
func (f *TextField) Required() *TextField { f.required = true; return f }

// MaxLength rejects input longer than n characters.
func (f *TextField) MaxLength(n int) *TextField { f.maxLength = n; return f }

func (f *TextField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *TextField) Check(target reflect.Type) error {
	if target.Kind() != reflect.String {
		return fmt.Errorf("form.Text(%q) needs a string field, the model field is %s", f.name, target)
	}
	return nil
}

func (f *TextField) Format(v any) string { return fmt.Sprint(v) }

func (f *TextField) Parse(raw string, target reflect.Type) (any, string) {
	if f.required && strings.TrimSpace(raw) == "" {
		return nil, i18n.T("validation.required")
	}
	if f.maxLength > 0 && utf8.RuneCountInString(raw) > f.maxLength {
		return nil, fmt.Sprintf(i18n.T("validation.max_length"), f.maxLength)
	}
	out := reflect.New(target).Elem()
	out.SetString(raw)
	return out.Interface(), ""
}

func (f *TextField) Render(v Value) templ.Component {
	if f.multiline {
		return textareaInput(f, v)
	}
	return textInput(f, v)
}

// NumberField is a numeric input bound to an integer or float field.
type NumberField struct {
	name     string
	label    string
	required bool
}

// Number returns a numeric input for the named model field.
func Number(name string) *NumberField {
	return &NumberField{name: name, label: humanize.Name(name)}
}

// Label overrides the default label.
func (f *NumberField) Label(label string) *NumberField { f.label = label; return f }

// Required rejects empty input. Without it, empty input stores zero.
func (f *NumberField) Required() *NumberField { f.required = true; return f }

func (f *NumberField) Info() Info {
	return Info{Name: f.name, Label: f.label, Required: f.required}
}

func (f *NumberField) Check(target reflect.Type) error {
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	}
	return fmt.Errorf("form.Number(%q) needs an integer or float field, the model field is %s", f.name, target)
}

func (f *NumberField) Format(v any) string {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, rv.Type().Bits())
	}
	return fmt.Sprint(v)
}

func (f *NumberField) Parse(raw string, target reflect.Type) (any, string) {
	raw = strings.TrimSpace(raw)
	out := reflect.New(target).Elem()
	if raw == "" {
		if f.required {
			return nil, i18n.T("validation.required")
		}
		return out.Interface(), ""
	}
	invalid := i18n.T("validation.number")
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, target.Bits())
		if err != nil {
			return nil, invalid
		}
		out.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, target.Bits())
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, invalid
		}
		out.SetFloat(n)
	}
	return out.Interface(), ""
}

func (f *NumberField) Render(v Value) templ.Component { return numberInput(f, v) }

// ToggleField is a checkbox bound to a bool field.
type ToggleField struct {
	name  string
	label string
}

// Toggle returns a checkbox for the named model field.
func Toggle(name string) *ToggleField {
	return &ToggleField{name: name, label: humanize.Name(name)}
}

// Label overrides the default label.
func (f *ToggleField) Label(label string) *ToggleField { f.label = label; return f }

func (f *ToggleField) Info() Info { return Info{Name: f.name, Label: f.label} }

func (f *ToggleField) Check(target reflect.Type) error {
	if target.Kind() != reflect.Bool {
		return fmt.Errorf("form.Toggle(%q) needs a bool field, the model field is %s", f.name, target)
	}
	return nil
}

// Format returns "true" for a checked toggle and "" otherwise.
func (f *ToggleField) Format(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Bool && rv.Bool() {
		return "true"
	}
	return ""
}

// Parse treats an absent or empty checkbox as false, because browsers do not
// submit unchecked checkboxes.
func (f *ToggleField) Parse(raw string, target reflect.Type) (any, string) {
	out := reflect.New(target).Elem()
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "false", "0", "off":
	default:
		out.SetBool(true)
	}
	return out.Interface(), ""
}

func (f *ToggleField) Render(v Value) templ.Component { return toggleInput(f, v) }
````

Create `form/render.templ`:

````templ
package form

import "strconv"

templ fieldLabel(name, label string, required bool) {
	<label class="label" for={ "f-" + name }>
		{ label }
		if required {
			<span class="req" aria-hidden="true">*</span>
		}
	</label>
}

templ fieldError(name, message string) {
	if message != "" {
		<p class="field-error" id={ "e-" + name }>{ message }</p>
	}
}

templ textInput(f *TextField, v Value) {
	<div class="field">
		@fieldLabel(f.name, f.label, f.required)
		<input
			class="input"
			type="text"
			id={ "f-" + f.name }
			name={ f.name }
			value={ v.Raw }
			required?={ f.required }
			if f.maxLength > 0 {
				maxlength={ strconv.Itoa(f.maxLength) }
			}
			if v.Error != "" {
				aria-invalid="true"
				aria-describedby={ "e-" + f.name }
			}
		/>
		@fieldError(f.name, v.Error)
	</div>
}

templ textareaInput(f *TextField, v Value) {
	<div class="field">
		@fieldLabel(f.name, f.label, f.required)
		<textarea
			class="input"
			rows="5"
			id={ "f-" + f.name }
			name={ f.name }
			required?={ f.required }
			if f.maxLength > 0 {
				maxlength={ strconv.Itoa(f.maxLength) }
			}
			if v.Error != "" {
				aria-invalid="true"
				aria-describedby={ "e-" + f.name }
			}
		>{ v.Raw }</textarea>
		@fieldError(f.name, v.Error)
	</div>
}

templ numberInput(f *NumberField, v Value) {
	<div class="field">
		@fieldLabel(f.name, f.label, f.required)
		<input
			class="input"
			type="number"
			step="any"
			id={ "f-" + f.name }
			name={ f.name }
			value={ v.Raw }
			required?={ f.required }
			if v.Error != "" {
				aria-invalid="true"
				aria-describedby={ "e-" + f.name }
			}
		/>
		@fieldError(f.name, v.Error)
	</div>
}

templ toggleInput(f *ToggleField, v Value) {
	<div class="field field-toggle">
		<label class="check">
			<input type="checkbox" name={ f.name } value="true" checked?={ v.Raw == "true" }/>
			<span>{ f.label }</span>
		</label>
		@fieldError(f.name, v.Error)
	</div>
}
````

- [ ] **Step 5: Generate the templ code**

```bash
go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

This writes `*_templ.go` next to each `.templ` file. Commit them.

- [ ] **Step 6: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./form/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/form`.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum form
git commit -m "feat: add form fields with parsing, validation and templ rendering"
```

### Task 8: Table columns

The `table` package: `Text` and `Boolean` columns.

**Files:**
- Create: `table/column.go`
- Create: `table/render.templ`
- Test: `table/column_test.go`

**Interfaces:**
- Consumes: `humanize.Name`, `i18n.T` keys `common.yes`, `common.no`.
- Produces: `table.Info{Name, Label string; Searchable, Sortable bool}` and the `table.Column` interface with `Info() Info` and `Cell(v any) templ.Component`. `table.Text(name)` returns `*TextColumn` (`Label`, `Searchable`, `Sortable`). `table.Boolean(name)` returns `*BooleanColumn` (`Label`, `Sortable`) rendering a `badge-on` or `badge-off` pill.

- [ ] **Step 1: Write the failing test**

Create `table/column_test.go`:

````go
package table

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func cell(t *testing.T, c Column, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Cell(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTextColumnInfoAndDefaults(t *testing.T) {
	info := Text("CreatedAt").Searchable().Sortable().Info()
	if info.Name != "CreatedAt" || info.Label != "Created at" || !info.Searchable || !info.Sortable {
		t.Errorf("unexpected info: %+v", info)
	}
	plain := Text("Name").Info()
	if plain.Searchable || plain.Sortable {
		t.Errorf("flags must default to off: %+v", plain)
	}
	if got := Text("Name").Label("Title").Info().Label; got != "Title" {
		t.Errorf("label override: %q", got)
	}
}

func TestTextCellEscapesAndHandlesValues(t *testing.T) {
	col := Text("Name")
	if got := cell(t, col, "<b>x</b>"); strings.Contains(got, "<b>") {
		t.Errorf("cell was not escaped: %s", got)
	}
	if got := cell(t, col, 42); got != "42" {
		t.Errorf("int cell: %q", got)
	}
	s := "hello"
	if got := cell(t, col, &s); got != "hello" {
		t.Errorf("pointer cell: %q", got)
	}
	var nilPtr *string
	if got := cell(t, col, nilPtr); got != "" {
		t.Errorf("nil pointer cell: %q", got)
	}
	if got := cell(t, col, nil); got != "" {
		t.Errorf("nil cell: %q", got)
	}
}

func TestBooleanCell(t *testing.T) {
	col := Boolean("Active")
	if got := cell(t, col, true); !strings.Contains(got, "Yes") || !strings.Contains(got, "badge-on") {
		t.Errorf("true cell: %s", got)
	}
	if got := cell(t, col, false); !strings.Contains(got, "No") || !strings.Contains(got, "badge-off") {
		t.Errorf("false cell: %s", got)
	}
	if !Boolean("Active").Sortable().Info().Sortable {
		t.Error("Sortable flag lost")
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./table/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Text` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `table/column.go`:

````go
// Package table holds the column builders used to describe a resource's list.
package table

import (
	"fmt"
	"reflect"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/humanize"
)

// Info is the metadata every column exposes.
type Info struct {
	Name       string // Go struct field name on the model
	Label      string // column heading
	Searchable bool   // included in the list's text search
	Sortable   bool   // heading is a sort link
}

// Column is one column of a resource list.
type Column interface {
	Info() Info
	// Cell draws the model field's value.
	Cell(v any) templ.Component
}

// TextColumn shows a value as plain text.
type TextColumn struct {
	name       string
	label      string
	searchable bool
	sortable   bool
}

// Text returns a plain-text column for the named model field.
func Text(name string) *TextColumn {
	return &TextColumn{name: name, label: humanize.Name(name)}
}

// Label overrides the default heading.
func (c *TextColumn) Label(label string) *TextColumn { c.label = label; return c }

// Searchable includes the column in the list's text search.
func (c *TextColumn) Searchable() *TextColumn { c.searchable = true; return c }

// Sortable makes the heading a sort link.
func (c *TextColumn) Sortable() *TextColumn { c.sortable = true; return c }

func (c *TextColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Searchable: c.searchable, Sortable: c.sortable}
}

func (c *TextColumn) Cell(v any) templ.Component { return textCell(display(v)) }

// BooleanColumn shows a bool as a Yes or No badge.
type BooleanColumn struct {
	name     string
	label    string
	sortable bool
}

// Boolean returns a Yes/No badge column for the named bool field.
func Boolean(name string) *BooleanColumn {
	return &BooleanColumn{name: name, label: humanize.Name(name)}
}

// Label overrides the default heading.
func (c *BooleanColumn) Label(label string) *BooleanColumn { c.label = label; return c }

// Sortable makes the heading a sort link.
func (c *BooleanColumn) Sortable() *BooleanColumn { c.sortable = true; return c }

func (c *BooleanColumn) Info() Info {
	return Info{Name: c.name, Label: c.label, Sortable: c.sortable}
}

func (c *BooleanColumn) Cell(v any) templ.Component {
	rv := reflect.ValueOf(v)
	return boolCell(rv.Kind() == reflect.Bool && rv.Bool())
}

// display turns a model value into text, following pointers.
func display(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return ""
	}
	return fmt.Sprint(rv.Interface())
}
````

Create `table/render.templ`:

````templ
package table

import "github.com/TechnoVizor/tellus/internal/i18n"

templ textCell(text string) {
	{ text }
}

templ boolCell(on bool) {
	if on {
		<span class="badge badge-on">{ i18n.T("common.yes") }</span>
	} else {
		<span class="badge badge-off">{ i18n.T("common.no") }</span>
	}
}
````

- [ ] **Step 4: Generate the templ code**

```bash
go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

This writes `*_templ.go` next to each `.templ` file. Commit them.

- [ ] **Step 5: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./table/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/table`.

- [ ] **Step 6: Commit**

```bash
git add table
git commit -m "feat: add table columns"
```

### Task 9: DataSource and the GORM source

The storage seam of the panel and its GORM implementation, tested against real Postgres.

**Files:**
- Create: `datasource.go`
- Create: `gormsource.go`
- Test: `gormsource_test.go`

**Interfaces:**
- Consumes: `testdb.Open`.
- Produces: `tellus.ErrNotFound`. `tellus.ListQuery{Search string; SearchFields []string; SortField string; SortDesc bool; Page, PerPage int}`. `tellus.ListResult[T]{Items []T; Total int64}`. The `tellus.DataSource[T]` interface: `List(ctx, ListQuery) (ListResult[T], error)`, `Find(ctx, id string) (*T, error)`, `Create(ctx, *T) error`, `Update(ctx, *T) error`, `Delete(ctx, id string) error`, `ID(*T) string`. `tellus.NewGormSource[T any](db *gorm.DB) (*GormSource[T], error)` (needs a single integer or string primary key). Constants `defaultPerPage = 25`, `maxPerPage = 200`. The test file defines the shared test models `Product{ID uint; Name string; Price int; Active bool; CreatedAt time.Time}` and `Note` (soft delete), reused by later tests.

- [ ] **Step 1: Write the failing test**

Create `gormsource_test.go`:

````go
package tellus

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/internal/testdb"
)

type Product struct {
	ID        uint
	Name      string
	Price     int
	Active    bool
	CreatedAt time.Time
}

type Note struct {
	ID        uint
	Body      string
	DeletedAt gorm.DeletedAt
}

func productSource(t *testing.T, n int) (*GormSource[Product], *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		p := Product{Name: fmt.Sprintf("Product %02d", i), Price: i * 10, Active: i%2 == 0}
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	src, err := NewGormSource[Product](db)
	if err != nil {
		t.Fatal(err)
	}
	return src, db
}

func TestGormSourcePagination(t *testing.T) {
	src, _ := productSource(t, 25)
	ctx := context.Background()

	res, err := src.List(ctx, ListQuery{Page: 1, PerPage: 10})
	if err != nil || len(res.Items) != 10 || res.Total != 25 {
		t.Fatalf("page 1: len=%d total=%d err=%v", len(res.Items), res.Total, err)
	}
	res, _ = src.List(ctx, ListQuery{Page: 3, PerPage: 10})
	if len(res.Items) != 5 || res.Items[0].Name != "Product 21" {
		t.Fatalf("page 3: %+v", res.Items)
	}
	res, _ = src.List(ctx, ListQuery{Page: 9, PerPage: 10})
	if len(res.Items) != 0 || res.Total != 25 {
		t.Fatalf("page past the end: len=%d total=%d", len(res.Items), res.Total)
	}
	res, _ = src.List(ctx, ListQuery{})
	if len(res.Items) != 25 {
		t.Fatalf("zero-value query must use defaults, got %d items", len(res.Items))
	}
}

func TestGormSourceSort(t *testing.T) {
	src, _ := productSource(t, 5)
	ctx := context.Background()

	res, err := src.List(ctx, ListQuery{SortField: "Price", SortDesc: true, PerPage: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Price != 50 || res.Items[4].Price != 10 {
		t.Fatalf("descending sort wrong: %+v", res.Items)
	}
	res, _ = src.List(ctx, ListQuery{SortField: "Name", PerPage: 5})
	if res.Items[0].Name != "Product 01" {
		t.Fatalf("ascending sort wrong: %+v", res.Items)
	}
}

func TestGormSourceSearch(t *testing.T) {
	src, db := productSource(t, 12)
	ctx := context.Background()
	db.Create(&Product{Name: "100% pure"})
	db.Create(&Product{Name: "1000 pure"})
	db.Create(&Product{Name: "a_b"})
	db.Create(&Product{Name: "axb"})

	search := func(q string, fields ...string) []Product {
		t.Helper()
		res, err := src.List(ctx, ListQuery{Search: q, SearchFields: fields, PerPage: 50})
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return res.Items
	}

	if got := search("product 07", "Name"); len(got) != 1 {
		t.Errorf("case-insensitive match: %d", len(got))
	}
	if got := search("100%", "Name"); len(got) != 1 || got[0].Name != "100% pure" {
		t.Errorf("percent must be literal: %+v", got)
	}
	if got := search("a_b", "Name"); len(got) != 1 || got[0].Name != "a_b" {
		t.Errorf("underscore must be literal: %+v", got)
	}
	if got := search("120", "Name", "Price"); len(got) != 1 || got[0].Price != 120 {
		t.Errorf("numeric column must be searchable as text: %+v", got)
	}
	if got := search(`'; DROP TABLE products; --`, "Name"); len(got) != 0 {
		t.Errorf("injection string matched rows: %+v", got)
	}
	if got := search("Product", "Name"); len(got) != 12 {
		t.Errorf("table damaged or search broken: %d rows", len(got))
	}
	if got := search("anything"); len(got) != 16 {
		t.Errorf("no search fields means no filter, got %d", len(got))
	}
}

func TestGormSourceRejectsUnknownFields(t *testing.T) {
	src, _ := productSource(t, 2)
	ctx := context.Background()
	if _, err := src.List(ctx, ListQuery{SortField: "Price; DROP TABLE products"}); err == nil {
		t.Error("unknown sort field must be an error")
	}
	if _, err := src.List(ctx, ListQuery{Search: "x", SearchFields: []string{"Nope"}}); err == nil {
		t.Error("unknown search field must be an error")
	}
}

func TestGormSourceCRUD(t *testing.T) {
	src, _ := productSource(t, 0)
	ctx := context.Background()

	p := &Product{Name: "Lamp", Price: 30, Active: true}
	if err := src.Create(ctx, p); err != nil || p.ID == 0 {
		t.Fatalf("create: id=%d err=%v", p.ID, err)
	}
	id := src.ID(p)
	if id != fmt.Sprint(p.ID) {
		t.Fatalf("ID() = %q", id)
	}

	got, err := src.Find(ctx, id)
	if err != nil || got.Name != "Lamp" {
		t.Fatalf("find: %+v %v", got, err)
	}

	got.Name, got.Price, got.Active = "Desk lamp", 0, false
	if err := src.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := src.Find(ctx, id)
	if again.Name != "Desk lamp" || again.Price != 0 || again.Active {
		t.Fatalf("update must persist zero values: %+v", again)
	}

	if err := src.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("find after delete: %v", err)
	}
	if err := src.Update(ctx, got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a deleted record must be ErrNotFound, got %v", err)
	}
	var count int64
	src.db.Model(&Product{}).Count(&count)
	if count != 0 {
		t.Fatalf("update resurrected the deleted record, count=%d", count)
	}
}

func TestGormSourceBadIDs(t *testing.T) {
	src, _ := productSource(t, 1)
	ctx := context.Background()
	for _, id := range []string{"abc", "", "-1", "1.5", "99999999999999999999"} {
		if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Find(%q): want ErrNotFound, got %v", id, err)
		}
		if err := src.Delete(ctx, id); err != nil {
			t.Errorf("Delete(%q) of a missing record must not fail: %v", id, err)
		}
	}
}

func TestGormSourceSoftDelete(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Note{}); err != nil {
		t.Fatal(err)
	}
	src, err := NewGormSource[Note](db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	n := &Note{Body: "hello"}
	if err := src.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := src.Delete(ctx, src.ID(n)); err != nil {
		t.Fatal(err)
	}
	res, _ := src.List(ctx, ListQuery{})
	if res.Total != 0 {
		t.Fatalf("soft-deleted record still listed: %+v", res)
	}
	if _, err := src.Find(ctx, src.ID(n)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted record still found: %v", err)
	}
}

func TestNewGormSourceRejectsUnsupportedModels(t *testing.T) {
	db := testdb.Open(t)
	type NoKey struct{ Name string }
	type Composite struct {
		A uint `gorm:"primaryKey"`
		B uint `gorm:"primaryKey"`
	}
	type FloatKey struct {
		ID float64 `gorm:"primaryKey"`
	}
	if _, err := NewGormSource[NoKey](db); err == nil {
		t.Error("model without a primary key must be rejected")
	}
	if _, err := NewGormSource[Composite](db); err == nil {
		t.Error("composite primary key must be rejected")
	}
	if _, err := NewGormSource[FloatKey](db); err == nil {
		t.Error("float primary key must be rejected")
	}
	if _, err := NewGormSource[Product](nil); err == nil {
		t.Error("nil db must be rejected")
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test . -run 'GormSource' -v
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: NewGormSource` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `datasource.go`:

````go
package tellus

import (
	"context"
	"errors"
)

// ErrNotFound is returned by a DataSource when a record does not exist.
var ErrNotFound = errors.New("tellus: record not found")

// ListQuery describes one page of a resource list. Field names are Go struct
// field names of the model.
type ListQuery struct {
	Search       string   // text to look for; empty means no search
	SearchFields []string // fields the search looks at
	SortField    string   // empty means primary key order
	SortDesc     bool
	Page         int // 1-based
	PerPage      int
}

// ListResult is one page of records plus the total number of matches.
type ListResult[T any] struct {
	Items []T
	Total int64
}

// DataSource is where a resource reads and writes its records. GormSource is
// the built-in implementation. Ids travel as strings because they come from
// URLs.
type DataSource[T any] interface {
	List(ctx context.Context, q ListQuery) (ListResult[T], error)
	Find(ctx context.Context, id string) (*T, error) // ErrNotFound when missing
	Create(ctx context.Context, item *T) error
	Update(ctx context.Context, item *T) error // ErrNotFound when the record is gone
	Delete(ctx context.Context, id string) error
	ID(item *T) string
}
````

Create `gormsource.go`:

````go
package tellus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const (
	defaultPerPage = 25
	maxPerPage     = 200
)

// GormSource is a DataSource backed by GORM. The model needs exactly one
// primary key column of an integer or string type.
//
// ponytail: text search uses Postgres ILIKE. Use LOWER(col) LIKE LOWER(?) when
// other databases are supported.
type GormSource[T any] struct {
	db     *gorm.DB
	schema *schema.Schema
	pk     *schema.Field
}

// NewGormSource parses the model T and returns a source for it.
func NewGormSource[T any](db *gorm.DB) (*GormSource[T], error) {
	if db == nil {
		return nil, errors.New("tellus: NewGormSource needs a *gorm.DB")
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(new(T)); err != nil {
		return nil, fmt.Errorf("tellus: parse model: %w", err)
	}
	s := stmt.Schema
	if len(s.PrimaryFields) != 1 {
		return nil, fmt.Errorf("tellus: model %s needs exactly one primary key column", s.Name)
	}
	pk := s.PrimaryFields[0]
	switch k := pk.FieldType.Kind(); {
	case isIntKind(k), isUintKind(k), k == reflect.String:
	default:
		return nil, fmt.Errorf("tellus: model %s has a %s primary key, only integer and string keys are supported", s.Name, pk.FieldType)
	}
	return &GormSource[T]{db: db, schema: s, pk: pk}, nil
}

func (s *GormSource[T]) List(ctx context.Context, q ListQuery) (ListResult[T], error) {
	page, perPage := q.Page, q.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	base := s.db.WithContext(ctx).Model(new(T))
	if q.Search != "" && len(q.SearchFields) > 0 {
		pattern := "%" + escapeLike(q.Search) + "%"
		conds := make([]string, 0, len(q.SearchFields))
		args := make([]any, 0, len(q.SearchFields))
		for _, name := range q.SearchFields {
			f, err := s.column(name)
			if err != nil {
				return ListResult[T]{}, err
			}
			conds = append(conds, "CAST("+s.db.Statement.Quote(f.DBName)+` AS text) ILIKE ? ESCAPE '\'`)
			args = append(args, pattern)
		}
		base = base.Where("("+strings.Join(conds, " OR ")+")", args...)
	}

	var res ListResult[T]
	if err := base.Session(&gorm.Session{}).Count(&res.Total).Error; err != nil {
		return ListResult[T]{}, err
	}

	rows := base.Session(&gorm.Session{})
	if q.SortField != "" {
		f, err := s.column(q.SortField)
		if err != nil {
			return ListResult[T]{}, err
		}
		rows = rows.Order(clause.OrderByColumn{Column: clause.Column{Name: f.DBName}, Desc: q.SortDesc})
	}
	// Primary key as a tiebreaker keeps pages stable when sort values repeat.
	rows = rows.Order(clause.OrderByColumn{Column: clause.Column{Name: s.pk.DBName}})
	if err := rows.Limit(perPage).Offset((page - 1) * perPage).Find(&res.Items).Error; err != nil {
		return ListResult[T]{}, err
	}
	return res, nil
}

func (s *GormSource[T]) Find(ctx context.Context, id string) (*T, error) {
	key, err := s.parseID(id)
	if err != nil {
		return nil, err
	}
	// Limit(1).Find rather than First: a missing record is normal here, and First
	// would log it as an error.
	var item T
	tx := s.db.WithContext(ctx).Where(clause.Eq{Column: clause.Column{Name: s.pk.DBName}, Value: key}).Limit(1).Find(&item)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &item, nil
}

func (s *GormSource[T]) Create(ctx context.Context, item *T) error {
	return s.db.WithContext(ctx).Create(item).Error
}

// Update writes every column of item, zero values included. It never inserts:
// a record deleted in the meantime yields ErrNotFound.
func (s *GormSource[T]) Update(ctx context.Context, item *T) error {
	tx := s.db.WithContext(ctx).Model(item).Select("*").Omit(s.pk.DBName).Updates(item)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the record. Deleting a missing record is not an error.
func (s *GormSource[T]) Delete(ctx context.Context, id string) error {
	key, err := s.parseID(id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where(clause.Eq{Column: clause.Column{Name: s.pk.DBName}, Value: key}).Delete(new(T)).Error
}

func (s *GormSource[T]) ID(item *T) string {
	v, _ := s.pk.ValueOf(context.Background(), reflect.ValueOf(item))
	return fmt.Sprint(v)
}

func (s *GormSource[T]) column(name string) (*schema.Field, error) {
	f, ok := s.schema.FieldsByName[name]
	if !ok || f.DBName == "" {
		return nil, fmt.Errorf("tellus: model %s has no column for field %q", s.schema.Name, name)
	}
	return f, nil
}

// parseID converts a URL id into the primary key's Go type. An id that cannot
// be that type cannot exist, so it reports ErrNotFound.
func (s *GormSource[T]) parseID(id string) (any, error) {
	switch k := s.pk.FieldType.Kind(); {
	case isIntKind(k):
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, ErrNotFound
		}
		return n, nil
	case isUintKind(k):
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n > math.MaxInt64 {
			return nil, ErrNotFound
		}
		return int64(n), nil
	default:
		return id, nil
	}
}

func isIntKind(k reflect.Kind) bool  { return k >= reflect.Int && k <= reflect.Int64 }
func isUintKind(k reflect.Kind) bool { return k >= reflect.Uint && k <= reflect.Uint64 }

// escapeLike escapes the characters that are special in a LIKE pattern so user
// input is matched literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test . -run 'GormSource' -v
```

Expected: PASS, every `TestGormSource*` and `TestNewGormSourceRejectsUnsupportedModels` passes.

- [ ] **Step 5: Commit**

```bash
git add datasource.go gormsource.go gormsource_test.go go.mod go.sum
git commit -m "feat: add DataSource interface and GORM implementation"
```

### Task 10: Embedded assets: design, htmx, Alpine, Inter

The panel's CSS, JavaScript and fonts embedded in the binary and served with correct types and caching.

**Files:**
- Create: `internal/assets/assets.go`
- Create: `internal/assets/static/app.css`
- Create: `internal/assets/static/app.js`
- Create: `internal/assets/static/theme-init.js`
- Create: `THIRD_PARTY_NOTICES.md` (through commands below), downloaded `htmx.min.js`, `alpine.min.js`, `fonts/*.woff2`
- Test: `internal/assets/assets_test.go`

**Interfaces:**
- Consumes: Nothing.
- Produces: `assets.Handler() http.Handler` (mount it with `http.StripPrefix`; explicit content types, `nosniff`, ETag, `immutable` for `?v=<Version()>` and everything under `fonts/`, no directory listings). `assets.Version() string` is a 10-character content hash used in asset URLs. Static files: `app.css` (tokens from the Farhaven and mybee.lv look, light and dark), `app.js` (Alpine components `themeToggle` and `confirmSubmit`), `theme-init.js` (sets the saved theme before first paint), `htmx.min.js` 2.0.11, `alpine.min.js` 3.17.4, `fonts/inter-{latin,latin-ext,cyrillic}.woff2` (Inter Variable 5.3.0).

- [ ] **Step 1: Prepare**

```bash
mkdir -p internal/assets/static/fonts
curl -sSL -o internal/assets/static/htmx.min.js https://cdn.jsdelivr.net/npm/htmx.org@2.0.11/dist/htmx.min.js
curl -sSL -o internal/assets/static/alpine.min.js https://cdn.jsdelivr.net/npm/alpinejs@3.17.4/dist/cdn.min.js
for f in latin latin-ext cyrillic; do curl -sSL -o internal/assets/static/fonts/inter-$f.woff2 https://cdn.jsdelivr.net/npm/@fontsource-variable/inter@5.3.0/files/inter-$f-wght-normal.woff2; done
```

- [ ] **Step 2: Write the failing test**

Create `internal/assets/assets_test.go`:

````go
package assets

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestServesCSSAndJSWithExplicitTypes(t *testing.T) {
	css := get(t, "/app.css")
	if css.Code != http.StatusOK || !strings.HasPrefix(css.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("css: %d %q", css.Code, css.Header().Get("Content-Type"))
	}
	if !strings.Contains(css.Body.String(), ":root") {
		t.Error("css body looks wrong")
	}
	js := get(t, "/app.js")
	if js.Code != http.StatusOK || !strings.HasPrefix(js.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("js: %d %q", js.Code, js.Header().Get("Content-Type"))
	}
	if js.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
}

func TestVendoredLibrariesAreReallyEmbedded(t *testing.T) {
	for _, name := range []string{"/htmx.min.js", "/alpine.min.js"} {
		rec := get(t, name)
		if rec.Code != http.StatusOK || rec.Body.Len() < 10_000 {
			t.Errorf("%s: status %d, %d bytes", name, rec.Code, rec.Body.Len())
		}
	}
}

func TestCachingDependsOnVersionParam(t *testing.T) {
	versioned := get(t, "/app.css?v="+Version())
	if cc := versioned.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("versioned URL should be immutable, got %q", cc)
	}
	stale := get(t, "/app.css?v=old")
	if cc := stale.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("unversioned URL should be no-cache, got %q", cc)
	}
}

func TestFontsAreServedImmutableWithFontType(t *testing.T) {
	rec := get(t, "/fonts/inter-latin.woff2")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/woff2" {
		t.Fatalf("font: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("fonts should be immutable, got %q", cc)
	}
	if rec.Body.Len() < 10_000 {
		t.Errorf("font looks truncated: %d bytes", rec.Body.Len())
	}
}

func TestETagRevalidation(t *testing.T) {
	first := get(t, "/app.css")
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	req := httptest.NewRequest(http.MethodGet, "/app.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("matching If-None-Match should give 304, got %d", rec.Code)
	}
}

func TestNoDirectoryListingAndUnknownFile(t *testing.T) {
	if rec := get(t, "/"); rec.Code != http.StatusNotFound {
		t.Errorf("directory listing: %d", rec.Code)
	}
	if rec := get(t, "/nope.css"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown file: %d", rec.Code)
	}
}

func TestVersionIsShortAndStable(t *testing.T) {
	if len(Version()) != 10 || Version() != computeVersion() {
		t.Errorf("unexpected version %q", Version())
	}
}
````

- [ ] **Step 3: Run to verify it fails**

```bash
go test ./internal/assets/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Handler` or `no non-test Go files`).

- [ ] **Step 4: Write the implementation**

Create `internal/assets/assets.go`:

````go
// Package assets embeds the panel's CSS and JavaScript so a host application
// needs no CDN and no frontend build.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var static embed.FS

var version = computeVersion()

// Version is a short content hash used for cache-busting asset URLs.
func Version() string { return version }

// Handler serves the embedded files. Mount it with http.StripPrefix. Requests
// carrying the current ?v=<Version()> are cached as immutable, and so is
// everything under fonts/ (a changed font gets a new file name). Everything
// else revalidates through an ETag.
func Handler() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // no directory listings
			return
		}
		// Set explicitly: on Windows the OS registry can map .js to text/plain.
		switch path.Ext(r.URL.Path) {
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".woff2":
			w.Header().Set("Content-Type", "font/woff2")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("ETag", `"`+version+`"`)
		if r.URL.Query().Get("v") == version || strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func computeVersion() string {
	h := sha256.New()
	err := fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := static.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		h.Write(data)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}
````

Create `internal/assets/static/app.css`:

````css
/*
  Tellus panel styles. One fixed design, light and dark themes.

  The look follows the Farhaven and mybee.lv sites: monochrome ink on white,
  hairline borders, Inter, large radii and pill buttons. Light is the default;
  dark overrides the same token names.
*/
@layer reset, tokens, base, components;

@font-face {
  font-family: "Inter Variable";
  font-style: normal;
  font-display: swap;
  font-weight: 100 900;
  src: url(fonts/inter-cyrillic.woff2) format("woff2-variations");
  unicode-range: U+0301, U+0400-045F, U+0490-0491, U+04B0-04B1, U+2116;
}
@font-face {
  font-family: "Inter Variable";
  font-style: normal;
  font-display: swap;
  font-weight: 100 900;
  src: url(fonts/inter-latin-ext.woff2) format("woff2-variations");
  unicode-range: U+0100-02BA, U+02BD-02C5, U+02C7-02CC, U+02CE-02D7, U+02DD-02FF, U+0304, U+0308, U+0329, U+1D00-1DBF, U+1E00-1E9F, U+1EF2-1EFF, U+2020, U+20A0-20AB, U+20AD-20C0, U+2113, U+2C60-2C7F, U+A720-A7FF;
}
@font-face {
  font-family: "Inter Variable";
  font-style: normal;
  font-display: swap;
  font-weight: 100 900;
  src: url(fonts/inter-latin.woff2) format("woff2-variations");
  unicode-range: U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+02C6, U+02DA, U+02DC, U+0304, U+0308, U+0329, U+2000-206F, U+20AC, U+2122, U+2191, U+2193, U+2212, U+2215, U+FEFF, U+FFFD;
}

@layer reset {
  *, *::before, *::after { box-sizing: border-box; }
  body, h1, h2, h3, p, figure, dl, dd { margin: 0; }
  button, input, textarea, select { font: inherit; color: inherit; }
  table { border-collapse: collapse; width: 100%; }
}

@layer tokens {
  :root {
    color-scheme: light;
    --canvas: #ffffff;
    --card: #ffffff;
    --surface-soft: #f5f5f5;
    --surface-strong: #e5e5e5;
    --hairline: #eaeaea;
    --border-strong: #d4d4d4;
    --ink: #0a0a0a;
    --ink-hover: #262626;
    --ink-body: #404040;
    --ink-muted: #5c5c5c;
    --on-ink: #ffffff;
    --danger: #c13515;
    --radius-card: 20px;
    --radius-control: 12px;
    --radius-pill: 9999px;
    --shadow-float: 0 0 0 1px rgb(0 0 0 / 0.02), 0 2px 6px 0 rgb(0 0 0 / 0.04), 0 4px 8px 0 rgb(0 0 0 / 0.1);
    --ease: cubic-bezier(0.2, 0, 0, 1);
    --font: "Inter Variable", Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  }
  @media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
      color-scheme: dark;
      --canvas: #0b0b0c;
      --card: #151517;
      --surface-soft: #171719;
      --surface-strong: #26262a;
      --hairline: #2a2a2a;
      --border-strong: #454548;
      --ink: #f5f5f5;
      --ink-hover: #d4d4d4;
      --ink-body: #c7c7c7;
      --ink-muted: #a3a3a3;
      --on-ink: #0b0b0c;
      --danger: #f26d5b;
    }
  }
  :root[data-theme="dark"] {
    color-scheme: dark;
    --canvas: #0b0b0c;
    --card: #151517;
    --surface-soft: #171719;
    --surface-strong: #26262a;
    --hairline: #2a2a2a;
    --border-strong: #454548;
    --ink: #f5f5f5;
    --ink-hover: #d4d4d4;
    --ink-body: #c7c7c7;
    --ink-muted: #a3a3a3;
    --on-ink: #0b0b0c;
    --danger: #f26d5b;
  }
}

@layer base {
  body {
    background: var(--canvas);
    color: var(--ink);
    font: 15px/1.5 var(--font);
    -webkit-font-smoothing: antialiased;
  }
  a { color: inherit; text-decoration: underline; text-underline-offset: 3px; }
  h1, h2, h3 { letter-spacing: -0.021em; text-wrap: balance; }
  h1 { font-size: 28px; font-weight: 600; line-height: 1.15; }
  .muted { color: var(--ink-muted); }
  ::selection { background: var(--surface-strong); color: var(--ink); }
  button:not(:disabled) { cursor: pointer; }
  :where(a, button, summary, [tabindex]):focus-visible {
    outline: 2px solid var(--ink);
    outline-offset: 2px;
    border-radius: 4px;
  }
  .sr-only {
    position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0;
    overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0;
  }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after { transition: none !important; animation: none !important; }
  }
}

@layer components {
  /* layout */
  .shell { display: grid; grid-template-columns: 256px 1fr; min-height: 100vh; }
  .sidebar {
    display: flex; flex-direction: column; gap: 28px;
    padding: 20px 16px; background: var(--canvas); border-right: 1px solid var(--hairline);
    position: sticky; top: 0; height: 100vh;
  }
  .brand { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; padding: 4px 12px; }
  .nav { display: flex; flex-direction: column; gap: 2px; flex: 1; }
  .nav a {
    padding: 9px 14px; border-radius: var(--radius-pill); color: var(--ink-muted);
    font-weight: 500; text-decoration: none;
    transition: background-color 0.15s var(--ease), color 0.15s var(--ease);
  }
  .nav a:hover { background: var(--surface-soft); color: var(--ink); }
  .nav a[aria-current="page"] { background: var(--surface-soft); color: var(--ink); }
  .sidebar-foot { display: flex; flex-direction: column; gap: 8px; }
  .user { color: var(--ink-muted); font-size: 13px; padding: 0 12px; overflow-wrap: anywhere; }
  .main { padding: 40px 40px 64px; max-width: 1200px; width: 100%; }
  .page-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 24px; }

  /* buttons */
  .btn {
    display: inline-flex; align-items: center; justify-content: center; gap: 6px;
    padding: 10px 18px; border-radius: var(--radius-pill);
    border: 1px solid color-mix(in srgb, var(--ink) 30%, transparent);
    background: var(--canvas); color: var(--ink);
    font-size: 14px; font-weight: 500; line-height: 1.2; text-decoration: none;
    transition: background-color 0.15s var(--ease), border-color 0.15s var(--ease);
  }
  .btn:hover { background: var(--surface-soft); }
  .btn-primary { background: var(--ink); border-color: var(--ink); color: var(--on-ink); }
  .btn-primary:hover { background: var(--ink-hover); border-color: var(--ink-hover); }
  .btn-danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent); }
  .btn-danger:hover { background: color-mix(in srgb, var(--danger) 8%, transparent); }
  .btn-ghost { background: transparent; border-color: transparent; color: var(--ink-muted); }
  .btn-sm { padding: 6px 13px; font-size: 13px; }
  .btn-block { width: 100%; }
  .btn[aria-disabled="true"] { opacity: 0.4; pointer-events: none; }

  /* cards and messages */
  .card { background: var(--card); border: 1px solid var(--hairline); border-radius: var(--radius-card); }
  .card-soft { background: var(--surface-soft); border-radius: 24px; }
  .card-body { padding: 28px; }
  .notice, .alert {
    padding: 12px 16px; margin-bottom: 16px; border-radius: var(--radius-control);
    font-weight: 500; font-size: 14px;
  }
  .notice { background: var(--surface-soft); border: 1px solid var(--hairline); }
  .alert {
    color: var(--danger);
    background: color-mix(in srgb, var(--danger) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--danger) 30%, transparent);
  }
  .empty { padding: 64px 24px; text-align: center; color: var(--ink-muted); }

  /* forms */
  .field { display: grid; gap: 8px; margin-bottom: 20px; }
  .label { font-weight: 600; font-size: 14px; }
  .req { color: var(--danger); margin-left: 2px; }
  .input {
    width: 100%; padding: 11px 14px; border-radius: var(--radius-control);
    border: 1px solid var(--border-strong); background: var(--canvas);
    transition: border-color 0.15s var(--ease), box-shadow 0.15s var(--ease);
  }
  .input::placeholder { color: var(--ink-muted); }
  .input:focus {
    outline: none; border-color: var(--ink);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--ink) 12%, transparent);
  }
  .input[aria-invalid="true"] { border-color: var(--danger); }
  textarea.input { resize: vertical; }
  .field-error { color: var(--danger); font-size: 13px; font-weight: 500; }
  .check { display: inline-flex; align-items: center; gap: 10px; font-weight: 500; cursor: pointer; }
  .check input { width: 18px; height: 18px; accent-color: var(--ink); }
  .form-actions { display: flex; gap: 10px; margin-top: 12px; }
  .form-narrow { max-width: 640px; }

  /* list */
  .toolbar { display: flex; align-items: center; gap: 12px; padding: 16px 20px; border-bottom: 1px solid var(--hairline); }
  .toolbar .input { max-width: 340px; border-radius: var(--radius-pill); padding-inline: 18px; background: var(--surface-soft); border-color: transparent; }
  .toolbar .input:focus { background: var(--canvas); border-color: var(--ink); }
  .table-wrap { overflow-x: auto; }
  .table { font-variant-numeric: tabular-nums; }
  .table th {
    text-align: left; padding: 12px 20px; font-size: 13px; font-weight: 500;
    color: var(--ink-muted); border-bottom: 1px solid var(--hairline); white-space: nowrap;
  }
  .table th a { color: inherit; text-decoration: none; }
  .table th a:hover { color: var(--ink); }
  .table td { padding: 14px 20px; border-bottom: 1px solid var(--hairline); vertical-align: middle; }
  .table tbody tr:last-child td { border-bottom: 0; }
  .table tbody tr:hover { background: var(--surface-soft); }
  .table .actions { text-align: right; white-space: nowrap; }
  .table .actions form { display: inline; }
  .badge {
    display: inline-block; padding: 2px 11px; border-radius: var(--radius-pill);
    font-size: 12px; font-weight: 600;
  }
  .badge-on { background: var(--ink); color: var(--on-ink); }
  .badge-off { color: var(--ink-muted); border: 1px solid var(--border-strong); }
  .pager {
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
    padding: 14px 20px; color: var(--ink-muted); font-size: 13px;
    border-top: 1px solid var(--hairline);
  }
  .pager-links { display: flex; gap: 8px; }

  /* login */
  .auth { min-height: 100vh; display: grid; place-items: center; padding: 24px; }
  .auth .card-soft { width: 100%; max-width: 400px; }
  .auth .card-body { padding: 36px; }
  .auth h1 { margin-bottom: 6px; }
  .auth .lede { color: var(--ink-muted); margin-bottom: 24px; }
  .auth .btn-primary { padding-block: 13px; }

  @media (max-width: 800px) {
    .shell { grid-template-columns: 1fr; }
    .sidebar {
      position: static; height: auto; flex-direction: row; flex-wrap: wrap;
      align-items: center; gap: 12px; border-right: 0; border-bottom: 1px solid var(--hairline);
    }
    .nav { flex-direction: row; flex: 1; }
    .sidebar-foot { flex-direction: row; align-items: center; }
    .main { padding: 24px 16px 48px; }
    .card-body { padding: 20px; }
  }
}
````

Create `internal/assets/static/app.js`:

````javascript
// Alpine components used by the panel. Loaded before Alpine itself.
document.addEventListener("alpine:init", function () {
  Alpine.data("themeToggle", function () {
    return {
      toggle: function () {
        var root = document.documentElement;
        var current =
          root.dataset.theme ||
          (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
        var next = current === "dark" ? "light" : "dark";
        root.dataset.theme = next;
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
      },
    };
  });

  // Put data-confirm="message" on a form to ask before it submits.
  Alpine.data("confirmSubmit", function () {
    return {
      ask: function (event) {
        if (!window.confirm(this.$el.dataset.confirm)) event.preventDefault();
      },
    };
  });
});
````

Create `internal/assets/static/theme-init.js`:

````javascript
// Runs before first paint so a saved theme does not flash the wrong colors.
(function () {
  try {
    var t = localStorage.getItem("tellus-theme");
    if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
  } catch (e) {}
})();
````

- [ ] **Step 5: Write the third-party notices**

```bash
{
  echo "# Third-party notices"
  echo
  echo "Tellus embeds the files below in its assets. Their licenses are reproduced in full."
  echo
  echo "## htmx 2.0.11 (Zero-Clause BSD)"
  echo
  curl -sSL https://cdn.jsdelivr.net/npm/htmx.org@2.0.11/LICENSE
  echo
  echo "## Alpine.js 3.17.4 (MIT)"
  echo
  curl -sSL https://raw.githubusercontent.com/alpinejs/alpine/main/LICENSE.md
  echo
  echo "## Inter Variable 5.3.0 (SIL Open Font License 1.1)"
  echo
  curl -sSL https://raw.githubusercontent.com/rsms/inter/master/LICENSE.txt
} > THIRD_PARTY_NOTICES.md
```

- [ ] **Step 6: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/assets/
```

Expected: PASS, `ok  github.com/TechnoVizor/tellus/internal/assets`.

- [ ] **Step 7: Commit**

```bash
git add internal/assets THIRD_PARTY_NOTICES.md
git commit -m "feat: embed panel CSS, htmx, Alpine and Inter with cache-safe serving"
```

### Task 11: UI pages

The templ pages (layout, sign-in, list, form) with plain view structs, plus a guard that every UI string key exists.

**Files:**
- Create: `internal/ui/views.go`
- Create: `internal/ui/layout.templ`
- Create: `internal/ui/login.templ`
- Create: `internal/ui/list.templ`
- Create: `internal/ui/form.templ`
- Test: `internal/ui/ui_test.go`
- Test: `internal/i18n/keys_test.go`

**Interfaces:**
- Consumes: `assets.Version`, `i18n.T`.
- Produces: View structs `ui.Shell{Title, Brand, Prefix string; Nav []NavItem; UserName, CSRF string}`, `ui.NavItem{Label, Href string; Active bool}`, `ui.LoginView{Brand, Prefix, Action, Email, Error, CSRF string}`, `ui.ColumnView{Label string; Sortable bool; SortURL, SortMark, AriaSort string}`, `ui.RowView{Cells []templ.Component; EditURL, DeleteURL string}`, `ui.ListView{Shell; Heading, Notice, NewURL, Search string; Searchable bool; ListURL, Sort, Dir string; Columns []ColumnView; Rows []RowView; Total int64; Page, TotalPages int; PrevURL, NextURL string}`, `ui.FormView{Shell; Heading, Action, CancelURL, Error string; Fields []templ.Component}`. Components: `ui.Layout(Shell)` (takes children), `ui.MessagePage(Shell, heading, message string)`, `ui.LoginPage(LoginView)`, `ui.ListPage(ListView)`, `ui.Records(ListView)` (the swappable `#records` region), `ui.FormPage(FormView)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/ui_test.go`:

````go
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
		`role="search"`, `hx-target="#records"`, `hx-push-url="true"`, `name="q"`,
		`name="sort" value="Name"`, `name="dir" value="asc"`, // search keeps the sort
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
		"Save", "Cancel", `novalidate`)
}
````

Create `internal/i18n/keys_test.go`:

````go
package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryKeyUsedInSourceExists catches a typo in a key before it reaches the
// screen as a raw "list.emtpy".
func TestEveryKeyUsedInSourceExists(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`i18n\.T\("([^"]+)"\)`)

	found := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".templ") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range call.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s uses unknown i18n key %q", strings.TrimPrefix(path, root), m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 20 {
		t.Fatalf("scanned the wrong tree: only %d i18n.T calls found under %s", found, root)
	}
}

// TestNoticeKeysExist covers the keys built at run time in resource_handlers.go.
func TestNoticeKeysExist(t *testing.T) {
	for _, kind := range []string{"created", "updated", "deleted"} {
		if _, ok := en["notice."+kind]; !ok {
			t.Errorf("missing notice.%s", kind)
		}
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/ui/ ./internal/i18n/
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Shell` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `internal/ui/views.go`:

````go
// Package ui holds the templ pages of the panel and the plain view structs
// they render. It knows nothing about models or databases.
package ui

import (
	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/assets"
)

// Shell is the data every page with the panel layout needs.
type Shell struct {
	Title    string
	Brand    string
	Prefix   string // panel URL prefix without a trailing slash
	Nav      []NavItem
	UserName string
	CSRF     string
}

// NavItem is one sidebar link.
type NavItem struct {
	Label  string
	Href   string
	Active bool
}

func (s Shell) asset(name string) string {
	return s.Prefix + "/assets/" + name + "?v=" + assets.Version()
}

// LoginView is the data of the sign-in page.
type LoginView struct {
	Brand  string
	Prefix string
	Action string
	Email  string
	Error  string
	CSRF   string
}

func (v LoginView) asset(name string) string {
	return v.Prefix + "/assets/" + name + "?v=" + assets.Version()
}

// ColumnView is one list heading.
type ColumnView struct {
	Label    string
	Sortable bool
	SortURL  string // link that sorts by this column
	SortMark string // "▲", "▼" or "" for the active sort
	AriaSort string // "ascending", "descending" or "" when not the active sort
}

// RowView is one list row.
type RowView struct {
	Cells     []templ.Component
	EditURL   string
	DeleteURL string
}

// ListView is the data of a resource list page and of its #records region.
type ListView struct {
	Shell      Shell
	Heading    string
	Notice     string
	NewURL     string
	Search     string
	Searchable bool
	ListURL    string // base URL of the list, used by the search form
	Sort       string // current sort field name, "" for none
	Dir        string // "asc" or "desc"
	Columns    []ColumnView
	Rows       []RowView
	Total      int64
	Page       int
	TotalPages int
	PrevURL    string // empty when there is no previous page
	NextURL    string // empty when there is no next page
}

// FormView is the data of the create and edit page.
type FormView struct {
	Shell     Shell
	Heading   string
	Action    string
	CancelURL string
	Error     string // summary shown above the fields, empty when none
	Fields    []templ.Component
}
````

Create `internal/ui/layout.templ`:

````templ
package ui

import "github.com/TechnoVizor/tellus/internal/i18n"

templ head(title, brand string, asset func(string) string) {
	<head>
		<meta charset="utf-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<meta name="robots" content="noindex, nofollow"/>
		<title>{ title } | { brand }</title>
		<script src={ asset("theme-init.js") }></script>
		<link rel="stylesheet" href={ asset("app.css") }/>
		<script src={ asset("app.js") } defer></script>
		<script src={ asset("htmx.min.js") } defer></script>
		<script src={ asset("alpine.min.js") } defer></script>
	</head>
}

templ themeToggle() {
	<button type="button" class="btn btn-ghost btn-sm" x-data="themeToggle" x-on:click="toggle()" aria-label={ i18n.T("ui.toggle_theme") }>
		<span aria-hidden="true">◐</span> { i18n.T("ui.theme") }
	</button>
}

// MessagePage is a panel page with a heading and one line of text.
templ MessagePage(s Shell, heading, message string) {
	@Layout(s) {
		<div class="page-head">
			<h1>{ heading }</h1>
		</div>
		<p class="muted">{ message }</p>
	}
}

// Layout is the panel frame: sidebar navigation around the page content.
templ Layout(s Shell) {
	<!DOCTYPE html>
	<html lang="en">
		@head(s.Title, s.Brand, s.asset)
		<body>
			<div class="shell">
				<aside class="sidebar">
					<div class="brand">{ s.Brand }</div>
					<nav class="nav" aria-label={ i18n.T("ui.navigation") }>
						for _, item := range s.Nav {
							<a
								href={ templ.SafeURL(item.Href) }
								if item.Active {
									aria-current="page"
								}
							>{ item.Label }</a>
						}
					</nav>
					<div class="sidebar-foot">
						@themeToggle()
						<div class="user">{ s.UserName }</div>
						<form method="post" action={ templ.SafeURL(s.Prefix + "/logout") }>
							<input type="hidden" name="_csrf" value={ s.CSRF }/>
							<button type="submit" class="btn btn-sm btn-block">{ i18n.T("ui.sign_out") }</button>
						</form>
					</div>
				</aside>
				<main class="main">
					{ children... }
				</main>
			</div>
		</body>
	</html>
}
````

Create `internal/ui/login.templ`:

````templ
package ui

import "github.com/TechnoVizor/tellus/internal/i18n"

templ LoginPage(v LoginView) {
	<!DOCTYPE html>
	<html lang="en">
		@head(i18n.T("login.title"), v.Brand, v.asset)
		<body>
			<main class="auth">
				<div class="card">
					<div class="card-body">
						<h1>{ v.Brand }</h1>
						<p class="lede">{ i18n.T("login.lede") }</p>
						if v.Error != "" {
							<p class="alert" role="alert">{ v.Error }</p>
						}
						<form method="post" action={ templ.SafeURL(v.Action) }>
							<input type="hidden" name="_csrf" value={ v.CSRF }/>
							<div class="field">
								<label class="label" for="email">{ i18n.T("login.email") }</label>
								<input class="input" type="email" id="email" name="email" value={ v.Email } autocomplete="username" required autofocus/>
							</div>
							<div class="field">
								<label class="label" for="password">{ i18n.T("login.password") }</label>
								<input class="input" type="password" id="password" name="password" autocomplete="current-password" required/>
							</div>
							<button type="submit" class="btn btn-primary btn-block">{ i18n.T("login.submit") }</button>
						</form>
					</div>
				</div>
			</main>
		</body>
	</html>
}
````

Create `internal/ui/list.templ`:

````templ
package ui

import (
	"fmt"

	"github.com/TechnoVizor/tellus/internal/i18n"
)

// ListPage is a full resource list page.
templ ListPage(v ListView) {
	@Layout(v.Shell) {
		<div class="page-head">
			<h1>{ v.Heading }</h1>
			<a class="btn btn-primary" href={ templ.SafeURL(v.NewURL) }>{ i18n.T("list.new") }</a>
		</div>
		if v.Notice != "" {
			<p class="notice" role="status">{ v.Notice }</p>
		}
		@Records(v)
	}
}

// Records is the swappable region: search box, table and pager. htmx replaces
// it as a whole when the user searches, sorts or changes page.
templ Records(v ListView) {
	<div id="records" class="card">
		if v.Searchable {
			<form
				class="toolbar"
				role="search"
				method="get"
				action={ templ.SafeURL(v.ListURL) }
				hx-get={ v.ListURL }
				hx-target="#records"
				hx-swap="outerHTML"
				hx-push-url="true"
				hx-trigger="input changed delay:300ms from:find input, submit"
			>
				<input
					class="input"
					type="search"
					name="q"
					value={ v.Search }
					placeholder={ i18n.T("list.search") }
					aria-label={ i18n.T("list.search") }
				/>
				if v.Sort != "" {
					<input type="hidden" name="sort" value={ v.Sort }/>
					<input type="hidden" name="dir" value={ v.Dir }/>
				}
			</form>
		}
		if len(v.Rows) == 0 {
			<div class="empty">{ i18n.T("list.empty") }</div>
		} else {
			<div class="table-wrap">
				<table class="table">
					<thead>
						<tr>
							for _, col := range v.Columns {
								<th
									scope="col"
									if col.AriaSort != "" {
										aria-sort={ col.AriaSort }
									}
								>
									if col.Sortable {
										<a
											href={ templ.SafeURL(col.SortURL) }
											hx-get={ col.SortURL }
											hx-target="#records"
											hx-swap="outerHTML"
											hx-push-url="true"
										>{ col.Label } <span aria-hidden="true">{ col.SortMark }</span></a>
									} else {
										{ col.Label }
									}
								</th>
							}
							<th scope="col"><span class="sr-only">{ i18n.T("list.actions") }</span></th>
						</tr>
					</thead>
					<tbody>
						for _, row := range v.Rows {
							<tr>
								for _, cell := range row.Cells {
									<td>@cell</td>
								}
								<td class="actions">
									<a class="btn btn-sm" href={ templ.SafeURL(row.EditURL) }>{ i18n.T("list.edit") }</a>
									<form
										method="post"
										action={ templ.SafeURL(row.DeleteURL) }
										data-confirm={ i18n.T("list.confirm_delete") }
										x-data="confirmSubmit"
										x-on:submit="ask($event)"
									>
										<input type="hidden" name="_csrf" value={ v.Shell.CSRF }/>
										<button type="submit" class="btn btn-sm btn-danger">{ i18n.T("list.delete") }</button>
									</form>
								</td>
							</tr>
						}
					</tbody>
				</table>
			</div>
		}
		<div class="pager">
			<span>{ fmt.Sprintf(i18n.T("list.summary"), v.Total, v.Page, v.TotalPages) }</span>
			<span class="pager-links">
				@pagerLink(v.PrevURL, i18n.T("list.previous"))
				@pagerLink(v.NextURL, i18n.T("list.next"))
			</span>
		</div>
	</div>
}

templ pagerLink(href, label string) {
	if href == "" {
		<span class="btn btn-sm" aria-disabled="true">{ label }</span>
	} else {
		<a
			class="btn btn-sm"
			href={ templ.SafeURL(href) }
			hx-get={ href }
			hx-target="#records"
			hx-swap="outerHTML"
			hx-push-url="true"
		>{ label }</a>
	}
}
````

Create `internal/ui/form.templ`:

````templ
package ui

import "github.com/TechnoVizor/tellus/internal/i18n"

// FormPage is the create and edit page.
templ FormPage(v FormView) {
	@Layout(v.Shell) {
		<div class="page-head">
			<h1>{ v.Heading }</h1>
		</div>
		<div class="card form-narrow">
			<div class="card-body">
				if v.Error != "" {
					<p class="alert" role="alert">{ v.Error }</p>
				}
				<form method="post" action={ templ.SafeURL(v.Action) } novalidate>
					<input type="hidden" name="_csrf" value={ v.Shell.CSRF }/>
					for _, field := range v.Fields {
						@field
					}
					<div class="form-actions">
						<button type="submit" class="btn btn-primary">{ i18n.T("form.save") }</button>
						<a class="btn" href={ templ.SafeURL(v.CancelURL) }>{ i18n.T("form.cancel") }</a>
					</div>
				</form>
			</div>
		</div>
	}
}
````

- [ ] **Step 4: Generate the templ code**

```bash
go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
```

This writes `*_templ.go` next to each `.templ` file. Commit them.

- [ ] **Step 5: Tidy modules, run tests to verify they pass**

```bash
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test ./internal/ui/ ./internal/i18n/
```

Expected: PASS, `ok` for both packages.

- [ ] **Step 6: Commit**

```bash
git add internal/ui internal/i18n
git commit -m "feat: add templ pages for layout, sign-in, list and form"
```

### Task 12: Accounts and the Authenticator interface

The `User` and `Authenticator` contracts and the built-in GORM accounts.

**Files:**
- Create: `auth.go`
- Create: `auth_gorm.go`
- Test: `auth_gorm_test.go`

**Interfaces:**
- Consumes: `secure.HashPassword`, `secure.VerifyPassword`, `secure.DummyVerify`, `testdb.Open`.
- Produces: `tellus.User` (`UserID() string`, `DisplayName() string`, `CanAccessPanel() bool`). `tellus.Authenticator` (`Authenticate(ctx, email, password string) (User, error)` returning `ErrInvalidCredentials`, `UserByID(ctx, id string) (User, error)` returning `ErrUserNotFound`). `tellus.AdminUser` (table `tellus_users`). `tellus.NewGormAuthenticator(db *gorm.DB) *GormAuthenticator` with `Migrate() error` and `CreateUser(ctx, email, name, password string) error` (email lowercased, unique; password at least 8 characters).

- [ ] **Step 1: Write the failing test**

Create `auth_gorm_test.go`:

````go
package tellus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TechnoVizor/tellus/internal/testdb"
)

func newGormAuth(t *testing.T) *GormAuthenticator {
	t.Helper()
	auth := NewGormAuthenticator(testdb.Open(t))
	if err := auth.Migrate(); err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestGormAuthenticatorSignIn(t *testing.T) {
	auth := newGormAuth(t)
	ctx := context.Background()
	if err := auth.CreateUser(ctx, "  Admin@Example.com ", "Ada", "correct horse"); err != nil {
		t.Fatal(err)
	}

	u, err := auth.Authenticate(ctx, "admin@example.COM", "correct horse")
	if err != nil {
		t.Fatalf("valid sign-in failed: %v", err)
	}
	if u.DisplayName() != "Ada" || !u.CanAccessPanel() {
		t.Errorf("unexpected user: %+v", u)
	}

	if _, err := auth.Authenticate(ctx, "admin@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := auth.Authenticate(ctx, "nobody@example.com", "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email must look like a wrong password: %v", err)
	}

	byID, err := auth.UserByID(ctx, u.UserID())
	if err != nil || byID.UserID() != u.UserID() {
		t.Errorf("UserByID: %v %v", byID, err)
	}
	for _, bad := range []string{"", "abc", "999999", "-1"} {
		if _, err := auth.UserByID(ctx, bad); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("UserByID(%q): want ErrUserNotFound, got %v", bad, err)
		}
	}
}

func TestGormAuthenticatorStoresHashNotPassword(t *testing.T) {
	auth := newGormAuth(t)
	if err := auth.CreateUser(context.Background(), "a@b.co", "", "correct horse"); err != nil {
		t.Fatal(err)
	}
	var u AdminUser
	if err := auth.db.First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u.PasswordHash, "correct horse") || !strings.HasPrefix(u.PasswordHash, "$argon2id$") {
		t.Errorf("password not hashed: %q", u.PasswordHash)
	}
	if u.DisplayName() != "a@b.co" {
		t.Errorf("display name should fall back to the email, got %q", u.DisplayName())
	}
}

func TestCreateUserValidation(t *testing.T) {
	auth := newGormAuth(t)
	ctx := context.Background()
	if err := auth.CreateUser(ctx, "not-an-email", "", "correct horse"); err == nil {
		t.Error("invalid email accepted")
	}
	if err := auth.CreateUser(ctx, "a@b.co", "", "short"); err == nil {
		t.Error("short password accepted")
	}
	if err := auth.CreateUser(ctx, "a@b.co", "", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := auth.CreateUser(ctx, "A@B.CO", "", "correct horse"); err == nil {
		t.Error("duplicate email (different case) accepted")
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test . -run 'GormAuthenticator|CreateUser' -v
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: NewGormAuthenticator` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `auth.go`:

````go
package tellus

import (
	"context"
	"errors"
)

// User is a person signed in to the panel.
type User interface {
	UserID() string // stable id stored in the session
	DisplayName() string
	CanAccessPanel() bool
}

// Authenticator checks credentials and loads users. NewGormAuthenticator is the
// built-in implementation; a host application with its own accounts implements
// this interface instead.
type Authenticator interface {
	// Authenticate returns ErrInvalidCredentials for an unknown email or a wrong
	// password. Both must look identical to the caller.
	Authenticate(ctx context.Context, email, password string) (User, error)
	// UserByID returns ErrUserNotFound when the account no longer exists.
	UserByID(ctx context.Context, id string) (User, error)
}

var (
	// ErrInvalidCredentials is returned by Authenticate for a failed sign-in.
	ErrInvalidCredentials = errors.New("tellus: invalid credentials")
	// ErrUserNotFound is returned by UserByID for an unknown account.
	ErrUserNotFound = errors.New("tellus: user not found")
)
````

Create `auth_gorm.go`:

````go
package tellus

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/internal/secure"
)

// AdminUser is an account of the built-in authenticator, stored in the
// tellus_users table.
type AdminUser struct {
	ID           uint
	Email        string `gorm:"uniqueIndex;not null"`
	Name         string
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
}

func (AdminUser) TableName() string { return "tellus_users" }

func (u *AdminUser) UserID() string { return strconv.FormatUint(uint64(u.ID), 10) }

func (u *AdminUser) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

func (u *AdminUser) CanAccessPanel() bool { return true }

// GormAuthenticator is the built-in Authenticator: email and argon2id password
// hash in the tellus_users table.
type GormAuthenticator struct{ db *gorm.DB }

// NewGormAuthenticator returns the built-in authenticator. Call Migrate once to
// create its table and CreateUser to add the first account.
func NewGormAuthenticator(db *gorm.DB) *GormAuthenticator { return &GormAuthenticator{db: db} }

// Migrate creates or updates the tellus_users table.
func (a *GormAuthenticator) Migrate() error { return a.db.AutoMigrate(&AdminUser{}) }

// CreateUser adds an account. Emails are stored lowercased and must be unique.
func (a *GormAuthenticator) CreateUser(ctx context.Context, email, name, password string) error {
	email = normalizeEmail(email)
	if !strings.Contains(email, "@") {
		return errors.New("tellus: invalid email")
	}
	if len(password) < 8 {
		return errors.New("tellus: password must be at least 8 characters")
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return err
	}
	return a.db.WithContext(ctx).Create(&AdminUser{Email: email, Name: name, PasswordHash: hash}).Error
}

func (a *GormAuthenticator) Authenticate(ctx context.Context, email, password string) (User, error) {
	var u AdminUser
	tx := a.db.WithContext(ctx).Where("email = ?", normalizeEmail(email)).Limit(1).Find(&u)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		secure.DummyVerify(password) // same cost as a real check, so timing does not reveal unknown emails
		return nil, ErrInvalidCredentials
	}
	ok, err := secure.VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

func (a *GormAuthenticator) UserByID(ctx context.Context, id string) (User, error) {
	n, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return nil, ErrUserNotFound
	}
	var u AdminUser
	tx := a.db.WithContext(ctx).Where("id = ?", n).Limit(1).Find(&u)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test . -run 'GormAuthenticator|CreateUser' -v
```

Expected: PASS, `TestGormAuthenticatorSignIn`, `TestGormAuthenticatorStoresHashNotPassword` and `TestCreateUserValidation` pass.

- [ ] **Step 5: Commit**

```bash
git add auth.go auth_gorm.go auth_gorm_test.go
git commit -m "feat: add Authenticator interface and built-in GORM accounts"
```

### Task 13: Panel core and sign-in flow

`tellus.New`, the panel handler with security headers, CSRF, body limit, sessions, sign-in, sign-out and the home page.

**Files:**
- Create: `panel.go`
- Create: `login.go`
- Test: `helpers_test.go`
- Test: `panel_test.go`

**Interfaces:**
- Consumes: `secure.*`, `assets.Handler`, `ui.*`, `tellus.Authenticator`, `i18n.T`.
- Produces: `tellus.Config{Authenticator Authenticator; SessionSecret []byte; Prefix, Name string; SessionTTL time.Duration; InsecureCookies bool}`. `tellus.New(cfg Config) (*Panel, error)`. `(*Panel).Prefix() string`, `Register(...Registrable) error`, `Handler() http.Handler` (strips the prefix itself), `Mount(mux *http.ServeMux)`. `tellus.Registrable` (unexported method `register(p *Panel) error`, implemented by the resource builder in the next tasks). Internal seams used by resources: fields `nav []navEntry`, `slugs map[string]bool`, `mounters []func(*http.ServeMux)`, methods `authed(authedHandler) http.HandlerFunc`, `render(w, r, status, templ.Component)`, `serverError(w, err)`, `shell(r, u User, title, activeSlug string) ui.Shell`, `renderMessage(...)`, `url(path string) string`. Test helpers: `newHarness(t, cfg, regs...)` (real `httptest` server, cookie jar, redirects not followed) with `get`, `post` (adds the CSRF token), `postRaw`, `login`, `loginAdmin`, `csrfToken`, `sessionCookie`; `memAuth` (users `admin@example.com` and `guest@example.com`, password `correct horse`).

- [ ] **Step 1: Write the failing tests**

Create `helpers_test.go`:

````go
package tellus

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// --- fake authenticator ---

type memUser struct {
	id        string
	name      string
	canAccess bool
}

func (u memUser) UserID() string       { return u.id }
func (u memUser) DisplayName() string  { return u.name }
func (u memUser) CanAccessPanel() bool { return u.canAccess }

type memAccount struct {
	password string
	user     memUser
}

// memAuth is an in-memory Authenticator keyed by email.
type memAuth struct{ accounts map[string]memAccount }

func newMemAuth() *memAuth {
	return &memAuth{accounts: map[string]memAccount{
		"admin@example.com": {password: "correct horse", user: memUser{id: "1", name: "Ada", canAccess: true}},
		"guest@example.com": {password: "correct horse", user: memUser{id: "2", name: "Guest", canAccess: false}},
	}}
}

func (a *memAuth) Authenticate(_ context.Context, email, password string) (User, error) {
	acc, ok := a.accounts[strings.ToLower(email)]
	if !ok || acc.password != password {
		return nil, ErrInvalidCredentials
	}
	return acc.user, nil
}

func (a *memAuth) UserByID(_ context.Context, id string) (User, error) {
	for _, acc := range a.accounts {
		if acc.user.id == id {
			return acc.user, nil
		}
	}
	return nil, ErrUserNotFound
}

// --- HTTP harness ---

const testSecret = "0123456789abcdef0123456789abcdef"

type result struct {
	status int
	body   string
	header http.Header
}

func (r result) location() string { return r.header.Get("Location") }

type harness struct {
	t      *testing.T
	panel  *Panel
	srv    *httptest.Server
	client *http.Client
	base   string
}

// newHarness mounts a panel with the fake authenticator on a real test server.
// Redirects are not followed so tests can assert on them.
func newHarness(t *testing.T, cfg Config, regs ...Registrable) *harness {
	t.Helper()
	if cfg.Authenticator == nil {
		cfg.Authenticator = newMemAuth()
	}
	if cfg.SessionSecret == nil {
		cfg.SessionSecret = []byte(testSecret)
	}
	cfg.InsecureCookies = true // httptest serves plain http
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Register(regs...); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	p.Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	// A private transport per harness: the shared default transport could hand a
	// later test a pooled connection to a closed server that reused its port.
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Jar:           jar,
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &harness{t: t, panel: p, srv: srv, client: client, base: srv.URL + p.Prefix()}
}

func (h *harness) do(req *http.Request) result {
	h.t.Helper()
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return result{status: resp.StatusCode, body: string(body), header: resp.Header}
}

func (h *harness) get(path string, headers ...string) result {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.base+path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return h.do(req)
}

// csrfToken returns the CSRF token for this client, fetching one first if the
// jar has none.
func (h *harness) csrfToken() string {
	h.t.Helper()
	u, _ := url.Parse(h.base)
	find := func() string {
		for _, c := range h.client.Jar.Cookies(u) {
			if c.Name == "tellus_csrf" {
				return c.Value
			}
		}
		return ""
	}
	if tok := find(); tok != "" {
		return tok
	}
	h.get("/login")
	return find()
}

// post submits a form with a valid CSRF token.
func (h *harness) post(path string, v url.Values, headers ...string) result {
	h.t.Helper()
	if v == nil {
		v = url.Values{}
	}
	v.Set("_csrf", h.csrfToken())
	return h.postRaw(path, v, headers...)
}

// postRaw submits a form exactly as given.
func (h *harness) postRaw(path string, v url.Values, headers ...string) result {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.base+path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return h.do(req)
}

func (h *harness) login(email, password string) result {
	h.t.Helper()
	return h.post("/login", url.Values{"email": {email}, "password": {password}})
}

func (h *harness) loginAdmin() {
	h.t.Helper()
	if res := h.login("admin@example.com", "correct horse"); res.status != http.StatusSeeOther {
		h.t.Fatalf("admin login failed: %d %s", res.status, res.body)
	}
}

func (h *harness) sessionCookie() *http.Cookie {
	u, _ := url.Parse(h.base)
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == "tellus_session" {
			return c
		}
	}
	return nil
}
````

Create `panel_test.go`:

````go
package tellus

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// stubResource is a Registrable that does nothing, so panel behavior can be
// tested without a real resource.
type stubResource struct{ err error }

func (s stubResource) register(*Panel) error { return s.err }

func TestNewValidatesConfig(t *testing.T) {
	good := Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)}

	cases := map[string]func(*Config){
		"missing authenticator": func(c *Config) { c.Authenticator = nil },
		"short secret":          func(c *Config) { c.SessionSecret = []byte("short") },
		"prefix with query":     func(c *Config) { c.Prefix = "/admin?x=1" },
		"prefix with space":     func(c *Config) { c.Prefix = "/ad min" },
		"prefix with dotdot":    func(c *Config) { c.Prefix = "/../admin" },
		"prefix with dot":       func(c *Config) { c.Prefix = "/./admin" },
		"prefix double slash":   func(c *Config) { c.Prefix = "/a//b" },
	}
	for name, mutate := range cases {
		cfg := good
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	p, err := New(good)
	if err != nil || p.Prefix() != "/admin" {
		t.Fatalf("defaults: prefix=%q err=%v", p.Prefix(), err)
	}
	for in, want := range map[string]string{"panel": "/panel", "/x7f3/": "/x7f3", "/a/b": "/a/b"} {
		cfg := good
		cfg.Prefix = in
		p, err := New(cfg)
		if err != nil || p.Prefix() != want {
			t.Errorf("prefix %q: got %q err=%v, want %q", in, p.Prefix(), err, want)
		}
	}
}

func TestRegisterOrdering(t *testing.T) {
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret)})
	if err := p.Register(stubResource{}); err != nil {
		t.Fatalf("Register before Handler: %v", err)
	}
	if err := p.Register(stubResource{err: errors.New("bad resource")}); err == nil {
		t.Error("an error from a resource must be returned")
	}
	p.Handler()
	if err := p.Register(stubResource{}); err == nil {
		t.Error("Register after Handler must fail")
	}
}

func TestUnauthenticatedRequestsGoToLogin(t *testing.T) {
	h := newHarness(t, Config{})

	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/admin/login" {
		t.Fatalf("home: %d %q", res.status, res.location())
	}
	// An htmx request must not get a redirect that would swap the login page
	// into a region of the current page.
	res = h.get("/", "HX-Request", "true")
	if res.status != http.StatusUnauthorized || res.header.Get("HX-Redirect") != "/admin/login" {
		t.Fatalf("htmx: %d HX-Redirect=%q", res.status, res.header.Get("HX-Redirect"))
	}
}

func TestLoginPageAndSecurityHeaders(t *testing.T) {
	h := newHarness(t, Config{Name: "Acme Admin"})
	res := h.get("/login")
	if res.status != http.StatusOK {
		t.Fatalf("status %d", res.status)
	}
	for _, want := range []string{"Acme Admin", `name="email"`, `name="password"`, `name="_csrf"`, "/admin/assets/app.css?v="} {
		if !strings.Contains(res.body, want) {
			t.Errorf("login page missing %q", want)
		}
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy":         "same-origin",
		"Cache-Control":           "no-store",
	} {
		if got := res.header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestAssetsAreServedWithoutLogin(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.get("/assets/app.css")
	if res.status != http.StatusOK || !strings.Contains(res.body, ":root") {
		t.Fatalf("app.css: %d", res.status)
	}
	if !strings.HasPrefix(res.header.Get("Content-Type"), "text/css") {
		t.Errorf("content type %q", res.header.Get("Content-Type"))
	}
}

func TestLoginSuccessAndSessionCookie(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.login("Admin@Example.com", "correct horse")
	if res.status != http.StatusSeeOther || res.location() != "/admin/" {
		t.Fatalf("login: %d %q", res.status, res.location())
	}
	if h.sessionCookie() == nil {
		t.Fatal("no session cookie")
	}
	if res = h.get("/"); res.status != http.StatusOK || !strings.Contains(res.body, "Ada") {
		t.Fatalf("home after login: %d", res.status)
	}
	// The login page is pointless once signed in.
	if res = h.get("/login"); res.status != http.StatusSeeOther {
		t.Errorf("signed-in user on /login: %d", res.status)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	// Secure by default: drive the handler directly so the http test server does
	// not matter.
	p, _ := New(Config{Authenticator: newMemAuth(), SessionSecret: []byte(testSecret), Prefix: "/x7f3"})
	h := p.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x7f3/login", nil))
	var csrf *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "tellus_csrf" {
			csrf = c
		}
	}
	if csrf == nil || !csrf.Secure || !csrf.HttpOnly || csrf.Path != "/x7f3" {
		t.Fatalf("csrf cookie: %+v", csrf)
	}

	form := url.Values{"email": {"admin@example.com"}, "password": {"correct horse"}, "_csrf": {csrf.Value}}
	req := httptest.NewRequest(http.MethodPost, "/x7f3/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "tellus_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatalf("no session cookie, status %d", rec.Code)
	}
	if !session.Secure || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Path != "/x7f3" {
		t.Errorf("session cookie attributes: %+v", session)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	h := newHarness(t, Config{})
	wrong := h.login("admin@example.com", "wrong")
	unknown := h.login("nobody@example.com", "correct horse")
	noAccess := h.login("guest@example.com", "correct horse")

	for name, res := range map[string]result{"wrong password": wrong, "unknown email": unknown, "no panel access": noAccess} {
		if res.status != http.StatusUnauthorized {
			t.Errorf("%s: status %d", name, res.status)
		}
		if !strings.Contains(res.body, "Invalid email or password.") {
			t.Errorf("%s: missing the generic message", name)
		}
		if h.sessionCookie() != nil {
			t.Fatalf("%s: a session was issued", name)
		}
	}
}

func TestLoginKeepsEmailAndEscapesIt(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.login(`"><script>alert(1)</script>@x.co`, "nope")
	if strings.Contains(res.body, "<script>alert(1)</script>") {
		t.Fatal("email was reflected without escaping")
	}
}

func TestLoginRequiresCSRFToken(t *testing.T) {
	h := newHarness(t, Config{})
	res := h.postRaw("/login", url.Values{"email": {"admin@example.com"}, "password": {"correct horse"}})
	if res.status != http.StatusForbidden {
		t.Fatalf("login without CSRF token: %d", res.status)
	}
	if h.sessionCookie() != nil {
		t.Fatal("session issued without a CSRF token")
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	h := newHarness(t, Config{})
	for i := 0; i < loginAttempts; i++ {
		if res := h.login("admin@example.com", "wrong"); res.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, res.status)
		}
	}
	res := h.login("admin@example.com", "correct horse")
	if res.status != http.StatusTooManyRequests || !strings.Contains(res.body, "Too many attempts") {
		t.Fatalf("attempt over the limit: %d", res.status)
	}
	if h.sessionCookie() != nil {
		t.Fatal("a blocked attempt must not sign in, even with the right password")
	}
}

func TestSessionExpiresAndRejectsTampering(t *testing.T) {
	h := newHarness(t, Config{SessionTTL: time.Hour})
	h.loginAdmin()
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatalf("signed-in home: %d", res.status)
	}

	// Tampered cookie value.
	u, _ := url.Parse(h.base)
	c := h.sessionCookie()
	forged := &http.Cookie{Name: c.Name, Value: "A" + c.Value[1:], Path: "/admin"}
	h.client.Jar.SetCookies(u, []*http.Cookie{forged})
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("tampered cookie accepted: %d", res.status)
	}

	// Expired session.
	h.client.Jar.SetCookies(u, []*http.Cookie{{Name: c.Name, Value: c.Value, Path: "/admin"}})
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatalf("restored cookie should work: %d", res.status)
	}
	h.panel.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("expired session accepted: %d", res.status)
	}
}

func TestDeletedAccountLosesAccess(t *testing.T) {
	auth := newMemAuth()
	h := newHarness(t, Config{Authenticator: auth})
	h.loginAdmin()
	delete(auth.accounts, "admin@example.com")
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("deleted account still has access: %d", res.status)
	}
}

func TestAccountWithoutPanelAccessIsForbidden(t *testing.T) {
	auth := newMemAuth()
	h := newHarness(t, Config{Authenticator: auth})
	h.loginAdmin()
	acc := auth.accounts["admin@example.com"]
	acc.user.canAccess = false
	auth.accounts["admin@example.com"] = acc
	if res := h.get("/"); res.status != http.StatusForbidden {
		t.Errorf("revoked access: %d", res.status)
	}
}

func TestLogoutNeedsCSRFAndEndsSession(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()

	if res := h.postRaw("/logout", url.Values{}); res.status != http.StatusForbidden {
		t.Fatalf("logout without CSRF token: %d", res.status)
	}
	if res := h.get("/"); res.status != http.StatusOK {
		t.Fatal("a rejected logout must not end the session")
	}

	res := h.post("/logout", nil)
	if res.status != http.StatusSeeOther || res.location() != "/admin/login" {
		t.Fatalf("logout: %d %q", res.status, res.location())
	}
	if res := h.get("/"); res.status != http.StatusSeeOther {
		t.Errorf("still signed in after logout: %d", res.status)
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	h := newHarness(t, Config{})
	big := url.Values{"email": {"a@b.co"}, "password": {strings.Repeat("x", 2<<20)}}
	res := h.post("/login", big)
	if res.status < 400 {
		t.Fatalf("2 MiB body accepted: %d", res.status)
	}
	if h.sessionCookie() != nil {
		t.Fatal("session issued for an oversized body")
	}
}

func TestCustomPrefixMovesEverything(t *testing.T) {
	h := newHarness(t, Config{Prefix: "/secret-x7f3"})
	if !strings.HasSuffix(h.base, "/secret-x7f3") {
		t.Fatalf("base %s", h.base)
	}
	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/secret-x7f3/login" {
		t.Fatalf("redirect: %d %q", res.status, res.location())
	}
	page := h.get("/login")
	if !strings.Contains(page.body, "/secret-x7f3/assets/app.css") || strings.Contains(page.body, `"/admin/`) {
		t.Errorf("links do not follow the prefix: %s", page.body)
	}
	// The default path must not answer.
	resp, err := h.client.Get(h.srv.URL + "/admin/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/admin still answers: %d", resp.StatusCode)
	}
}

func TestHomeWithoutResources(t *testing.T) {
	h := newHarness(t, Config{})
	h.loginAdmin()
	res := h.get("/")
	if res.status != http.StatusOK || !strings.Contains(res.body, "No resources are registered yet.") {
		t.Fatalf("empty home: %d", res.status)
	}
}
````

- [ ] **Step 2: Run to verify it fails**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test . -v -run 'TestNew|TestRegister|TestUnauth|TestLogin|TestAssets|TestSession|TestDeleted|TestAccount|TestLogout|TestOversized|TestCustomPrefix|TestHome'
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Config` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `panel.go`:

````go
package tellus

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/assets"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
	"github.com/TechnoVizor/tellus/internal/ui"
)

const (
	sessionCookie  = "tellus_session"
	maxBodyBytes   = 1 << 20 // form posts above 1 MiB are rejected
	loginAttempts  = 10
	loginWindow    = 10 * time.Minute
	defaultPrefix  = "/admin"
	defaultName    = "Tellus"
	defaultSession = 12 * time.Hour
)

var prefixPattern = regexp.MustCompile(`^/[A-Za-z0-9._~-]+(/[A-Za-z0-9._~-]+)*$`)

// Config configures a Panel.
type Config struct {
	// Authenticator signs users in. Use NewGormAuthenticator for the built-in
	// accounts table or implement the interface for your own users. Required.
	Authenticator Authenticator
	// SessionSecret signs session cookies. At least 32 random bytes. Required.
	SessionSecret []byte
	// Prefix is the URL path the panel is mounted at. Default "/admin". It can
	// be any path, for example a secret one.
	Prefix string
	// Name is the brand name shown in the UI. Default "Tellus".
	Name string
	// SessionTTL is how long a sign-in lasts. Default 12 hours.
	SessionTTL time.Duration
	// InsecureCookies drops the Secure cookie attribute so the panel works over
	// plain http. For local development only.
	InsecureCookies bool
}

// Panel is the admin panel. Create it with New, add resources with Register,
// then mount Handler on your router at Prefix.
type Panel struct {
	cfg     Config
	prefix  string
	signer  *secure.Signer
	csrf    secure.CSRF
	limiter *secure.Limiter
	now     func() time.Time

	mu       sync.Mutex
	built    bool
	handler  http.Handler
	nav      []navEntry
	slugs    map[string]bool
	mounters []func(*http.ServeMux)
}

type navEntry struct{ slug, label string }

// New validates cfg and returns a Panel.
func New(cfg Config) (*Panel, error) {
	if cfg.Authenticator == nil {
		return nil, errors.New("tellus: Config.Authenticator is required, use NewGormAuthenticator for the built-in accounts")
	}
	signer, err := secure.NewSigner(cfg.SessionSecret)
	if err != nil {
		return nil, fmt.Errorf("tellus: Config.SessionSecret: %w", err)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if !prefixPattern.MatchString(prefix) || slices.Contains(strings.Split(prefix, "/"), "..") || slices.Contains(strings.Split(prefix, "/"), ".") {
		return nil, fmt.Errorf("tellus: Config.Prefix %q is not a valid URL path", cfg.Prefix)
	}
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSession
	}
	return &Panel{
		cfg:     cfg,
		prefix:  prefix,
		signer:  signer,
		csrf:    secure.CSRF{Path: prefix, Secure: !cfg.InsecureCookies},
		limiter: secure.NewLimiter(loginAttempts, loginWindow),
		now:     time.Now,
		slugs:   map[string]bool{},
	}, nil
}

// Prefix is the URL path the panel must be mounted at, without a trailing slash.
func (p *Panel) Prefix() string { return p.prefix }

// Registrable is implemented by resource builders. Pass them to Panel.Register.
type Registrable interface {
	register(p *Panel) error
}

// Register adds resources. Call it before Handler.
func (p *Panel) Register(resources ...Registrable) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.built {
		return errors.New("tellus: Register must be called before Handler")
	}
	for _, r := range resources {
		if err := r.register(p); err != nil {
			return err
		}
	}
	return nil
}

// Handler returns the panel's http.Handler. Mount it at Prefix()+"/", or call
// Mount for a *http.ServeMux. It strips the prefix itself.
func (p *Panel) Handler() http.Handler {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.built {
		p.handler = p.build()
		p.built = true
	}
	return p.handler
}

// Mount registers the panel on mux at its prefix.
func (p *Panel) Mount(mux *http.ServeMux) { mux.Handle(p.prefix+"/", p.Handler()) }

func (p *Panel) build() http.Handler {
	app := http.NewServeMux()
	app.HandleFunc("GET /login", p.loginForm)
	app.HandleFunc("POST /login", p.loginSubmit)
	app.HandleFunc("POST /logout", p.logout)
	app.HandleFunc("GET /{$}", p.authed(p.home))
	for _, mount := range p.mounters {
		mount(app)
	}

	guarded := securityHeaders(limitBody(p.csrf.Middleware(app)))
	outer := http.NewServeMux()
	outer.Handle("GET /assets/", http.StripPrefix("/assets/", assets.Handler()))
	outer.Handle("/", guarded)
	return http.StripPrefix(p.prefix, outer)
}

// url builds a panel URL from a path such as "/login".
func (p *Panel) url(path string) string { return p.prefix + path }

// --- middleware ---

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// --- sessions ---

func (p *Panel) cookiePath() string { return p.prefix }

func (p *Panel) startSession(w http.ResponseWriter, u User) {
	now := p.now()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    p.signer.Sign(u.UserID(), p.cfg.SessionTTL, now),
		Path:     p.cookiePath(),
		Expires:  now.Add(p.cfg.SessionTTL),
		HttpOnly: true,
		Secure:   !p.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (p *Panel) endSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     p.cookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !p.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// currentUser resolves the session cookie to a user. ErrUserNotFound means
// nobody is signed in.
func (p *Panel) currentUser(r *http.Request) (User, error) {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, ErrUserNotFound
	}
	id, ok := p.signer.Verify(ck.Value, p.now())
	if !ok {
		return nil, ErrUserNotFound
	}
	return p.cfg.Authenticator.UserByID(r.Context(), id)
}

type authedHandler func(w http.ResponseWriter, r *http.Request, u User)

// authed wraps a handler so it only runs for a signed-in user allowed into the
// panel.
func (p *Panel) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := p.currentUser(r)
		switch {
		case errors.Is(err, ErrUserNotFound):
			p.redirectToLogin(w, r)
			return
		case err != nil:
			p.serverError(w, err)
			return
		case !u.CanAccessPanel():
			http.Error(w, i18n.T("login.forbidden"), http.StatusForbidden)
			return
		}
		h(w, r, u)
	}
}

func (p *Panel) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	login := p.url("/login")
	if r.Header.Get("HX-Request") == "true" {
		// A plain redirect would make htmx swap the login page into the list
		// region, so tell htmx to navigate the whole page instead.
		w.Header().Set("HX-Redirect", login)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, login, http.StatusSeeOther)
}

// --- rendering ---

func (p *Panel) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		p.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (p *Panel) serverError(w http.ResponseWriter, err error) {
	slog.Error("tellus: request failed", "error", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

func (p *Panel) shell(r *http.Request, u User, title, activeSlug string) ui.Shell {
	nav := make([]ui.NavItem, 0, len(p.nav))
	for _, e := range p.nav {
		nav = append(nav, ui.NavItem{Label: e.label, Href: p.url("/" + e.slug), Active: e.slug == activeSlug})
	}
	return ui.Shell{
		Title:    title,
		Brand:    p.cfg.Name,
		Prefix:   p.prefix,
		Nav:      nav,
		UserName: u.DisplayName(),
		CSRF:     secure.CSRFToken(r.Context()),
	}
}

func (p *Panel) renderMessage(w http.ResponseWriter, r *http.Request, u User, status int, heading, message string) {
	p.render(w, r, status, ui.MessagePage(p.shell(r, u, heading, ""), heading, message))
}

// --- pages ---

func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.nav) > 0 {
		http.Redirect(w, r, p.url("/"+p.nav[0].slug), http.StatusSeeOther)
		return
	}
	p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
}
````

Create `login.go`:

````go
package tellus

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
	"github.com/TechnoVizor/tellus/internal/ui"
)

func (p *Panel) loginView(r *http.Request, email, message string) ui.LoginView {
	return ui.LoginView{
		Brand:  p.cfg.Name,
		Prefix: p.prefix,
		Action: p.url("/login"),
		Email:  email,
		Error:  message,
		CSRF:   secure.CSRFToken(r.Context()),
	}
}

func (p *Panel) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, err := p.currentUser(r); err == nil {
		http.Redirect(w, r, p.url("/"), http.StatusSeeOther)
		return
	}
	p.render(w, r, http.StatusOK, ui.LoginPage(p.loginView(r, "", "")))
}

func (p *Panel) loginSubmit(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))

	if !p.limiter.Allow(clientIP(r)) {
		p.render(w, r, http.StatusTooManyRequests, ui.LoginPage(p.loginView(r, email, i18n.T("login.too_many"))))
		return
	}

	user, err := p.cfg.Authenticator.Authenticate(r.Context(), email, r.PostFormValue("password"))
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		p.render(w, r, http.StatusUnauthorized, ui.LoginPage(p.loginView(r, email, i18n.T("login.invalid"))))
		return
	case err != nil:
		p.serverError(w, err)
		return
	case !user.CanAccessPanel():
		// Same message as a wrong password so the response does not reveal that
		// the account exists.
		p.render(w, r, http.StatusUnauthorized, ui.LoginPage(p.loginView(r, email, i18n.T("login.invalid"))))
		return
	}

	p.startSession(w, user)
	http.Redirect(w, r, p.url("/"), http.StatusSeeOther)
}

func (p *Panel) logout(w http.ResponseWriter, r *http.Request) {
	p.endSession(w)
	http.Redirect(w, r, p.url("/login"), http.StatusSeeOther)
}

// clientIP is the peer address. Behind a reverse proxy, set r.RemoteAddr from
// the trusted forwarding header (for example with chi's RealIP middleware)
// before the request reaches the panel.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test . -v -run 'TestNew|TestRegister|TestUnauth|TestLogin|TestAssets|TestSession|TestDeleted|TestAccount|TestLogout|TestOversized|TestCustomPrefix|TestHome'
```

Expected: PASS, all listed tests pass.

- [ ] **Step 5: Commit**

```bash
git add panel.go login.go helpers_test.go panel_test.go
git commit -m "feat: add panel core with sessions, sign-in, CSRF and security headers"
```

### Task 14: Resources: the builder and CRUD handlers

`tellus.Resource[T]` with `Table` and `Form`, startup validation, and the list, create, edit and delete handlers.

**Files:**
- Create: `resource.go`
- Create: `resource_handlers.go`
- Test: `resource_helpers_test.go`
- Test: `resource_test.go`

**Interfaces:**
- Consumes: Everything above: `DataSource`, `NewGormSource`, `form.Field`, `table.Column`, `ui.*`, `Panel` seams.
- Produces: `tellus.Resource[T any](db *gorm.DB) *ResourceBuilder[T]` with `Source(DataSource[T])`, `Slug(string)`, `Label(singular, plural string)`, `PerPage(int)`, `Table(...table.Column)`, `Form(...form.Field)`, all returning the builder. Routes under the panel prefix: `GET /{slug}`, `GET /{slug}/new`, `POST /{slug}`, `GET /{slug}/{id}/edit`, `POST /{slug}/{id}`, `POST /{slug}/{id}/delete`. List query parameters: `q`, `sort`, `dir`, `page`, `notice`. Constants `maxPage`, `maxSearchRunes`. Test helpers: `memSource` (in-memory `DataSource[Product]` that records every `ListQuery`), `productResource(src)`, `loggedIn(t, src, mods...)`, `errBoom`.

- [ ] **Step 1: Write the failing tests**

Create `resource_helpers_test.go`:

````go
package tellus

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

// --- fake data source ---

// memSource is an in-memory DataSource[Product]. It only paginates, and it
// records the last ListQuery so tests can check what the handler asked for.
type memSource struct {
	items     []Product
	nextID    uint
	queries   []ListQuery
	failWith  error
	updateErr error
}

func newMemSource(n int) *memSource {
	s := &memSource{nextID: 1}
	for i := 1; i <= n; i++ {
		s.items = append(s.items, Product{ID: s.nextID, Name: fmt.Sprintf("Product %02d", i), Price: i * 10, Active: i%2 == 0})
		s.nextID++
	}
	return s
}

func (s *memSource) last() ListQuery { return s.queries[len(s.queries)-1] }

func (s *memSource) List(_ context.Context, q ListQuery) (ListResult[Product], error) {
	s.queries = append(s.queries, q)
	if s.failWith != nil {
		return ListResult[Product]{}, s.failWith
	}
	per := q.PerPage
	if per < 1 {
		per = defaultPerPage
	}
	page := max(q.Page, 1)
	start := min((page-1)*per, len(s.items))
	end := min(start+per, len(s.items))
	return ListResult[Product]{Items: append([]Product(nil), s.items[start:end]...), Total: int64(len(s.items))}, nil
}

func (s *memSource) index(id string) int {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return -1
	}
	for i, p := range s.items {
		if uint64(p.ID) == n {
			return i
		}
	}
	return -1
}

func (s *memSource) Find(_ context.Context, id string) (*Product, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	i := s.index(id)
	if i < 0 {
		return nil, ErrNotFound
	}
	p := s.items[i]
	return &p, nil
}

func (s *memSource) Create(_ context.Context, p *Product) error {
	if s.failWith != nil {
		return s.failWith
	}
	p.ID = s.nextID
	s.nextID++
	s.items = append(s.items, *p)
	return nil
}

func (s *memSource) Update(_ context.Context, p *Product) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	i := s.index(fmt.Sprint(p.ID))
	if i < 0 {
		return ErrNotFound
	}
	s.items[i] = *p
	return nil
}

func (s *memSource) Delete(_ context.Context, id string) error {
	if s.failWith != nil {
		return s.failWith
	}
	if i := s.index(id); i >= 0 {
		s.items = append(s.items[:i], s.items[i+1:]...)
	}
	return nil
}

func (s *memSource) ID(p *Product) string { return fmt.Sprint(p.ID) }

var errBoom = errors.New("boom: secret database detail")

// productResource is the resource used by most tests.
func productResource(src DataSource[Product]) *ResourceBuilder[Product] {
	return Resource[Product](nil).Source(src).
		Table(
			table.Text("Name").Searchable().Sortable(),
			table.Text("Price").Sortable(),
			table.Boolean("Active"),
		).
		Form(
			form.Text("Name").Required().MaxLength(100),
			form.Number("Price").Required(),
			form.Toggle("Active"),
		)
}
````

Create `resource_test.go`:

````go
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

func TestHomeRedirectsToFirstResource(t *testing.T) {
	h := loggedIn(t, newMemSource(1))
	res := h.get("/")
	if res.status != http.StatusSeeOther || res.location() != "/admin/products" {
		t.Fatalf("home: %d %q", res.status, res.location())
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
````

- [ ] **Step 2: Run to verify it fails**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go test . -v
```

Expected: FAIL. The build fails because the code under test does not exist yet (for example `undefined: Resource` or `no non-test Go files`).

- [ ] **Step 3: Write the implementation**

Create `resource.go`:

````go
package tellus

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/humanize"
	"github.com/TechnoVizor/tellus/table"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// reservedSlugs are panel routes a resource must not shadow.
var reservedSlugs = map[string]bool{"login": true, "logout": true, "assets": true}

// ResourceBuilder describes how a model appears in the panel. Create one with
// Resource and chain Table and Form.
type ResourceBuilder[T any] struct {
	db       *gorm.DB
	source   DataSource[T]
	slug     string
	singular string
	plural   string
	perPage  int
	columns  []table.Column
	fields   []form.Field
}

// Resource starts describing the model T. db backs the built-in GORM data
// source; it may be nil when Source is used.
func Resource[T any](db *gorm.DB) *ResourceBuilder[T] {
	return &ResourceBuilder[T]{db: db}
}

// Source replaces the built-in GORM data source.
func (b *ResourceBuilder[T]) Source(ds DataSource[T]) *ResourceBuilder[T] { b.source = ds; return b }

// Slug sets the URL segment. Default: the table name, for example "products".
func (b *ResourceBuilder[T]) Slug(slug string) *ResourceBuilder[T] { b.slug = slug; return b }

// Label sets the singular and plural names shown in the UI.
func (b *ResourceBuilder[T]) Label(singular, plural string) *ResourceBuilder[T] {
	b.singular, b.plural = singular, plural
	return b
}

// PerPage sets the page size of the list. Default 25, maximum 200.
func (b *ResourceBuilder[T]) PerPage(n int) *ResourceBuilder[T] { b.perPage = n; return b }

// Table sets the columns of the list.
func (b *ResourceBuilder[T]) Table(columns ...table.Column) *ResourceBuilder[T] {
	b.columns = columns
	return b
}

// Form sets the fields of the create and edit form.
func (b *ResourceBuilder[T]) Form(fields ...form.Field) *ResourceBuilder[T] {
	b.fields = fields
	return b
}

// resource is a validated ResourceBuilder bound to a panel.
type resource[T any] struct {
	panel    *Panel
	source   DataSource[T]
	slug     string
	singular string
	plural   string
	perPage  int

	columns  []table.Column
	colIndex [][]int // reflect field index path per column

	fields    []form.Field
	fieldIdx  [][]int
	fieldType []reflect.Type

	sortable     map[string]bool
	searchFields []string
}

func (b *ResourceBuilder[T]) register(p *Panel) error {
	model := reflect.TypeFor[T]()
	if model.Kind() != reflect.Struct {
		return fmt.Errorf("tellus: Resource[%s] needs a struct type", model)
	}

	source, tableName := b.source, ""
	if source == nil {
		gs, err := NewGormSource[T](b.db)
		if err != nil {
			return err
		}
		source, tableName = gs, gs.schema.Table
	}
	if tableName == "" {
		tableName = strings.ToLower(model.Name()) + "s"
	}

	rs := &resource[T]{
		panel:    p,
		source:   source,
		slug:     b.slug,
		singular: b.singular,
		plural:   b.plural,
		perPage:  b.perPage,
		sortable: map[string]bool{},
	}
	if rs.slug == "" {
		rs.slug = strings.ReplaceAll(tableName, "_", "-")
	}
	if rs.singular == "" {
		rs.singular = humanize.Name(model.Name())
	}
	if rs.plural == "" {
		rs.plural = humanize.Name(tableName)
	}
	if rs.perPage <= 0 {
		rs.perPage = defaultPerPage
	}
	rs.perPage = min(rs.perPage, maxPerPage)

	if !slugPattern.MatchString(rs.slug) || reservedSlugs[rs.slug] {
		return fmt.Errorf("tellus: resource slug %q is invalid or reserved", rs.slug)
	}
	if p.slugs[rs.slug] {
		return fmt.Errorf("tellus: resource slug %q is already registered", rs.slug)
	}
	if len(b.columns) == 0 {
		return fmt.Errorf("tellus: resource %q needs at least one column, call Table(...)", rs.slug)
	}
	if len(b.fields) == 0 {
		return fmt.Errorf("tellus: resource %q needs at least one form field, call Form(...)", rs.slug)
	}

	for _, c := range b.columns {
		info := c.Info()
		sf, ok := model.FieldByName(info.Name)
		if !ok || !sf.IsExported() {
			return fmt.Errorf("tellus: resource %q: column %q is not an exported field of %s", rs.slug, info.Name, model)
		}
		rs.columns = append(rs.columns, c)
		rs.colIndex = append(rs.colIndex, sf.Index)
		if info.Sortable {
			rs.sortable[info.Name] = true
		}
		if info.Searchable {
			rs.searchFields = append(rs.searchFields, info.Name)
		}
	}

	seen := map[string]bool{}
	for _, f := range b.fields {
		info := f.Info()
		if seen[info.Name] {
			return fmt.Errorf("tellus: resource %q: form field %q is used twice", rs.slug, info.Name)
		}
		seen[info.Name] = true
		sf, ok := model.FieldByName(info.Name)
		if !ok || !sf.IsExported() {
			return fmt.Errorf("tellus: resource %q: form field %q is not an exported field of %s", rs.slug, info.Name, model)
		}
		if err := f.Check(sf.Type); err != nil {
			return fmt.Errorf("tellus: resource %q: %w", rs.slug, err)
		}
		rs.fields = append(rs.fields, f)
		rs.fieldIdx = append(rs.fieldIdx, sf.Index)
		rs.fieldType = append(rs.fieldType, sf.Type)
	}

	p.slugs[rs.slug] = true
	p.nav = append(p.nav, navEntry{slug: rs.slug, label: rs.plural})
	p.mounters = append(p.mounters, rs.mount)
	return nil
}
````

Create `resource_handlers.go`:

````go
package tellus

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/ui"
)

const (
	maxPage        = 1_000_000
	maxSearchRunes = 200
)

func (rs *resource[T]) mount(mux *http.ServeMux) {
	base := "/" + rs.slug
	authed := rs.panel.authed
	mux.HandleFunc("GET "+base, authed(rs.list))
	mux.HandleFunc("GET "+base+"/new", authed(rs.newForm))
	mux.HandleFunc("POST "+base, authed(rs.create))
	mux.HandleFunc("GET "+base+"/{id}/edit", authed(rs.editForm))
	mux.HandleFunc("POST "+base+"/{id}", authed(rs.update))
	mux.HandleFunc("POST "+base+"/{id}/delete", authed(rs.delete))
}

// --- list ---

type listParams struct {
	Search string
	Sort   string // a sortable field name or ""
	Dir    string // "asc" or "desc" when Sort is set
	Page   int
}

// parseList reads the list query string. Anything unexpected falls back to a
// safe default, so a hand-edited URL never breaks the page.
func (rs *resource[T]) parseList(r *http.Request) listParams {
	q := r.URL.Query()
	lp := listParams{Search: truncateRunes(strings.TrimSpace(q.Get("q")), maxSearchRunes), Page: 1}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 1 {
		lp.Page = min(n, maxPage)
	}
	if s := q.Get("sort"); rs.sortable[s] { // only declared sortable columns
		lp.Sort, lp.Dir = s, "asc"
		if q.Get("dir") == "desc" {
			lp.Dir = "desc"
		}
	}
	return lp
}

func (rs *resource[T]) listURL(lp listParams) string {
	v := url.Values{}
	if lp.Search != "" {
		v.Set("q", lp.Search)
	}
	if lp.Sort != "" {
		v.Set("sort", lp.Sort)
		v.Set("dir", lp.Dir)
	}
	if lp.Page > 1 {
		v.Set("page", strconv.Itoa(lp.Page))
	}
	u := rs.panel.url("/" + rs.slug)
	if enc := v.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

func (rs *resource[T]) list(w http.ResponseWriter, r *http.Request, u User) {
	lp := rs.parseList(r)
	query := ListQuery{
		Search:       lp.Search,
		SearchFields: rs.searchFields,
		SortField:    lp.Sort,
		SortDesc:     lp.Dir == "desc",
		Page:         lp.Page,
		PerPage:      rs.perPage,
	}
	res, err := rs.source.List(r.Context(), query)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	totalPages := max(1, int((res.Total+int64(rs.perPage)-1)/int64(rs.perPage)))
	if lp.Page > totalPages { // page past the end, for example after deleting the last row of a page
		lp.Page, query.Page = totalPages, totalPages
		if res, err = rs.source.List(r.Context(), query); err != nil {
			rs.panel.serverError(w, err)
			return
		}
	}

	view := rs.listView(r, u, lp, res, totalPages)
	w.Header().Add("Vary", "HX-Request")
	// htmx asks for the region only. A history restore needs the whole page.
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" {
		rs.panel.render(w, r, http.StatusOK, ui.Records(view))
		return
	}
	rs.panel.render(w, r, http.StatusOK, ui.ListPage(view))
}

func (rs *resource[T]) listView(r *http.Request, u User, lp listParams, res ListResult[T], totalPages int) ui.ListView {
	cols := make([]ui.ColumnView, len(rs.columns))
	for i, c := range rs.columns {
		info := c.Info()
		cv := ui.ColumnView{Label: info.Label, Sortable: info.Sortable}
		if info.Sortable {
			next := listParams{Search: lp.Search, Sort: info.Name, Dir: "asc"}
			if lp.Sort == info.Name {
				if lp.Dir == "asc" {
					next.Dir, cv.SortMark, cv.AriaSort = "desc", "▲", "ascending"
				} else {
					cv.SortMark, cv.AriaSort = "▼", "descending"
				}
			}
			cv.SortURL = rs.listURL(next)
		}
		cols[i] = cv
	}

	rows := make([]ui.RowView, len(res.Items))
	for i := range res.Items {
		item := &res.Items[i]
		rv := reflect.ValueOf(item).Elem()
		cells := make([]templ.Component, len(rs.columns))
		for j, c := range rs.columns {
			cells[j] = c.Cell(rv.FieldByIndex(rs.colIndex[j]).Interface())
		}
		id := url.PathEscape(rs.source.ID(item))
		rows[i] = ui.RowView{
			Cells:     cells,
			EditURL:   rs.panel.url("/" + rs.slug + "/" + id + "/edit"),
			DeleteURL: rs.panel.url("/" + rs.slug + "/" + id + "/delete"),
		}
	}

	view := ui.ListView{
		Shell:      rs.panel.shell(r, u, rs.plural, rs.slug),
		Heading:    rs.plural,
		Notice:     rs.notice(r.URL.Query().Get("notice")),
		NewURL:     rs.panel.url("/" + rs.slug + "/new"),
		Search:     lp.Search,
		Searchable: len(rs.searchFields) > 0,
		ListURL:    rs.listURL(listParams{}),
		Sort:       lp.Sort,
		Dir:        lp.Dir,
		Columns:    cols,
		Rows:       rows,
		Total:      res.Total,
		Page:       lp.Page,
		TotalPages: totalPages,
	}
	if lp.Page > 1 {
		prev := lp
		prev.Page--
		view.PrevURL = rs.listURL(prev)
	}
	if lp.Page < totalPages {
		next := lp
		next.Page++
		view.NextURL = rs.listURL(next)
	}
	return view
}

// notice maps the ?notice= value to a message. Only known keys are shown, so
// the parameter can never inject text into the page.
func (rs *resource[T]) notice(kind string) string {
	switch kind {
	case "created", "updated", "deleted":
		return i18n.T("notice." + kind)
	}
	return ""
}

func (rs *resource[T]) redirectWithNotice(w http.ResponseWriter, r *http.Request, kind string) {
	http.Redirect(w, r, rs.listURL(listParams{})+"?notice="+kind, http.StatusSeeOther)
}

// --- create and edit ---

func (rs *resource[T]) newForm(w http.ResponseWriter, r *http.Request, u User) {
	rs.renderForm(w, r, u, http.StatusOK, rs.newTitle(), rs.panel.url("/"+rs.slug), nil, "")
}

func (rs *resource[T]) create(w http.ResponseWriter, r *http.Request, u User) {
	if !rs.parseForm(w, r) {
		return
	}
	var item T
	values, valid, err := rs.apply(&item, r.PostForm)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	if !valid {
		rs.renderForm(w, r, u, http.StatusUnprocessableEntity, rs.newTitle(), rs.panel.url("/"+rs.slug), values, i18n.T("form.error_summary"))
		return
	}
	if err := rs.source.Create(r.Context(), &item); err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "created")
}

func (rs *resource[T]) editForm(w http.ResponseWriter, r *http.Request, u User) {
	id := r.PathValue("id")
	item, err := rs.source.Find(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rv := reflect.ValueOf(item).Elem()
	values := make(map[string]form.Value, len(rs.fields))
	for i, f := range rs.fields {
		values[f.Info().Name] = form.Value{Raw: f.Format(rv.FieldByIndex(rs.fieldIdx[i]).Interface())}
	}
	rs.renderForm(w, r, u, http.StatusOK, rs.editTitle(), rs.itemURL(id), values, "")
}

func (rs *resource[T]) update(w http.ResponseWriter, r *http.Request, u User) {
	if !rs.parseForm(w, r) {
		return
	}
	id := r.PathValue("id")
	item, err := rs.source.Find(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	// Only the declared form fields are copied onto the record, so extra posted
	// values such as ID or CreatedAt are ignored.
	values, valid, err := rs.apply(item, r.PostForm)
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	if !valid {
		rs.renderForm(w, r, u, http.StatusUnprocessableEntity, rs.editTitle(), rs.itemURL(id), values, i18n.T("form.error_summary"))
		return
	}
	err = rs.source.Update(r.Context(), item)
	if errors.Is(err, ErrNotFound) {
		rs.notFound(w, r, u)
		return
	}
	if err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "updated")
}

func (rs *resource[T]) delete(w http.ResponseWriter, r *http.Request, _ User) {
	if err := rs.source.Delete(r.Context(), r.PathValue("id")); err != nil {
		rs.panel.serverError(w, err)
		return
	}
	rs.redirectWithNotice(w, r, "deleted")
}

// apply copies the submitted values of the declared fields onto item. It
// returns each field's raw input with its validation message and whether every
// field was valid.
func (rs *resource[T]) apply(item *T, in url.Values) (map[string]form.Value, bool, error) {
	values := make(map[string]form.Value, len(rs.fields))
	valid := true
	rv := reflect.ValueOf(item).Elem()
	for i, f := range rs.fields {
		name := f.Info().Name
		raw := in.Get(name)
		parsed, msg := f.Parse(raw, rs.fieldType[i])
		values[name] = form.Value{Raw: raw, Error: msg}
		if msg != "" {
			valid = false
			continue
		}
		pv := reflect.ValueOf(parsed)
		if !pv.IsValid() || !pv.Type().AssignableTo(rs.fieldType[i]) {
			return nil, false, fmt.Errorf("tellus: field %q returned %T, want %s", name, parsed, rs.fieldType[i])
		}
		rv.FieldByIndex(rs.fieldIdx[i]).Set(pv)
	}
	return values, valid, nil
}

func (rs *resource[T]) renderForm(w http.ResponseWriter, r *http.Request, u User, status int, heading, action string, values map[string]form.Value, summary string) {
	comps := make([]templ.Component, len(rs.fields))
	for i, f := range rs.fields {
		comps[i] = f.Render(values[f.Info().Name])
	}
	rs.panel.render(w, r, status, ui.FormPage(ui.FormView{
		Shell:     rs.panel.shell(r, u, heading, rs.slug),
		Heading:   heading,
		Action:    action,
		CancelURL: rs.listURL(listParams{}),
		Error:     summary,
		Fields:    comps,
	}))
}

// parseForm parses the request body. It answers the client itself and returns
// false when the body is unreadable or over the size limit.
func (rs *resource[T]) parseForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Bad request", http.StatusBadRequest)
		}
		return false
	}
	return true
}

func (rs *resource[T]) notFound(w http.ResponseWriter, r *http.Request, u User) {
	rs.panel.renderMessage(w, r, u, http.StatusNotFound, i18n.T("common.not_found"), "")
}

func (rs *resource[T]) itemURL(id string) string {
	return rs.panel.url("/" + rs.slug + "/" + url.PathEscape(id))
}

func (rs *resource[T]) newTitle() string {
	return fmt.Sprintf(i18n.T("form.new_title"), rs.singular)
}

func (rs *resource[T]) editTitle() string {
	return fmt.Sprintf(i18n.T("form.edit_title"), rs.singular)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
````

- [ ] **Step 4: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test . -v
```

Expected: PASS, the whole root package passes, including `TestRegisterValidation` and `TestUserInputIsEscapedEverywhere`.

- [ ] **Step 5: Commit**

```bash
git add resource.go resource_handlers.go resource_helpers_test.go resource_test.go
git commit -m "feat: add resource builder with list, create, edit and delete handlers"
```

### Task 15: Milestone M0 acceptance test

One test that walks the whole flow with the real database, the built-in accounts and the builder description from the README.

**Files:**
- Test: `m0_test.go`

**Interfaces:**
- Consumes: Everything.
- Produces: Nothing new. If this test passes, the M0 definition of done in `docs/spec.md` section 10 holds: a `Product` resource end to end, assembled from a builder description of fewer than 30 lines.

- [ ] **Step 1: Write the acceptance test**

Create `m0_test.go`:

````go
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
````

- [ ] **Step 2: Tidy modules, run tests to verify they pass**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
go mod tidy
test -z "$(gofmt -l .)" && go vet ./...
go test . -run TestM0ProductResourceEndToEnd -v
```

Expected: PASS, `--- PASS: TestM0ProductResourceEndToEnd`.

- [ ] **Step 3: Commit**

```bash
git add m0_test.go
git commit -m "test: add milestone M0 end-to-end acceptance test"
```

### Task 16: Example shop, README and final verification

A runnable example, the README, and a manual browser check of the parts automated tests cannot see.

**Files:**
- Create: `examples/shop/main.go`
- Create: `examples/shop/go.mod`
- Create: `README.md`
- Modify: none

**Interfaces:**
- Consumes: Everything.
- Produces: `examples/shop` is a separate module (`replace github.com/TechnoVizor/tellus => ../..`) that serves the panel on port 8091 with 45 seeded products and a demo account.

- [ ] **Step 1: Add the example and the README**

Create `examples/shop/main.go`:

````go
// Command shop is a small storefront backend with a Tellus admin panel. It is
// the example that grows into the public live demo.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus"
	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

type Product struct {
	ID          uint
	Name        string `gorm:"not null"`
	Description string
	Price       int
	Active      bool
	CreatedAt   time.Time
}

func main() {
	db, err := gorm.Open(postgres.Open(env("DATABASE_URL", "postgres://tellus:tellus@localhost:55432/tellus_dev?sslmode=disable")), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&Product{}); err != nil {
		log.Fatal(err)
	}
	seedProducts(db)

	auth := tellus.NewGormAuthenticator(db)
	if err := auth.Migrate(); err != nil {
		log.Fatal(err)
	}
	seedAdmin(auth)

	panel, err := tellus.New(tellus.Config{
		Authenticator:   auth,
		SessionSecret:   sessionSecret(),
		Prefix:          env("ADMIN_PREFIX", "/admin"),
		Name:            "Shop admin",
		InsecureCookies: true, // plain http on localhost
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := panel.Register(
		tellus.Resource[Product](db).
			Table(
				table.Text("Name").Searchable().Sortable(),
				table.Text("Price").Sortable(),
				table.Boolean("Active"),
			).
			Form(
				form.Text("Name").Required().MaxLength(100),
				form.Textarea("Description"),
				form.Number("Price").Required(),
				form.Toggle("Active"),
			),
	); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	panel.Mount(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, panel.Prefix()+"/", http.StatusSeeOther)
	})

	addr := env("ADDR", ":8091")
	log.Printf("shop admin on http://localhost%s%s", addr, panel.Prefix())
	log.Fatal(http.ListenAndServe(addr, mux))
}

func seedProducts(db *gorm.DB) {
	var n int64
	db.Model(&Product{}).Count(&n)
	if n > 0 {
		return
	}
	for i := 1; i <= 45; i++ {
		db.Create(&Product{
			Name:        fmt.Sprintf("Product %02d", i),
			Description: "Sample product for the demo.",
			Price:       i * 10,
			Active:      i%3 != 0,
		})
	}
}

// seedAdmin creates the demo account on first run. Local demo credentials only.
func seedAdmin(auth *tellus.GormAuthenticator) {
	email := env("ADMIN_EMAIL", "admin@example.com")
	password := env("ADMIN_PASSWORD", "demo-password-123")
	if _, err := auth.Authenticate(context.Background(), email, password); err == nil {
		return
	}
	if err := auth.CreateUser(context.Background(), email, "Demo admin", password); err != nil {
		log.Printf("demo admin not created (it may exist with another password): %v", err)
	}
}

// sessionSecret uses SESSION_SECRET when set. Otherwise it generates a random
// one, so every restart signs everyone out.
func sessionSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
````

Create `examples/shop/go.mod`:

````go
module github.com/TechnoVizor/tellus/examples/shop

go 1.26.0

require (
	github.com/TechnoVizor/tellus v0.0.0
	gorm.io/driver/postgres v1.6.3
	gorm.io/gorm v1.31.2
)

require (
	github.com/a-h/templ v0.3.1020 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/TechnoVizor/tellus => ../..
````

Create `README.md`:

````markdown
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

mux := http.NewServeMux()
panel.Mount(mux) // serves the panel at panel.Prefix()
log.Fatal(http.ListenAndServe(":8080", mux))
```

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
````

Then tidy the example module:

```bash
(cd examples/shop && go mod tidy)
```

- [ ] **Step 2: Full verification**

```bash
export TELLUS_TEST_DSN='postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable'
gofmt -l .        # prints nothing
go build ./... && go vet ./...
go test ./...
(cd examples/shop && go vet ./... && go build -o /dev/null .)
```

Expected: no output from `gofmt -l .`, every package `ok`, both builds succeed.

- [ ] **Step 3: Manual browser check of what tests cannot see**

Start the example (`cd examples/shop && go run .`), open http://localhost:8091/admin and check, in a real browser:

1. The sign-in page loads with the Inter font (the Network tab shows `inter-latin.woff2` served from `/admin/assets/fonts/`). Sign in as `admin@example.com` with `demo-password-123`.
2. Typing in the search box updates the rows after a short pause and rewrites the address bar (`?q=...`). Clicking a sortable heading sorts, shows the arrow, and keeps the search text.
3. After a search or sort swap, the Delete button still asks for confirmation. This is the htmx plus Alpine interplay flagged as a risk in the spec. Do not confirm the dialog. In the console you can stub `window.confirm = () => false` and press Delete to prove the handler is wired.
4. The Theme button switches between light and dark and the choice survives a reload. Both themes look right: monochrome ink on white, hairline borders, pill buttons.
5. Saving an empty form shows the error summary and the field messages, keeps what was typed, and stores nothing.
6. Signing out returns to the sign-in page, and the back button does not show the list again from cache (`Cache-Control: no-store`).

If item 3 fails, stop and report it: it invalidates the htmx plus Alpine approach chosen in the spec.

- [ ] **Step 4: Commit**

```bash
git add examples README.md go.mod go.sum
git commit -m "feat: add example shop admin and README"
```
