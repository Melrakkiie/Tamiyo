package token

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("refresh token not found")
var ErrInvalid = errors.New("invalid or expired refresh token")

type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	Added      time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
}

type Repository interface {
	Create(ctx context.Context, t RefreshToken) (RefreshToken, error)
	FindByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	FindByID(ctx context.Context, id string) (RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	// Replace revokes id and links it to successorID only if it is still active
	// and has no successor; it reports whether it did.
	Replace(ctx context.Context, id string, successorID string) (bool, error)
	// RevokeSession revokes id and every token that replaced it, following
	// replaced_by; tokens already revoked keep their revocation date.
	RevokeSession(ctx context.Context, userID string, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}
