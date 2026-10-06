package token

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

const (
	reuseGrace       = 10 * time.Second
	maxSuccessorHops = 5
)

type Service struct {
	repo Repository
	ttl  time.Duration
}

func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl}
}

func (s *Service) IssueRefreshToken(ctx context.Context, userID string) (string, error) {
	plaintext, _, err := s.issue(ctx, userID)
	return plaintext, err
}

func (s *Service) issue(ctx context.Context, userID string) (plaintext string, id string, err error) {
	plaintext, err = generatePlaintext()
	if err != nil {
		return "", "", err
	}

	created, err := s.repo.Create(ctx, RefreshToken{
		UserID:    userID,
		TokenHash: hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	})
	if err != nil {
		return "", "", err
	}

	return plaintext, created.ID, nil
}

func (s *Service) Rotate(ctx context.Context, plaintext string) (userID string, newPlaintext string, err error) {
	rt, err := s.repo.FindByHash(ctx, hash(plaintext))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", "", ErrInvalid
		}
		return "", "", err
	}

	if rt.RevokedAt != nil {
		successor, err := s.activeSuccessor(ctx, rt)
		if err != nil {
			return "", "", err
		}
		if successor == nil {
			_ = s.repo.RevokeAllForUser(ctx, rt.UserID)
			return "", "", ErrInvalid
		}
		rt = *successor
	}

	if time.Now().After(rt.ExpiresAt) {
		return "", "", ErrInvalid
	}

	newPlaintext, successorID, err := s.issue(ctx, rt.UserID)
	if err != nil {
		return "", "", err
	}

	if err := s.repo.Replace(ctx, rt.ID, successorID); err != nil {
		return "", "", err
	}

	return rt.UserID, newPlaintext, nil
}

func (s *Service) activeSuccessor(ctx context.Context, rt RefreshToken) (*RefreshToken, error) {
	current := rt
	for range maxSuccessorHops {
		if current.ReplacedBy == nil || current.RevokedAt == nil || time.Since(*current.RevokedAt) > reuseGrace {
			return nil, nil
		}

		next, err := s.repo.FindByID(ctx, *current.ReplacedBy)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		if next.RevokedAt == nil {
			return &next, nil
		}
		current = next
	}

	return nil, nil
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
