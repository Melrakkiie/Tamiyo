package emailverification

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("verification token not found")
var ErrInvalid = errors.New("invalid or expired verification token")

type VerificationToken struct {
	ID        string
	UserID    string
	TokenHash string
	Added     time.Time
	ExpiresAt time.Time
}

type Repository interface {
	Create(ctx context.Context, t VerificationToken) (VerificationToken, error)
	// Consume atomically marks a valid token used and returns its user; ErrNotFound if unknown, used or expired.
	Consume(ctx context.Context, tokenHash string) (userID string, err error)
}
