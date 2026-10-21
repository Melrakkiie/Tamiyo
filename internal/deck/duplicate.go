package deck

import (
	"context"
	"strings"
)

const copySuffix = " (copie)"

// DuplicateRequest names the copy of a deck. An empty Name is the source's
// name plus a suffix, an empty Visibility is private.
type DuplicateRequest struct {
	Name       string
	FolderID   *int
	Visibility string
}

type DuplicateResult struct {
	Deck      Deck
	Placement PlacementSummary
}

// DuplicateDeck creates a deck for the user holding the content of a deck
// they can see, their own or a public or unlisted one, in one transaction.
// The copy uses the user's free owned copies first and pending cards for the
// rest. It keeps boards, commanders, finishes, proxy state and tags.
func (s *Service) DuplicateDeck(ctx context.Context, userID string, sourceID string, req DuplicateRequest) (DuplicateResult, error) {
	ownerID, source, err := s.findWithinReach(ctx, userID, sourceID, ReachVisible)
	if err != nil {
		return DuplicateResult{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = source.Name + copySuffix
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = VisibilityPrivate
	}

	var placement PlacementSummary
	created, err := s.repo.ComposeNewDeck(ctx, userID, Deck{
		FolderID:             req.FolderID,
		Name:                 name,
		Format:               source.Format,
		Visibility:           visibility,
		Bracket:              source.Bracket,
		BackgroundScryfallID: source.BackgroundScryfallID,
		BackgroundFace:       source.BackgroundFace,
	}, func(store CompositionStore, created Deck) error {
		content, err := readDeckContent(ctx, store, ownerID, sourceID)
		if err != nil {
			return err
		}
		tagRows, err := store.FindCardTags(ctx, ownerID, sourceID)
		if err != nil {
			return err
		}
		placement, err = placeCards(ctx, store, userID, created.ID, duplicationPlan(content.Entries, tagRows))
		return err
	})
	if err != nil {
		return DuplicateResult{}, err
	}
	return DuplicateResult{Deck: created, Placement: placement}, nil
}

// VisibleDeck is a deck the user can see: their own, or one shared by its owner.
type VisibleDeck struct {
	OwnerID string
	Deck    Deck
	Mine    bool
}

// GetVisibleDeck finds the user's own deck, or else a deck shared by its owner.
func (s *Service) GetVisibleDeck(ctx context.Context, userID string, deckID string) (VisibleDeck, error) {
	ownerID, d, err := s.findWithinReach(ctx, userID, deckID, ReachVisible)
	if err != nil {
		return VisibleDeck{}, err
	}
	return VisibleDeck{OwnerID: ownerID, Deck: d, Mine: userID != "" && ownerID == userID}, nil
}

func duplicationPlan(entries []DeckEntry, tagRows []CardTag) PlacementPlan {
	plan := PlacementPlan{Lines: make([]PlacementLine, 0, len(entries))}
	for _, e := range entries {
		proxy := RealOnly
		if e.Proxy {
			proxy = ProxyOnly
		}
		plan.Lines = append(plan.Lines, PlacementLine{
			Printing: Printing{
				Name: e.Name, ScryfallID: e.ScryfallID, SetCode: e.SetCode, CollectorNumber: e.CollectorNumber,
				ManaValue: e.ManaValue, Colors: e.Colors, CardType: e.CardType, ColorIdentity: e.ColorIdentity,
			},
			Foil:      e.Foil,
			Proxy:     proxy,
			Board:     e.Board,
			Quantity:  e.Quantity(),
			Commander: e.IsCommander(),
		})
	}
	index := make(map[string]int)
	for _, row := range tagRows {
		i, seen := index[row.CardName]
		if !seen {
			i = len(plan.Tags)
			index[row.CardName] = i
			plan.Tags = append(plan.Tags, TaggedCard{Name: row.CardName})
		}
		plan.Tags[i].Tags = append(plan.Tags[i].Tags, row.Tag)
	}
	return plan
}
