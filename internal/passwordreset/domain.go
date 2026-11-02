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
}

type Repository interface {
	Create(ctx context.Context, t ResetToken) (ResetToken, error)
	// Consume atomically marks a valid token used and returns its user; ErrNotFound if unknown, used or expired.
	Consume(ctx context.Context, tokenHash string) (userID string, err error)
	InvalidatePending(ctx context.Context, userID string) error
}
