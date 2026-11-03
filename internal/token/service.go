package token

import (
	"context"
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
	plaintext, err = GeneratePlaintext()
	if err != nil {
		return "", "", err
	}

	created, err := s.repo.Create(ctx, RefreshToken{
		UserID:    userID,
		TokenHash: Hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	})
	if err != nil {
		return "", "", err
	}

	return plaintext, created.ID, nil
}

func (s *Service) Rotate(ctx context.Context, plaintext string) (userID string, newPlaintext string, err error) {
	rt, err := s.repo.FindByHash(ctx, Hash(plaintext))
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
			_ = s.repo.RevokeSession(ctx, rt.UserID, rt.ID)
			return "", "", ErrInvalid
		}
		rt = *successor
	}

	// A lost Replace means another refresh won the race: retry from its successor.
	for range maxSuccessorHops {
		if time.Now().After(rt.ExpiresAt) {
			return "", "", ErrInvalid
		}

		newPlaintext, replaced, err := s.replaceOrDiscard(ctx, rt)
		if err != nil {
			return "", "", err
		}
		if replaced {
			return rt.UserID, newPlaintext, nil
		}

		current, err := s.repo.FindByID(ctx, rt.ID)
		if errors.Is(err, ErrNotFound) {
			return "", "", ErrInvalid
		}
		if err != nil {
			return "", "", err
		}
		successor, err := s.activeSuccessor(ctx, current)
		if err != nil {
			return "", "", err
		}
		if successor == nil {
			return "", "", ErrInvalid
		}
		rt = *successor
	}

	return "", "", ErrInvalid
}

// replaceOrDiscard issues a token and links it to rt; if rt was already
// replaced or revoked, the token just issued is revoked and replaced is false.
func (s *Service) replaceOrDiscard(ctx context.Context, rt RefreshToken) (newPlaintext string, replaced bool, err error) {
	newPlaintext, newID, err := s.issue(ctx, rt.UserID)
	if err != nil {
		return "", false, err
	}

	replaced, err = s.repo.Replace(ctx, rt.ID, newID)
	if err != nil {
		return "", false, err
	}
	if !replaced {
		if err := s.repo.Revoke(ctx, newID); err != nil {
			return "", false, err
		}
	}

	return newPlaintext, replaced, nil
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
	rt, err := s.repo.FindByHash(ctx, Hash(plaintext))
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
