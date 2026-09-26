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
