package deck

import (
	"context"
	"database/sql"
	"errors"
)

func (r *PostgresRepository) Compose(ctx context.Context, userID string, deckID string, apply func(CompositionStore) error) error {
	tx, err := r.pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var locked string
	if err := tx.GetContext(ctx, &locked, `SELECT id FROM tamiyo.deck WHERE id = $1 AND user_id = $2 FOR UPDATE`, deckID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := apply(&PostgresRepository{db: tx}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tamiyo.deck SET updated = now() WHERE id = $1 AND user_id = $2`, deckID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) ComposeCollection(ctx context.Context, apply func(CompositionStore) error) error {
	tx, err := r.pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := apply(&PostgresRepository{db: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) StorageExists(ctx context.Context, userID string, storageID int) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM tamiyo.storage WHERE id = $1 AND user_id = $2)`, storageID, userID)
	return exists, err
}

func (r *PostgresRepository) CreateCollectionCard(ctx context.Context, userID string, c DeckCard) (int, error) {
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
