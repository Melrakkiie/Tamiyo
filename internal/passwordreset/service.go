package passwordreset

import (
	"context"
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
	rt, err := s.repo.FindByHash(ctx, token.Hash(plaintext))
	if err != nil {
		if err == ErrNotFound {
			return "", ErrInvalid
		}
		return "", err
	}

	if rt.UsedAt != nil {
		return "", ErrInvalid
	}

	if time.Now().After(rt.ExpiresAt) {
		return "", ErrInvalid
	}

	if err := s.repo.MarkUsed(ctx, rt.ID); err != nil {
		return "", err
	}

	return rt.UserID, nil
}
