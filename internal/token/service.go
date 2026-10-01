package token

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

type Service struct {
	repo Repository
	ttl  time.Duration
}

func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl}
}

func (s *Service) IssueRefreshToken(ctx context.Context, userID string) (string, error) {
	plaintext, err := generatePlaintext()
	if err != nil {
		return "", err
	}

	if _, err := s.repo.Create(ctx, RefreshToken{
		UserID:    userID,
		TokenHash: hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	}); err != nil {
		return "", err
	}

	return plaintext, nil
}

// Rotate exchanges a valid, unused refresh token for a new one, revoking
// the old one in the same operation. If the submitted token has already
// been rotated out (reused — a sign it may have been stolen), every
// refresh token for that user is revoked, forcing a fresh login
// everywhere, and ErrInvalid is returned.
func (s *Service) Rotate(ctx context.Context, plaintext string) (userID string, newPlaintext string, err error) {
	rt, err := s.repo.FindByHash(ctx, hash(plaintext))
	if err != nil {
		if err == ErrNotFound {
			return "", "", ErrInvalid
		}
		return "", "", err
	}

	if rt.RevokedAt != nil {
		_ = s.repo.RevokeAllForUser(ctx, rt.UserID)
		return "", "", ErrInvalid
	}

	if time.Now().After(rt.ExpiresAt) {
		return "", "", ErrInvalid
	}

	if err := s.repo.Revoke(ctx, rt.ID); err != nil {
		return "", "", err
	}

	newPlaintext, err = s.IssueRefreshToken(ctx, rt.UserID)
	if err != nil {
		return "", "", err
	}

	return rt.UserID, newPlaintext, nil
}

func (s *Service) Revoke(ctx context.Context, plaintext string) error {
	rt, err := s.repo.FindByHash(ctx, hash(plaintext))
	if err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}

	return s.repo.Revoke(ctx, rt.ID)
}

func (s *Service) RevokeAllForUser(ctx context.Context, userID string) error {
	return s.repo.RevokeAllForUser(ctx, userID)
}

func generatePlaintext() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
