package user

import (
	"context"
	"errors"
	"time"
)

var ErrEmailAlreadyTaken = errors.New("email already registered")
var ErrNotFound = errors.New("user not found")
var ErrInvalidCredentials = errors.New("invalid email or password")
var ErrIncorrectPassword = errors.New("incorrect current password")
var ErrInvalidDisplayName = errors.New("display_name must be at most 32 characters")

const MaxDisplayNameLength = 32

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Added        time.Time
	Updated      time.Time
	DisplayName  *string
}

type Repository interface {
	Create(ctx context.Context, u User) (User, error)
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id string) (User, error)
	UpdatePassword(ctx context.Context, id string, passwordHash string) error
	UpdateEmail(ctx context.Context, id string, email string) error
	UpdateDisplayName(ctx context.Context, id string, displayName *string) (User, error)
}
