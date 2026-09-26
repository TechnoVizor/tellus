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
