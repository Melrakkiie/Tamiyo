package deck

import (
	"context"
	"strings"
)

// CopiesWanted names the printings (by Scryfall id) and the cards (by full or
// front face name) whose owned copies are looked for.
type CopiesWanted struct {
	ScryfallIDs []string
	Names       []string
}

type OwnedCopy struct {
	ID         int
	Name       string
	ScryfallID string
	Foil       bool
	Proxy      bool

	InDeck              bool
	InTargetDeck        bool
	LockedAgainstTarget bool
}

type ownedCopyFinder interface {
	FindOwnedCopies(ctx context.Context, userID string, targetDeckID string, scryfallIDs []string, nameKeys []string) ([]OwnedCopy, error)
}

func findOwnedCopies(ctx context.Context, finder ownedCopyFinder, userID string, targetDeckID string, wanted CopiesWanted) ([]OwnedCopy, error) {
	scryfallIDs := make([]string, 0, len(wanted.ScryfallIDs))
	for _, id := range wanted.ScryfallIDs {
		scryfallIDs = append(scryfallIDs, strings.ToLower(id))
	}
	nameKeys := make([]string, 0, len(wanted.Names))
	for _, name := range wanted.Names {
		nameKeys = append(nameKeys, CardNameKey(name))
	}
	if len(scryfallIDs) == 0 && len(nameKeys) == 0 {
		return nil, nil
	}
	return finder.FindOwnedCopies(ctx, userID, targetDeckID, scryfallIDs, nameKeys)
}
