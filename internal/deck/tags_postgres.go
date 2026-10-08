package deck

import (
	"context"
	"database/sql"
)

func (r *PostgresRepository) FindCardTags(ctx context.Context, userID string, deckID string) ([]CardTag, error) {
	var rows []struct {
		CardName string `db:"card_name"`
		Tag      string `db:"tag"`
	}
	query := `
		SELECT t.card_name, t.tag
		FROM tamiyo.deck_card_tags t
		JOIN tamiyo.deck d ON d.id = t.deck_id
		WHERE d.user_id = $1 AND t.deck_id = $2
		ORDER BY t.card_name, t.tag
	`
	if err := r.db.SelectContext(ctx, &rows, query, userID, deckID); err != nil {
		return nil, err
	}
	tags := make([]CardTag, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, CardTag{CardName: row.CardName, Tag: row.Tag})
	}
	return tags, nil
}

func (r *PostgresRepository) ReplaceCardTags(ctx context.Context, userID string, deckID string, cardName string, tags []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var owned bool
	if err := tx.GetContext(ctx, &owned, `SELECT EXISTS (SELECT 1 FROM tamiyo.deck WHERE id = $1 AND user_id = $2)`, deckID, userID); err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tamiyo.deck_card_tags WHERE deck_id = $1 AND card_name = $2`, deckID, cardName); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tamiyo.deck_card_tags (deck_id, card_name, tag) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			deckID, cardName, tag,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) RenameTag(ctx context.Context, userID string, deckID string, from string, to string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tamiyo.deck_card_tags (deck_id, card_name, tag)
		SELECT t.deck_id, t.card_name, $4
		FROM tamiyo.deck_card_tags t
		JOIN tamiyo.deck d ON d.id = t.deck_id
		WHERE d.user_id = $1 AND t.deck_id = $2 AND t.tag = $3
		ON CONFLICT DO NOTHING
	`, userID, deckID, from, to); err != nil {
		return err
	}
	if err := deleteTag(ctx, tx, userID, deckID, from); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) DeleteTag(ctx context.Context, userID string, deckID string, tag string) error {
	return deleteTag(ctx, r.db, userID, deckID, tag)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func deleteTag(ctx context.Context, db execer, userID string, deckID string, tag string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM tamiyo.deck_card_tags t
		USING tamiyo.deck d
		WHERE d.id = t.deck_id AND d.user_id = $1 AND t.deck_id = $2 AND t.tag = $3
	`, userID, deckID, tag)
	return err
}
