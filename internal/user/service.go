package user

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	return s.repo.Create(ctx, User{
		Email:        email,
		PasswordHash: string(hash),
	})
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if err == ErrNotFound {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	return u, nil
}
