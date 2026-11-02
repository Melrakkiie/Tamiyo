package passwordreset

import (
	"context"
	"errors"
	"time"

	"Melrakkiie/Tamiyo/internal/token"
)

type Service struct {
	repo Repository
	ttl  time.Duration
}

func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl}
}

func (s *Service) Issue(ctx context.Context, userID string) (string, error) {
	plaintext, err := token.GeneratePlaintext()
	if err != nil {
		return "", err
	}

	if _, err := s.repo.Create(ctx, ResetToken{
		UserID:    userID,
		TokenHash: token.Hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	}); err != nil {
		return "", err
	}

	return plaintext, nil
}

func (s *Service) Consume(ctx context.Context, plaintext string) (userID string, err error) {
	userID, err = s.repo.Consume(ctx, token.Hash(plaintext))
	if errors.Is(err, ErrNotFound) {
		return "", ErrInvalid
	}
	return userID, err
}

// InvalidatePending voids every unused reset link of the user.
func (s *Service) InvalidatePending(ctx context.Context, userID string) error {
	return s.repo.InvalidatePending(ctx, userID)
}
