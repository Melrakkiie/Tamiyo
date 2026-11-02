package emailchange

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

func (s *Service) Issue(ctx context.Context, userID, newEmail string) (string, error) {
	plaintext, err := token.GeneratePlaintext()
	if err != nil {
		return "", err
	}

	if _, err := s.repo.Create(ctx, ChangeToken{
		UserID:    userID,
		NewEmail:  newEmail,
		TokenHash: token.Hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	}); err != nil {
		return "", err
	}

	return plaintext, nil
}

func (s *Service) Consume(ctx context.Context, plaintext string) (userID, newEmail string, err error) {
	userID, newEmail, err = s.repo.Consume(ctx, token.Hash(plaintext))
	if errors.Is(err, ErrNotFound) {
		return "", "", ErrInvalid
	}
	return userID, newEmail, err
}
