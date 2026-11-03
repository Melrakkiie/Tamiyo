package card

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// importBatchSize keeps a statement far under Postgres' 65535 bind parameters.
const importBatchSize = 1000

// ImportCollection creates the plan's storages, decks, every copy and its deck link in one transaction, in grouped inserts.
func (r *PostgresRepository) ImportCollection(ctx context.Context, userID string, plan ImportPlan) (ImportResult, error) {
	if len(plan.Copies) == 0 && len(plan.Storages) == 0 && len(plan.Decks) == 0 {
		return ImportResult{}, nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := importPlan(ctx, tx, userID, plan)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ImportResult{}, ctxErr
		}
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return ImportResult{}, ErrStorageNotFound
		}
		return ImportResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func importPlan(ctx context.Context, tx *sqlx.Tx, userID string, plan ImportPlan) (ImportResult, error) {
	result := ImportResult{Cards: make([]Card, 0, len(plan.Copies))}
	for _, st := range plan.Storages {
		var id int
		if err := tx.GetContext(ctx, &id, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, $2, $3) RETURNING id`, userID, st.Name, st.Type); err != nil {
			return ImportResult{}, err
		}
		result.StorageIDs = append(result.StorageIDs, id)
	}

	for _, d := range plan.Decks {
		var id string
		// Same columns as the deck module's insert; the other columns have database defaults.
		if err := tx.GetContext(ctx, &id, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, $2, $3) RETURNING id`, userID, d.Name, d.Format); err != nil {
			return ImportResult{}, err
		}
		result.DeckIDs = append(result.DeckIDs, id)
	}

	copies := make([]ImportCopy, len(plan.Copies))
	copy(copies, plan.Copies)
	for i, c := range copies {
		if c.NewStorage != nil {
			id := result.StorageIDs[*c.NewStorage]
			copies[i].Card.StorageID = &id
		}
		if c.NewDeck != nil {
			id := result.DeckIDs[*c.NewDeck]
			copies[i].DeckID = &id
		}
	}
	for start := 0; start < len(copies); start += importBatchSize {
		end := min(start+importBatchSize, len(copies))
		created, err := insertCopies(ctx, tx, userID, copies[start:end])
		if err != nil {
			return ImportResult{}, err
		}
		result.Cards = append(result.Cards, created...)
	}
	if err := linkCopiesToDecks(ctx, tx, userID, copies, result.Cards); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

// linkCopiesToDecks puts each copy that names a deck on its main board. card_deck has no
// ownership trigger, so the join on deck.user_id is the check: a missing or foreign deck
// links fewer rows than expected and the whole import rolls back.
func linkCopiesToDecks(ctx context.Context, tx *sqlx.Tx, userID string, copies []ImportCopy, created []Card) error {
	var cardIDs []int
	var deckIDs []string
	for i, c := range copies {
		if c.DeckID != nil {
			cardIDs = append(cardIDs, created[i].ID)
			deckIDs = append(deckIDs, *c.DeckID)
		}
	}
	if len(cardIDs) == 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO tamiyo.card_deck (card_id, deck_id, board)
		SELECT t.card_id, t.deck_id, 'main'
		FROM unnest($2::int[], $3::uuid[]) AS t(card_id, deck_id)
		JOIN tamiyo.deck d ON d.id = t.deck_id AND d.user_id = $1`,
		userID, pq.Array(cardIDs), pq.Array(deckIDs))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if int(n) != len(cardIDs) {
		return ErrDeckNotFound
	}
	return nil
}

// insertCopies inserts one batch and returns its cards in input order. Ids are drawn
// from the sequence first so each returned row maps back to its input position
// explicitly, without relying on the order of RETURNING.
func insertCopies(ctx context.Context, tx *sqlx.Tx, userID string, copies []ImportCopy) ([]Card, error) {
	n := len(copies)
	names, scryfallIDs := make([]string, n), make([]string, n)
	setCodes, collectorNumbers := make([]string, n), make([]string, n)
	foils, proxies := make([]bool, n), make([]bool, n)
	manaValues := make([]float64, n)
	storageIDs := make([]sql.NullInt64, n)
	colors, cardTypes, identities := make([]sql.NullString, n), make([]sql.NullString, n), make([]sql.NullString, n)
	for i, c := range copies {
		names[i], scryfallIDs[i] = c.Card.Name, c.Card.ScryfallID
		setCodes[i], collectorNumbers[i] = c.Card.SetCode, c.Card.CollectorNumber
		foils[i], proxies[i], manaValues[i] = c.Card.Foil, c.Card.Proxy, c.Card.ManaValue
		if c.Card.StorageID != nil {
			storageIDs[i] = sql.NullInt64{Int64: int64(*c.Card.StorageID), Valid: true}
		}
		colors[i], cardTypes[i], identities[i] = nullString(c.Card.Colors), nullString(c.Card.CardType), nullString(c.Card.ColorIdentity)
	}

	rows, err := tx.QueryxContext(ctx, `
		WITH input AS MATERIALIZED (
			SELECT nextval(pg_get_serial_sequence('tamiyo.cards', 'id'))::int AS id, t.*
			FROM unnest($2::text[], $3::uuid[], $4::text[], $5::text[], $6::bool[], $7::bool[], $8::int[], $9::float8[], $10::text[], $11::text[], $12::text[])
				WITH ORDINALITY AS t(name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, ord)
		), inserted AS (
			INSERT INTO tamiyo.cards (id, user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity)
			SELECT id, $1, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity FROM input
			RETURNING id, added, updated
		)
		SELECT input.ord, inserted.id, inserted.added, inserted.updated
		FROM inserted JOIN input ON input.id = inserted.id
		ORDER BY input.ord`,
		userID, pq.Array(names), pq.Array(scryfallIDs), pq.Array(setCodes), pq.Array(collectorNumbers),
		pq.Array(foils), pq.Array(proxies), pq.GenericArray{A: storageIDs}, pq.Array(manaValues),
		pq.GenericArray{A: colors}, pq.GenericArray{A: cardTypes}, pq.GenericArray{A: identities},
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	created := make([]Card, 0, n)
	for rows.Next() {
		var ord int
		var c Card
		if err := rows.Scan(&ord, &c.ID, &c.Added, &c.Updated); err != nil {
			return nil, err
		}
		in := copies[ord-1].Card
		in.ID, in.Added, in.Updated = c.ID, c.Added, c.Updated
		created = append(created, in)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return created, nil
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
