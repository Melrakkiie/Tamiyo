package deck

import (
	"context"
	"fmt"
)

func (r *PostgresRepository) Like(ctx context.Context, userID string, deckID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.deck_likes (user_id, deck_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, deckID)
	return err
}

func (r *PostgresRepository) Unlike(ctx context.Context, userID string, deckID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tamiyo.deck_likes WHERE user_id = $1 AND deck_id = $2`, userID, deckID)
	return err
}

func (r *PostgresRepository) FindLikeStatus(ctx context.Context, userID string, deckID string) (LikeStatus, error) {
	var row struct {
		Count     int  `db:"count"`
		LikedByMe bool `db:"liked_by_me"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT
			(SELECT count(*) FROM tamiyo.deck_likes WHERE deck_id = $2) AS count,
			EXISTS (SELECT 1 FROM tamiyo.deck_likes WHERE deck_id = $2 AND user_id = $1) AS liked_by_me
	`, userID, deckID)
	if err != nil {
		return LikeStatus{}, fmt.Errorf("loading likes: %w", err)
	}
	return LikeStatus(row), nil
}

func (r *PostgresRepository) FindLiked(ctx context.Context, userID string, page int, limit int) ([]PublicDeck, int, error) {
	query := publicDecksQuery(
		"(SELECT l.added FROM tamiyo.deck_likes l WHERE l.deck_id = d.id AND l.user_id = $1)",
		"d.visibility IN ('public', 'unlisted') AND d.user_id <> $1 AND EXISTS (SELECT 1 FROM tamiyo.deck_likes l WHERE l.deck_id = d.id AND l.user_id = $1)",
	)

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT count(*) FROM ("+query+") liked", userID); err != nil {
		return nil, 0, fmt.Errorf("counting liked decks: %w", err)
	}

	var rows []publicDeckRow
	query += " ORDER BY liked_at DESC, id DESC LIMIT $2 OFFSET $3"
	if err := r.db.SelectContext(ctx, &rows, query, userID, limit, (page-1)*limit); err != nil {
		return nil, 0, fmt.Errorf("loading liked decks: %w", err)
	}
	decks := make([]PublicDeck, 0, len(rows))
	for _, row := range rows {
		decks = append(decks, toPublicDeck(row))
	}
	return decks, total, nil
}
