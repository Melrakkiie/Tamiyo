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
	FindByHash(ctx context.Context, tokenHash string) (ChangeToken, error)
	MarkUsed(ctx context.Context, id string) error
}
