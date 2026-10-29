package deckinsights

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"Melrakkiie/Tamiyo/internal/deck"
)

// PostgresRepository reads what deck insights need from the database.
type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Printings returns the stored printing of each of scryfallIDs, keyed by
// Scryfall id. A printing not stored yet, or stored as a placeholder because
// Scryfall didn't return it (empty type line), is left out: it is unknown.
func (r *PostgresRepository) Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error) {
	byID := make(map[string]Printing, len(scryfallIDs))
	if len(scryfallIDs) == 0 {
		return byID, nil
	}
	var rows []struct {
		ScryfallID        string         `db:"scryfall_id"`
		Name              string         `db:"name"`
		TypeLine          string         `db:"type_line"`
		OracleText        string         `db:"oracle_text"`
		Keywords          pq.StringArray `db:"keywords"`
		ManaValue         float64        `db:"mana_value"`
		Colors            pq.StringArray `db:"colors"`
		ColorIdentity     pq.StringArray `db:"color_identity"`
		LegalFormats      pq.StringArray `db:"legal_formats"`
		BannedFormats     pq.StringArray `db:"banned_formats"`
		RestrictedFormats pq.StringArray `db:"restricted_formats"`
	}
	err := r.db.SelectContext(ctx, &rows, `
		SELECT scryfall_id::text AS scryfall_id, name, type_line, COALESCE(oracle_text, '') AS oracle_text, keywords,
			mana_value::float8 AS mana_value, colors, color_identity, legal_formats, banned_formats, restricted_formats
		FROM tamiyo.printings
		WHERE scryfall_id = ANY($1::uuid[]) AND type_line <> ''
	`, pq.Array(scryfallIDs))
	if err != nil {
		return nil, fmt.Errorf("reading printings: %w", err)
	}
	for _, row := range rows {
		byID[row.ScryfallID] = Printing{
			ScryfallID: row.ScryfallID, Name: row.Name, TypeLine: row.TypeLine, OracleText: row.OracleText,
			Keywords: row.Keywords, ManaValue: row.ManaValue, Colors: row.Colors, ColorIdentity: row.ColorIdentity,
			LegalFormats: row.LegalFormats, BannedFormats: row.BannedFormats, RestrictedFormats: row.RestrictedFormats,
		}
	}
	return byID, nil
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
	mainBoard := deck.MainBoardSQL("")
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
			COALESCE(SUM(eur) FILTER (WHERE `+mainBoard+` AND NOT pending), 0)::float8 AS owned_eur,
			COALESCE(SUM(usd) FILTER (WHERE `+mainBoard+` AND NOT pending), 0)::float8 AS owned_usd,
			COALESCE(SUM(eur) FILTER (WHERE `+mainBoard+` AND pending), 0)::float8 AS pending_eur,
			COALESCE(SUM(usd) FILTER (WHERE `+mainBoard+` AND pending), 0)::float8 AS pending_usd,
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

// DeckHistoryRows returns every stored balance of the deck, unordered.
func (r *PostgresRepository) DeckHistoryRows(ctx context.Context, deckID string) ([]HistoryRow, error) {
	var rows []HistoryRow
	err := r.db.SelectContext(ctx, &rows, `
		SELECT day::text AS date, board, name, count
		FROM tamiyo.deck_history
		WHERE deck_id = $1
	`, deckID)
	if err != nil {
		return nil, fmt.Errorf("reading deck history: %w", err)
	}
	return rows, nil
}
