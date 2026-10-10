package deck

import "context"

type LikeStatus struct {
	Count     int
	LikedByMe bool
}

type LikeRepository interface {
	Like(ctx context.Context, userID string, deckID string) error
	Unlike(ctx context.Context, userID string, deckID string) error
	FindLikeStatus(ctx context.Context, userID string, deckID string) (LikeStatus, error)
	FindLiked(ctx context.Context, userID string, page int, limit int) ([]PublicDeck, int, error)
}

func (s *Service) LikeDeck(ctx context.Context, userID string, deckID string) error {
	return s.repo.Like(ctx, userID, deckID)
}

func (s *Service) UnlikeDeck(ctx context.Context, userID string, deckID string) error {
	return s.repo.Unlike(ctx, userID, deckID)
}

func (s *Service) GetLikeStatus(ctx context.Context, userID string, deckID string) (LikeStatus, error) {
	return s.repo.FindLikeStatus(ctx, userID, deckID)
}

func (s *Service) GetLikedDecks(ctx context.Context, userID string, page int, limit int) ([]PublicDeck, int, error) {
	return s.repo.FindLiked(ctx, userID, page, limit)
}
