package emailchange

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("email change token not found")
var ErrInvalid = errors.New("invalid or expired email change token")
var ErrSameEmail = errors.New("new email is the current email")

type ChangeToken struct {
	ID        string
	UserID    string
	NewEmail  string
	TokenHash string
	Added     time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type Repository interface {
	Create(ctx context.Context, t ChangeToken) (ChangeToken, error)
	// Consume atomically marks an unused, unexpired token used and returns its
	// user and new email; ErrNotFound when no such token exists.
	Consume(ctx context.Context, tokenHash string) (userID, newEmail string, err error)
}
