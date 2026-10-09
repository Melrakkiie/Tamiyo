package deck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type publicDeckRow struct {
	ID                   string         `db:"id"`
	Name                 string         `db:"name"`
	Format               string         `db:"format"`
	BackgroundScryfallID *string        `db:"background_scryfall_id"`
	CommanderScryfallID  *string        `db:"commander_scryfall_id"`
	CommanderName        *string        `db:"commander_name"`
	Identity             pq.StringArray `db:"identity"`
	CardCount            int            `db:"card_count"`
	OwnerID              string         `db:"user_id"`
	OwnerDisplayName     *string        `db:"display_name"`
	OwnerAvatarID        *string        `db:"avatar_scryfall_id"`
	Added                time.Time      `db:"added"`
	Updated              time.Time      `db:"updated"`
}

const identityLettersSQL = `ARRAY(SELECT DISTINCT letter FROM regexp_split_to_table(upper(coalesce(%s, '')), '') AS letter WHERE letter IN ('W', 'U', 'B', 'R', 'G'))`

const publicDecksSQL = `
	WITH base AS (
		SELECT d.id, d.user_id, d.name, d.format, d.background_scryfall_id, d.added, d.updated,
			u.display_name, u.avatar_scryfall_id,
			coalesce(cc.scryfall_id, cp.scryfall_id) AS commander_scryfall_id,
			coalesce(cc.name, cp.name) AS commander_name,
			coalesce(cc.color_identity, cp.color_identity) AS commander_identity,
			(SELECT count(*) FROM tamiyo.card_deck cd WHERE cd.deck_id = d.id AND cd.board = 'main')
				+ (SELECT coalesce(sum(p.quantity), 0) FROM tamiyo.deck_pending_cards p WHERE p.deck_id = d.id AND p.board = 'main') AS card_count,
			(
				SELECT string_agg(identity, '') FROM (
					SELECT c.color_identity AS identity FROM tamiyo.card_deck cd JOIN tamiyo.cards c ON c.id = cd.card_id
					WHERE cd.deck_id = d.id AND cd.board = 'main'
					UNION ALL
					SELECT p.color_identity FROM tamiyo.deck_pending_cards p WHERE p.deck_id = d.id AND p.board = 'main'
				) identities
			) AS cards_identity
		FROM tamiyo.deck d
		JOIN tamiyo.users u ON u.id = d.user_id
		LEFT JOIN tamiyo.cards cc ON cc.id = d.commander_id
		LEFT JOIN tamiyo.deck_pending_cards cp ON cp.id = d.commander_pending_id
		WHERE d.visibility = 'public'
	), decks AS (
		SELECT base.*,
			CASE WHEN commander_identity IS NOT NULL
				THEN ` + "%s" + `
				ELSE ` + "%s" + `
			END AS identity
		FROM base
	)
	SELECT id, user_id, name, format, background_scryfall_id, added, updated, display_name, avatar_scryfall_id,
		commander_scryfall_id, commander_name, card_count, identity
	FROM decks
`

func containsSQL(column string, pos int) string {
	return fmt.Sprintf("strpos(lower(coalesce(%s, '')), lower($%d)) > 0", column, pos)
}

func publicOrderBy(filter PublicFilter) string {
	dir := "ASC"
	if filter.SortDesc {
		dir = "DESC"
	}
	switch filter.SortField {
	case "name":
		return fmt.Sprintf(" ORDER BY lower(name) %s, id %s", dir, dir)
	case "added":
		return fmt.Sprintf(" ORDER BY added %s, id %s", dir, dir)
	case "card_count":
		return fmt.Sprintf(" ORDER BY card_count %s, updated DESC, id DESC", dir)
	default:
		return fmt.Sprintf(" ORDER BY updated %s, id %s", dir, dir)
	}
}

func (r *PostgresRepository) FindPublic(ctx context.Context, filter PublicFilter) ([]PublicDeck, int, error) {
	var conditions []string
	var args []interface{}
	add := func(condition func(pos int) string, value interface{}) {
		args = append(args, value)
		conditions = append(conditions, condition(len(args)))
	}

	if filter.Name != "" {
		add(func(pos int) string { return containsSQL("name", pos) }, filter.Name)
	}
	if filter.Format != "" {
		add(func(pos int) string { return fmt.Sprintf("lower(format) = lower($%d)", pos) }, filter.Format)
	}
	if filter.Commander != "" {
		add(func(pos int) string { return containsSQL("commander_name", pos) }, filter.Commander)
	}
	if filter.Owner != "" {
		add(func(pos int) string { return containsSQL("display_name", pos) }, filter.Owner)
	}
	if filter.Card != "" {
		add(func(pos int) string {
			return fmt.Sprintf(`(EXISTS (
				SELECT 1 FROM tamiyo.card_deck cd JOIN tamiyo.cards c ON c.id = cd.card_id
				WHERE cd.deck_id = decks.id AND cd.board = 'main' AND %s
			) OR EXISTS (
				SELECT 1 FROM tamiyo.deck_pending_cards p
				WHERE p.deck_id = decks.id AND p.board = 'main' AND %s
			))`, containsSQL("c.name", pos), containsSQL("p.name", pos))
		}, filter.Card)
	}
	if filter.Colorless {
		conditions = append(conditions, "cardinality(identity) = 0")
	} else if len(filter.Colors) > 0 {
		add(func(pos int) string {
			switch filter.ColorMode {
			case ColorModeInclude:
				return fmt.Sprintf("identity @> $%d::text[]", pos)
			case ColorModeWithin:
				return fmt.Sprintf("identity <@ $%d::text[]", pos)
			default:
				return fmt.Sprintf("(identity @> $%d::text[] AND identity <@ $%d::text[])", pos, pos)
			}
		}, pq.Array(filter.Colors))
	}
	if filter.ColorCount != nil {
		add(func(pos int) string { return fmt.Sprintf("cardinality(identity) = $%d", pos) }, *filter.ColorCount)
	}

	query := fmt.Sprintf(publicDecksSQL,
		fmt.Sprintf(identityLettersSQL, "commander_identity"),
		fmt.Sprintf(identityLettersSQL, "cards_identity"),
	)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT count(*) FROM ("+query+") filtered", args...); err != nil {
		return nil, 0, err
	}

	query += publicOrderBy(filter)
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	var rows []publicDeckRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, err
	}

	decks := make([]PublicDeck, 0, len(rows))
	for _, row := range rows {
		decks = append(decks, PublicDeck{
			ID:                   row.ID,
			Name:                 row.Name,
			Format:               row.Format,
			BackgroundScryfallID: row.BackgroundScryfallID,
			CommanderScryfallID:  row.CommanderScryfallID,
			CommanderName:        row.CommanderName,
			ColorIdentity:        scryfall.ColorCode(row.Identity),
			CardCount:            row.CardCount,
			OwnerID:              row.OwnerID,
			OwnerDisplayName:     row.OwnerDisplayName,
			OwnerAvatarID:        row.OwnerAvatarID,
			Added:                row.Added,
			Updated:              row.Updated,
		})
	}
	return decks, total, nil
}
