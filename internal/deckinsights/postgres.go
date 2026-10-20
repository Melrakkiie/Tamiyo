package deckinsights

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// PostgresRepository reads what deck insights need from the database.
type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// DeckValues holds a deck's values before rounding, priced by tamiyo.card_price.
type DeckValues struct {
	Owned       Value
	Pending     Value
	Sideboard   Value
	Considering Value
}

// DeckValues sums the price of every non-proxy copy in the deck, owned and
// pending (counted with their quantity): owned and pending on the main board,
// and each of the side boards.
func (r *PostgresRepository) DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error) {
	var row struct {
		OwnedEUR       float64 `db:"owned_eur"`
		OwnedUSD       float64 `db:"owned_usd"`
		PendingEUR     float64 `db:"pending_eur"`
		PendingUSD     float64 `db:"pending_usd"`
		SideboardEUR   float64 `db:"sideboard_eur"`
		SideboardUSD   float64 `db:"sideboard_usd"`
		ConsideringEUR float64 `db:"considering_eur"`
		ConsideringUSD float64 `db:"considering_usd"`
	}
	err := r.db.GetContext(ctx, &row, `
		WITH copies AS (
			SELECT cd.board, c.scryfall_id, c.foil, 1 AS quantity, false AS pending
			FROM tamiyo.cards c
			JOIN tamiyo.card_deck cd ON cd.card_id = c.id
			WHERE c.user_id = $1 AND cd.deck_id = $2 AND NOT c.proxy
			UNION ALL
			SELECT p.board, p.scryfall_id, p.foil, p.quantity, true AS pending
			FROM tamiyo.deck_pending_cards p
			WHERE p.user_id = $1 AND p.deck_id = $2 AND NOT p.proxy
		),
		priced AS (
			SELECT board, pending,
				COALESCE(tamiyo.card_price(scryfall_id, foil, 'eur'), 0) * quantity AS eur,
				COALESCE(tamiyo.card_price(scryfall_id, foil, 'usd'), 0) * quantity AS usd
			FROM copies
		)
		SELECT
			COALESCE(SUM(eur) FILTER (WHERE board NOT IN ('sideboard', 'considering') AND NOT pending), 0)::float8 AS owned_eur,
			COALESCE(SUM(usd) FILTER (WHERE board NOT IN ('sideboard', 'considering') AND NOT pending), 0)::float8 AS owned_usd,
			COALESCE(SUM(eur) FILTER (WHERE board NOT IN ('sideboard', 'considering') AND pending), 0)::float8 AS pending_eur,
			COALESCE(SUM(usd) FILTER (WHERE board NOT IN ('sideboard', 'considering') AND pending), 0)::float8 AS pending_usd,
			COALESCE(SUM(eur) FILTER (WHERE board = 'sideboard'), 0)::float8 AS sideboard_eur,
			COALESCE(SUM(usd) FILTER (WHERE board = 'sideboard'), 0)::float8 AS sideboard_usd,
			COALESCE(SUM(eur) FILTER (WHERE board = 'considering'), 0)::float8 AS considering_eur,
			COALESCE(SUM(usd) FILTER (WHERE board = 'considering'), 0)::float8 AS considering_usd
		FROM priced
	`, userID, deckID)
	if err != nil {
		return DeckValues{}, fmt.Errorf("summing deck values: %w", err)
	}
	return DeckValues{
		Owned:       Value{EUR: row.OwnedEUR, USD: row.OwnedUSD},
		Pending:     Value{EUR: row.PendingEUR, USD: row.PendingUSD},
		Sideboard:   Value{EUR: row.SideboardEUR, USD: row.SideboardUSD},
		Considering: Value{EUR: row.ConsideringEUR, USD: row.ConsideringUSD},
	}, nil
}
