package deck

import (
	"context"
	"errors"
)

// Reach says which decks a viewer may read.
type Reach int

const (
	// ReachOwn reads only the viewer's own deck.
	ReachOwn Reach = iota
	// ReachShared reads only an unlisted or public deck, even the viewer's own.
	ReachShared
	// ReachVisible reads the viewer's own deck, else a shared one. An anonymous
	// viewer reads as with ReachShared.
	ReachVisible
)

// ReadDeck is a deck as one viewer reads it.
type ReadDeck struct {
	OwnerID string
	// Mine is true when the viewer owns the deck, whatever the reach.
	Mine bool
	Deck Deck
	// Entries are the owned and pending copies merged and ordered as by
	// readDeckContent.
	Entries []DeckEntry
	// Tags maps a CardNameKey to the sorted tags of that card (ADR-0006).
	// Only cards in Entries appear.
	Tags map[string][]string
}

// MainBoard keeps the Entries order.
func (r ReadDeck) MainBoard() []DeckEntry {
	main := make([]DeckEntry, 0, len(r.Entries))
	for _, e := range r.Entries {
		if InMainBoard(e.Board) {
			main = append(main, e)
		}
	}
	return main
}

// ReadDeck returns the deck as the viewer (empty for an anonymous visitor)
// reads it within the reach. It answers ErrNotFound when the deck does not
// exist or is outside the reach.
func (s *Service) ReadDeck(ctx context.Context, viewerID string, deckID string, reach Reach) (ReadDeck, error) {
	ownerID, d, err := s.findWithinReach(ctx, viewerID, deckID, reach)
	if err != nil {
		return ReadDeck{}, err
	}
	entries, err := loadEntries(ctx, s.repo, ownerID, d)
	if err != nil {
		return ReadDeck{}, err
	}
	tagRows, err := s.repo.FindCardTags(ctx, ownerID, deckID)
	if err != nil {
		return ReadDeck{}, err
	}
	return ReadDeck{
		OwnerID: ownerID,
		Mine:    viewerID != "" && ownerID == viewerID,
		Deck:    d,
		Entries: entries,
		Tags:    tagsByName(cardNames(entries), tagRows),
	}, nil
}

// DeckAccess is what an access check needs of a deck, without its content.
type DeckAccess struct {
	OwnerID string
	// Mine is true when the viewer owns the deck, whatever the reach.
	Mine       bool
	Visibility string
}

// FindAccess resolves the deck within the reach as ReadDeck does, but reads
// neither entries nor tags. It answers ErrNotFound when the deck does not
// exist or is outside the reach.
func (s *Service) FindAccess(ctx context.Context, viewerID string, deckID string, reach Reach) (DeckAccess, error) {
	ownerID, d, err := s.findWithinReach(ctx, viewerID, deckID, reach)
	if err != nil {
		return DeckAccess{}, err
	}
	return DeckAccess{OwnerID: ownerID, Mine: viewerID != "" && ownerID == viewerID, Visibility: d.Visibility}, nil
}

// findWithinReach never looks a deck up by an empty viewer: user ids are
// uuids, so Postgres would reject the comparison instead of finding nothing.
func (s *Service) findWithinReach(ctx context.Context, viewerID string, deckID string, reach Reach) (string, Deck, error) {
	if viewerID != "" {
		d, err := s.repo.FindByID(ctx, viewerID, deckID)
		switch {
		case err != nil && !errors.Is(err, ErrNotFound):
			return viewerID, d, err
		case err == nil && (reach != ReachShared || d.Visibility != VisibilityPrivate):
			// ReachShared still hides a private deck, but not an unverified owner's own shared one.
			return viewerID, d, nil
		}
	}
	if reach == ReachOwn {
		return "", Deck{}, ErrNotFound
	}
	return s.repo.FindShared(ctx, deckID)
}
