package token

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("refresh token not found")
var ErrInvalid = errors.New("invalid or expired refresh token")

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	Added     time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type Repository interface {
	Create(ctx context.Context, t RefreshToken) (RefreshToken, error)
	FindByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}
