package deck

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// inTransaction runs run on a transaction's queries and commits it, or rolls
// it back when run fails.
func (r *PostgresRepository) inTransaction(ctx context.Context, run func(tx *sqlx.Tx, store *compositionQueries) error) error {
	tx, err := r.pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := run(tx, &compositionQueries{db: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) Compose(ctx context.Context, userID string, deckID string, apply func(CompositionStore) error) error {
	return r.inTransaction(ctx, func(tx *sqlx.Tx, store *compositionQueries) error {
		var locked string
		if err := tx.GetContext(ctx, &locked, `SELECT id FROM tamiyo.deck WHERE id = $1 AND user_id = $2 FOR UPDATE`, deckID, userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		tracked := &changeTracker{CompositionStore: store}
		if err := apply(tracked); err != nil {
			return err
		}
		if !tracked.changed {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE tamiyo.deck SET updated = now() WHERE id = $1 AND user_id = $2`, deckID, userID)
		return err
	})
}

func (r *PostgresRepository) ComposeCollection(ctx context.Context, apply func(CompositionStore) error) error {
	return r.inTransaction(ctx, func(_ *sqlx.Tx, store *compositionQueries) error {
		return apply(store)
	})
}

func (r *PostgresRepository) ComposeNewDeck(ctx context.Context, userID string, d Deck, apply func(CompositionStore, Deck) error) (Deck, error) {
	var created Deck
	err := r.inTransaction(ctx, func(_ *sqlx.Tx, store *compositionQueries) error {
		var err error
		if created, err = store.Create(ctx, userID, d); err != nil {
			return err
		}
		return apply(store, created)
	})
	if err != nil {
		return Deck{}, err
	}
	return created, nil
}

// changeTracker notes whether a composition wrote anything to the deck, so
// that Compose bumps the deck's updated date only then.
type changeTracker struct {
	CompositionStore
	changed bool
}

func (t *changeTracker) Update(ctx context.Context, userID string, d Deck) (Deck, error) {
	t.changed = true
	return t.CompositionStore.Update(ctx, userID, d)
}

func (t *changeTracker) UpdatePendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error) {
	t.changed = true
	return t.CompositionStore.UpdatePendingQuantity(ctx, userID, deckID, id, quantity)
}

func (t *changeTracker) DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error {
	t.changed = true
	return t.CompositionStore.DeletePendingCard(ctx, userID, deckID, id)
}

func (t *changeTracker) CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error) {
	t.changed = true
	return t.CompositionStore.CreatePendingCard(ctx, userID, p)
}

func (t *changeTracker) LinkCardToDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error {
	t.changed = true
	return t.CompositionStore.LinkCardToDeck(ctx, userID, deckID, cardID, board)
}

func (t *changeTracker) UnlinkCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error {
	t.changed = true
	return t.CompositionStore.UnlinkCardFromDeck(ctx, userID, deckID, cardID)
}

func (t *changeTracker) WriteCardTags(ctx context.Context, userID string, deckID string, cardName string, tags []string) error {
	t.changed = true
	return t.CompositionStore.WriteCardTags(ctx, userID, deckID, cardName, tags)
}

func (r *compositionQueries) StorageExists(ctx context.Context, userID string, storageID int) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM tamiyo.storage WHERE id = $1 AND user_id = $2)`, storageID, userID)
	return exists, err
}

func (r *compositionQueries) CreateCollectionCard(ctx context.Context, userID string, c DeckCard) (int, error) {
	var id int
	err := r.db.GetContext(ctx, &id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, userID, c.Name, c.ScryfallID, c.SetCode, c.CollectorNumber, c.Foil, c.Proxy, c.StorageID, c.ManaValue, c.Colors, c.CardType, c.ColorIdentity)
	if isForeignKeyViolation(err) {
		return 0, ErrTargetStorageNotFound
	}
	return id, err
}
