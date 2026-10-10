package follow

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrSelfFollow   = errors.New("you cannot follow yourself")
)

type Status struct {
	Followers    int
	Following    int
	FollowedByMe bool
	FollowsMe    bool
}

type Connection struct {
	ID               string
	DisplayName      *string
	AvatarScryfallID *string
	Since            time.Time
	FollowedByMe     bool
}

type Page struct {
	Number int
	Limit  int
}

type Repository interface {
	UserExists(ctx context.Context, userID string) (bool, error)
	Follow(ctx context.Context, followerID string, followedID string) error
	Unfollow(ctx context.Context, followerID string, followedID string) error
	Status(ctx context.Context, viewerID string, userID string) (Status, error)
	Followers(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error)
	Following(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) requireUser(ctx context.Context, userID string) error {
	exists, err := s.repo.UserExists(ctx, userID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	return nil
}

func (s *Service) Status(ctx context.Context, viewerID string, userID string) (Status, error) {
	if err := s.requireUser(ctx, userID); err != nil {
		return Status{}, err
	}
	return s.repo.Status(ctx, viewerID, userID)
}

func (s *Service) Follow(ctx context.Context, viewerID string, userID string) (Status, error) {
	if viewerID == userID {
		return Status{}, ErrSelfFollow
	}
	if err := s.requireUser(ctx, userID); err != nil {
		return Status{}, err
	}
	if err := s.repo.Follow(ctx, viewerID, userID); err != nil {
		return Status{}, err
	}
	return s.repo.Status(ctx, viewerID, userID)
}

func (s *Service) Unfollow(ctx context.Context, viewerID string, userID string) (Status, error) {
	if err := s.requireUser(ctx, userID); err != nil {
		return Status{}, err
	}
	if err := s.repo.Unfollow(ctx, viewerID, userID); err != nil {
		return Status{}, err
	}
	return s.repo.Status(ctx, viewerID, userID)
}

func (s *Service) Followers(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	if err := s.requireUser(ctx, userID); err != nil {
		return nil, 0, err
	}
	return s.repo.Followers(ctx, viewerID, userID, page)
}

func (s *Service) Following(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	if err := s.requireUser(ctx, userID); err != nil {
		return nil, 0, err
	}
	return s.repo.Following(ctx, viewerID, userID, page)
}
