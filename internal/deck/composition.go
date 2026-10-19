package deck

import "context"

// CompositionStore is what a composition operation reads and writes. Every
// call runs inside the transaction Composer.Compose opened, so the operation
// applies entirely or not at all.
type CompositionStore interface {
	FindByID(ctx context.Context, userID string, id string) (Deck, error)
	Update(ctx context.Context, userID string, d Deck) (Deck, error)
	FindCardsByDeckID(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]DeckCard, error)

	FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error)
	UpdatePendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error)
	DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error

	LinkCardToDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error

	FindCardTags(ctx context.Context, userID string, deckID string) ([]CardTag, error)
	FindDeckTags(ctx context.Context, userID string, deckID string) ([]string, error)
	WriteCardTags(ctx context.Context, userID string, deckID string, cardName string, tags []string) error

	StorageExists(ctx context.Context, userID string, storageID int) (bool, error)
	CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error)
	CreateCollectionCard(ctx context.Context, userID string, c DeckCard) (int, error)

	FindOwnedCopies(ctx context.Context, userID string, targetDeckID string, scryfallIDs []string, nameKeys []string) ([]OwnedCopy, error)
	UnlinkCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error
	CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error)
}

// Composer applies a change to one of the user's decks in a single
// transaction: it locks the deck (ErrNotFound when the user doesn't own it),
// runs apply, bumps the deck's updated date once and commits. Any error from
// apply rolls everything back.
type Composer interface {
	Compose(ctx context.Context, userID string, deckID string, apply func(CompositionStore) error) error
	// ComposeCollection runs apply in a single transaction that locks no
	// deck, for an operation that only adds to the user's collection.
	ComposeCollection(ctx context.Context, apply func(CompositionStore) error) error
}
