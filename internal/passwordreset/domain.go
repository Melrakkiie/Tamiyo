package passwordreset

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("reset token not found")
var ErrInvalid = errors.New("invalid or expired reset token")

type ResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	Added     time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type Repository interface {
	Create(ctx context.Context, t ResetToken) (ResetToken, error)
	FindByHash(ctx context.Context, tokenHash string) (ResetToken, error)
	MarkUsed(ctx context.Context, id string) error
}
