package emailverification

import (
	"context"
	"errors"
	"time"

	"Melrakkiie/Tamiyo/internal/token"
)

type userAccounts interface {
	// ConfirmEmail makes email the account's address and marks it verified; user.ErrEmailAlreadyTaken if another account has it.
	ConfirmEmail(ctx context.Context, userID, email string) error
}

type mailer interface {
	SendEmailVerification(ctx context.Context, toEmail, verificationToken string) error
}

type Service struct {
	repo   Repository
	users  userAccounts
	mailer mailer
	ttl    time.Duration
}

func NewService(repo Repository, users userAccounts, mailer mailer, ttl time.Duration) *Service {
	return &Service{repo: repo, users: users, mailer: mailer, ttl: ttl}
}

// IssueAndSend mints a verification link for the user and mails it to email.
// Registration, resend and email change all go through it.
func (s *Service) IssueAndSend(ctx context.Context, userID, email string) error {
	plaintext, err := token.GeneratePlaintext()
	if err != nil {
		return err
	}

	if _, err := s.repo.Create(ctx, VerificationToken{
		UserID:    userID,
		Email:     email,
		TokenHash: token.Hash(plaintext),
		ExpiresAt: time.Now().Add(s.ttl),
	}); err != nil {
		return err
	}

	return s.mailer.SendEmailVerification(ctx, email, plaintext)
}

// Verify consumes the token and makes its address the owner's verified email; ErrInvalid if unknown, used or expired.
// The token stays consumed when the address was taken meanwhile (user.ErrEmailAlreadyTaken).
func (s *Service) Verify(ctx context.Context, plaintext string) error {
	userID, email, err := s.repo.Consume(ctx, token.Hash(plaintext))
	if errors.Is(err, ErrNotFound) {
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	return s.users.ConfirmEmail(ctx, userID, email)
}
