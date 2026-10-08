package deck

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type pendingRow struct {
	ID              int       `db:"id"`
	UserID          string    `db:"user_id"`
	DeckID          string    `db:"deck_id"`
	Name            string    `db:"name"`
	ScryfallID      string    `db:"scryfall_id"`
	SetCode         string    `db:"set_code"`
	CollectorNumber string    `db:"collector_number"`
	Foil            bool      `db:"foil"`
	Quantity        int       `db:"quantity"`
	ManaValue       float64   `db:"mana_value"`
	Colors          *string   `db:"colors"`
	CardType        *string   `db:"card_type"`
	ColorIdentity   *string   `db:"color_identity"`
	Added           time.Time `db:"added"`
}

func (r pendingRow) toDomain() PendingCard {
	return PendingCard{
		ID:              r.ID,
		DeckID:          r.DeckID,
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		Quantity:        r.Quantity,
		ManaValue:       r.ManaValue,
		Colors:          r.Colors,
		CardType:        r.CardType,
		ColorIdentity:   r.ColorIdentity,
		Added:           r.Added,
	}
}

const pendingColumns = `id, deck_id, name, scryfall_id, set_code, collector_number, foil, quantity, mana_value, colors, card_type, color_identity, added`

func (r *PostgresRepository) FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error) {
	var rows []pendingRow
	query := `SELECT ` + pendingColumns + ` FROM tamiyo.deck_pending_cards WHERE user_id = $1 AND deck_id = $2 ORDER BY name, id`
	if err := r.db.SelectContext(ctx, &rows, query, userID, deckID); err != nil {
		return nil, err
	}
	cards := make([]PendingCard, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, row.toDomain())
	}
	return cards, nil
}

func (r *PostgresRepository) CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error) {
	row := pendingRow{
		UserID:          userID,
		DeckID:          p.DeckID,
		Name:            p.Name,
		ScryfallID:      p.ScryfallID,
		SetCode:         p.SetCode,
		CollectorNumber: p.CollectorNumber,
		Foil:            p.Foil,
		Quantity:        p.Quantity,
		ManaValue:       p.ManaValue,
		Colors:          p.Colors,
		CardType:        p.CardType,
		ColorIdentity:   p.ColorIdentity,
	}
	query := `
		INSERT INTO tamiyo.deck_pending_cards (user_id, deck_id, name, scryfall_id, set_code, collector_number, foil, quantity, mana_value, colors, card_type, color_identity)
		VALUES (:user_id, :deck_id, :name, :scryfall_id, :set_code, :collector_number, :foil, :quantity, :mana_value, :colors, :card_type, :color_identity)
		RETURNING ` + pendingColumns

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return PendingCard{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

	var created pendingRow
	if err := stmt.GetContext(ctx, &created, row); err != nil {
		return PendingCard{}, err
	}
	return created.toDomain(), nil
}

func (r *PostgresRepository) UpdatePendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error) {
	var row pendingRow
	query := `UPDATE tamiyo.deck_pending_cards SET quantity = $1 WHERE id = $2 AND deck_id = $3 AND user_id = $4 RETURNING ` + pendingColumns
	if err := r.db.GetContext(ctx, &row, query, quantity, id, deckID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PendingCard{}, ErrPendingCardNotFound
		}
		return PendingCard{}, err
	}
	return row.toDomain(), nil
}

func (r *PostgresRepository) DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tamiyo.deck_pending_cards WHERE id = $1 AND deck_id = $2 AND user_id = $3`, id, deckID, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPendingCardNotFound
	}
	return nil
}
