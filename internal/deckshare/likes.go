package deckshare

import (
	"context"
	"errors"

	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrLikeOwnDeck = errors.New("you cannot like your own deck")

func (s *Service) LikeStatus(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	if _, _, err := s.resolveVisible(ctx, userID, deckID); err != nil {
		return deck.LikeStatus{}, err
	}
	return s.decks.GetLikeStatus(ctx, userID, deckID)
}

func (s *Service) LikeDeck(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	ownerID, _, err := s.resolveVisible(ctx, userID, deckID)
	if err != nil {
		return deck.LikeStatus{}, err
	}
	if ownerID == userID {
		return deck.LikeStatus{}, ErrLikeOwnDeck
	}
	if err := s.decks.LikeDeck(ctx, userID, deckID); err != nil {
		return deck.LikeStatus{}, err
	}
	return s.decks.GetLikeStatus(ctx, userID, deckID)
}

func (s *Service) UnlikeDeck(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	if _, _, err := s.resolveVisible(ctx, userID, deckID); err != nil {
		return deck.LikeStatus{}, err
	}
	if err := s.decks.UnlikeDeck(ctx, userID, deckID); err != nil {
		return deck.LikeStatus{}, err
	}
	return s.decks.GetLikeStatus(ctx, userID, deckID)
}

func (s *Service) LikedDecks(ctx context.Context, userID string, page int, limit int) ([]deck.PublicDeck, int, error) {
	return s.decks.GetLikedDecks(ctx, userID, page, limit)
}
