package printing

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) IDsToRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]string, error) {
	var ids []string
	err := r.db.SelectContext(ctx, &ids, `
		SELECT c.scryfall_id::text
		FROM (
			SELECT scryfall_id FROM tamiyo.cards
			UNION
			SELECT scryfall_id FROM tamiyo.deck_pending_cards
		) c
		LEFT JOIN tamiyo.printings p ON p.scryfall_id = c.scryfall_id
		WHERE p.scryfall_id IS NULL OR p.refreshed_at < $1
		ORDER BY p.refreshed_at NULLS FIRST, c.scryfall_id
		LIMIT $2
	`, staleBefore, limit)
	return ids, err
}

func (r *PostgresRepository) Upsert(ctx context.Context, printings []Printing) error {
	if len(printings) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	for _, p := range printings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tamiyo.printings (scryfall_id, name, type_line, legal_formats, powers, toughnesses, eur, eur_foil, usd, usd_foil, usd_etched,
				oracle_text, mana_cost, rarity, keywords, mana_value, colors, color_identity, banned_formats, restricted_formats, game_changer, refreshed_at)
			VALUES ($1, $2, $3, $4, COALESCE($5::text[], '{}'), COALESCE($6::text[], '{}'), $7, $8, $9, $10, $11,
				NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), COALESCE($15::text[], '{}'), $16,
				COALESCE($17::text[], '{}'), COALESCE($18::text[], '{}'), COALESCE($19::text[], '{}'), COALESCE($20::text[], '{}'), $21, now())
			ON CONFLICT (scryfall_id) DO UPDATE
			SET name = EXCLUDED.name, type_line = EXCLUDED.type_line, legal_formats = EXCLUDED.legal_formats,
				powers = EXCLUDED.powers, toughnesses = EXCLUDED.toughnesses,
				eur = EXCLUDED.eur, eur_foil = EXCLUDED.eur_foil, usd = EXCLUDED.usd, usd_foil = EXCLUDED.usd_foil,
				usd_etched = EXCLUDED.usd_etched,
				oracle_text = EXCLUDED.oracle_text, mana_cost = EXCLUDED.mana_cost, rarity = EXCLUDED.rarity, keywords = EXCLUDED.keywords,
				mana_value = EXCLUDED.mana_value, colors = EXCLUDED.colors, color_identity = EXCLUDED.color_identity,
				banned_formats = EXCLUDED.banned_formats, restricted_formats = EXCLUDED.restricted_formats,
				game_changer = EXCLUDED.game_changer, refreshed_at = now()
		`, p.ScryfallID, p.Name, p.TypeLine, pq.Array(p.LegalFormats), pq.Array(p.Powers), pq.Array(p.Toughnesses),
			p.Prices.EUR, p.Prices.EURFoil, p.Prices.USD, p.Prices.USDFoil, p.Prices.USDEtched,
			p.OracleText, p.ManaCost, p.Rarity, pq.Array(p.Keywords), p.ManaValue,
			pq.Array(p.Colors), pq.Array(p.ColorIdentity), pq.Array(p.BannedFormats), pq.Array(p.RestrictedFormats), p.GameChanger); err != nil {
			return err
		}
		if p.Name == "" {
			continue
		}
		if err := renameCards(ctx, tx, p.ScryfallID, p.Name); err != nil {
			return err
		}
		if err := setManaValue(ctx, tx, p.ScryfallID, p.ManaValue); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) MarkRefreshed(ctx context.Context, ids []string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.printings (scryfall_id, type_line)
		SELECT id, '' FROM unnest($1::uuid[]) AS id
		ON CONFLICT (scryfall_id) DO UPDATE SET refreshed_at = now()
	`, pq.Array(ids))
	return err
}

func setManaValue(ctx context.Context, tx *sqlx.Tx, scryfallID string, manaValue float64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE tamiyo.cards SET mana_value = $2 WHERE scryfall_id = $1 AND mana_value <> $2`, scryfallID, manaValue); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE tamiyo.deck_pending_cards SET mana_value = $2 WHERE scryfall_id = $1 AND mana_value <> $2`, scryfallID, manaValue)
	return err
}

type deckName struct {
	DeckID string `db:"deck_id"`
	Name   string `db:"name"`
}

type deckKey struct {
	deckID string
	key    string
}

func renameCards(ctx context.Context, tx *sqlx.Tx, scryfallID string, name string) error {
	var renamed []deckName
	if err := tx.SelectContext(ctx, &renamed, `
		SELECT cd.deck_id::text AS deck_id, c.name
		FROM tamiyo.cards c
		JOIN tamiyo.card_deck cd ON cd.card_id = c.id
		WHERE c.scryfall_id = $1 AND c.name <> $2
		UNION
		SELECT deck_id::text, name
		FROM tamiyo.deck_pending_cards
		WHERE scryfall_id = $1 AND name <> $2
	`, scryfallID, name); err != nil {
		return err
	}

	newKey := cardNameKey(name)
	moved := make(map[deckKey]bool)
	for _, r := range renamed {
		old := deckKey{deckID: r.DeckID, key: cardNameKey(r.Name)}
		if old.key == newKey || moved[old] {
			continue
		}
		moved[old] = true
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tamiyo.deck_card_tags (deck_id, card_name, tag)
			SELECT deck_id, $3, tag
			FROM tamiyo.deck_card_tags
			WHERE deck_id = $1 AND card_name = $2
			ON CONFLICT DO NOTHING
		`, old.deckID, old.key, newKey); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE tamiyo.cards SET name = $2 WHERE scryfall_id = $1 AND name <> $2`, scryfallID, name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tamiyo.deck_pending_cards SET name = $2 WHERE scryfall_id = $1 AND name <> $2`, scryfallID, name); err != nil {
		return err
	}

	for old := range moved {
		var remaining []string
		if err := tx.SelectContext(ctx, &remaining, `
			SELECT c.name
			FROM tamiyo.cards c
			JOIN tamiyo.card_deck cd ON cd.card_id = c.id
			WHERE cd.deck_id = $1
			UNION
			SELECT name FROM tamiyo.deck_pending_cards WHERE deck_id = $1
		`, old.deckID); err != nil {
			return err
		}
		if containsKey(remaining, old.key) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tamiyo.deck_card_tags WHERE deck_id = $1 AND card_name = $2`, old.deckID, old.key); err != nil {
			return err
		}
	}
	return nil
}

func containsKey(names []string, key string) bool {
	for _, n := range names {
		if cardNameKey(n) == key {
			return true
		}
	}
	return false
}

func cardNameKey(name string) string {
	key := strings.ToLower(strings.ReplaceAll(name, "//", "/"))
	return strings.Join(strings.Fields(key), " ")
}
