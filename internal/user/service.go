package user

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"Melrakkiie/Tamiyo/internal/emailaddr"
)

type verificationSender interface {
	IssueAndSend(ctx context.Context, userID, email string) error
}

type Service struct {
	repo     Repository
	verifier verificationSender
	logger   *zap.Logger
}

type Option func(*Service)

// WithVerification makes Register mail a confirmation link; a failed send is logged, never returned.
func WithVerification(verifier verificationSender, logger *zap.Logger) Option {
	return func(s *Service) { s.verifier, s.logger = verifier, logger }
}

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Register(ctx context.Context, email, password string, displayName *string) (User, error) {
	normalizedName, err := normalizeDisplayName(displayName)
	if err != nil {
		return User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	created, err := s.repo.Create(ctx, User{
		Email:        emailaddr.Normalize(email),
		PasswordHash: string(hash),
		DisplayName:  normalizedName,
	})
	if err != nil {
		return User{}, err
	}

	s.sendVerification(ctx, created.ID, created.Email)

	return created, nil
}

// sendVerification proves an address; a failed send is logged, never returned.
func (s *Service) sendVerification(ctx context.Context, userID, email string) {
	if s.verifier == nil {
		return
	}
	if err := s.verifier.IssueAndSend(ctx, userID, email); err != nil {
		s.logger.Warn("could not send the verification email", zap.String("user_id", userID), zap.Error(err))
	}
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	u, err := s.repo.FindByEmail(ctx, emailaddr.Normalize(email))
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

func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrIncorrectPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(ctx, userID, string(hash))
}

func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return ErrIncorrectPassword
	}

	return s.repo.Delete(ctx, userID)
}

func (s *Service) FindIDByEmail(ctx context.Context, email string) (string, error) {
	u, err := s.repo.FindByEmail(ctx, emailaddr.Normalize(email))
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

func (s *Service) SetPassword(ctx context.Context, userID, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, userID, string(hash))
}

func (s *Service) GetUser(ctx context.Context, userID string) (User, error) {
	return s.repo.FindByID(ctx, userID)
}

func (s *Service) CheckPassword(ctx context.Context, userID, password string) (string, error) {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", ErrIncorrectPassword
	}

	return u.Email, nil
}

func (s *Service) EmailTaken(ctx context.Context, email string) (bool, error) {
	_, err := s.repo.FindByEmail(ctx, emailaddr.Normalize(email))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) ChangeEmail(ctx context.Context, userID, newEmail string) (string, error) {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}

	// The account keeps its address and its verification until the new one is proven (emailverification.Verify).
	if s.verifier == nil {
		return "", errors.New("email verification is not configured")
	}
	if err := s.verifier.IssueAndSend(ctx, userID, emailaddr.Normalize(newEmail)); err != nil {
		return "", err
	}

	return u.Email, nil
}

func (s *Service) SetDisplayName(ctx context.Context, userID string, displayName *string) (User, error) {
	normalized, err := normalizeDisplayName(displayName)
	if err != nil {
		return User{}, err
	}

	return s.repo.UpdateDisplayName(ctx, userID, normalized)
}

func normalizeDisplayName(displayName *string) (*string, error) {
	if displayName == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*displayName)
	if utf8.RuneCountInString(trimmed) > MaxDisplayNameLength {
		return nil, ErrInvalidDisplayName
	}
	if trimmed == "" {
		return nil, nil
	}
	return &trimmed, nil
}

func (s *Service) SetAvatar(ctx context.Context, userID string, avatarScryfallID *string) (User, error) {
	normalized, err := normalizeAvatarID(avatarScryfallID)
	if err != nil {
		return User{}, err
	}

	return s.repo.UpdateAvatar(ctx, userID, normalized)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func normalizeAvatarID(avatarScryfallID *string) (*string, error) {
	if avatarScryfallID == nil {
		return nil, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*avatarScryfallID))
	if normalized == "" {
		return nil, nil
	}
	if !uuidPattern.MatchString(normalized) {
		return nil, ErrInvalidAvatar
	}
	return &normalized, nil
}
