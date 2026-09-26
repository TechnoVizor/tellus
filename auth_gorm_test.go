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
