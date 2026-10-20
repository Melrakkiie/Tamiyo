//go:build integration

package card

import (
	"context"
	"net/url"
	"sort"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// criterionFixture is the collection every criterion row runs against:
// multicolored and colorless cards, a double-faced card with power on the back,
// cards with no printing row yet, artifact and enchantment creatures, foil and
// non-foil copies, and cards in an open binder and a locked deckbox.
type criterionFixture struct {
	repo        *PostgresRepository
	userID      string
	otherUserID string
	binderID    int
	deckboxID   int
	allowedDeck string
	otherDeck   string
}

type criterionSeed struct {
	name, scryfallID    string
	colors, cardType    *string
	typeLine            string
	manaValue           float64
	foil                bool
	storageID           *int
	legal               []string
	powers, toughnesses []string
}

func seedCriterionFixture(t *testing.T) criterionFixture {
	t.Helper()
	db := getTestDB(t)
	ctx := context.Background()
	f := criterionFixture{repo: NewPostgresRepository(db)}
	f.userID = seedUser(t, db, "alice@example.com")
	f.otherUserID = seedUser(t, db, "bob@example.com")

	require.NoError(t, db.Get(&f.binderID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Binder', 'binder') RETURNING id`, f.userID))
	require.NoError(t, db.Get(&f.deckboxID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Kess', 'Deckbox ') RETURNING id`, f.userID))
	require.NoError(t, db.Get(&f.allowedDeck, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Allowed', 'commander') RETURNING id`, f.userID))
	require.NoError(t, db.Get(&f.otherDeck, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Other', 'commander') RETURNING id`, f.userID))
	_, err := db.Exec(`UPDATE tamiyo.storage SET locked = true WHERE id = $1`, f.deckboxID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.storage_allowed_deck (storage_id, deck_id) VALUES ($1, $2)`, f.deckboxID, f.allowedDeck)
	require.NoError(t, err)

	var otherDeckboxID int
	require.NoError(t, db.Get(&otherDeckboxID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Bob box', 'deckbox') RETURNING id`, f.otherUserID))
	bobColors, bobType := "R", "Instant"
	_, err = f.repo.Create(ctx, f.otherUserID, Card{
		Name: "Bob's Bolt", ScryfallID: "99999999-9999-9999-9999-999999999999", SetCode: "tst", CollectorNumber: "1",
		StorageID: &otherDeckboxID, Colors: &bobColors, CardType: &bobType, ColorIdentity: &bobColors,
	})
	require.NoError(t, err)

	for _, s := range []criterionSeed{
		{name: "Llanowar Elves", scryfallID: "11111111-1111-1111-1111-111111111111", colors: strPtr("G"), cardType: strPtr("Creature"),
			typeLine: "Creature — Elf Druid", manaValue: 1, storageID: &f.binderID,
			legal: []string{"modern", "commander"}, powers: []string{"1"}, toughnesses: []string{"1"}},
		{name: "Lightning Helix", scryfallID: "22222222-2222-2222-2222-222222222222", colors: strPtr("WR"), cardType: strPtr("Instant"),
			typeLine: "Instant", manaValue: 2, storageID: &f.binderID, legal: []string{"modern", "commander"}},
		{name: "Lightning Helix", scryfallID: "22222222-2222-2222-2222-222222222222", colors: strPtr("WR"), cardType: strPtr("Instant"),
			manaValue: 2, foil: true, storageID: &f.deckboxID},
		{name: "Sol Ring", scryfallID: "33333333-3333-3333-3333-333333333333", colors: strPtr(""), cardType: strPtr("Artifact"),
			typeLine: "Artifact", manaValue: 1, storageID: &f.deckboxID, legal: []string{"commander"}},
		{name: "Esika's Chariot", scryfallID: "44444444-4444-4444-4444-444444444444", colors: strPtr("G"), cardType: strPtr("Artifact"),
			typeLine: "Legendary Artifact — Vehicle", manaValue: 4, foil: true,
			legal: []string{"commander", "pioneer"}, powers: []string{"4"}, toughnesses: []string{"4"}},
		{name: "Grist, the Hunger Tide", scryfallID: "55555555-5555-5555-5555-555555555555", colors: strPtr("BG"), cardType: strPtr("Planeswalker"),
			typeLine: "Legendary Planeswalker — Grist", manaValue: 3, storageID: &f.binderID, legal: []string{"modern"}},
		{name: "Delver of Secrets", scryfallID: "66666666-6666-6666-6666-666666666666", colors: strPtr("U"), cardType: strPtr("Creature"),
			typeLine: "Creature — Human Wizard // Creature — Human Insect", manaValue: 1,
			legal: []string{"modern", "pioneer"}, powers: []string{"1", "3"}, toughnesses: []string{"1", "2"}},
		{name: "Solemn Simulacrum", scryfallID: "77777777-7777-7777-7777-777777777777", colors: strPtr(""), cardType: strPtr("Creature"),
			typeLine: "Artifact Creature — Golem", manaValue: 4, storageID: &f.deckboxID,
			legal: []string{"modern", "commander"}, powers: []string{"2"}, toughnesses: []string{"2"}},
		{name: "Eidolon of Blossoms", scryfallID: "88888888-8888-8888-8888-888888888888", colors: strPtr("G"), cardType: strPtr("Creature"),
			typeLine: "Enchantment Creature — Spirit", manaValue: 4, storageID: &f.binderID,
			legal: []string{"modern", "pioneer", "commander"}, powers: []string{"2"}, toughnesses: []string{"2"}},
		{name: "Tarmogoyf", scryfallID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", colors: strPtr("G"), cardType: strPtr("Creature"),
			typeLine: "Creature — Lhurgoyf", manaValue: 2,
			legal: []string{"modern", "commander"}, powers: []string{"*"}, toughnesses: []string{"1+*"}},
		{name: "Half Squirrel", scryfallID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", colors: strPtr("W"), cardType: strPtr("Creature"),
			typeLine: "Creature — Squirrel", manaValue: 1, legal: []string{}, powers: []string{".5"}, toughnesses: []string{"-1"}},
		{name: "Mystery Elf", scryfallID: "cccccccc-cccc-cccc-cccc-cccccccccccc", colors: strPtr("U"), cardType: strPtr("Creature"), manaValue: 7},
		{name: "Unknown", scryfallID: "dddddddd-dddd-dddd-dddd-dddddddddddd", manaValue: 5},
	} {
		_, err := f.repo.Create(ctx, f.userID, Card{
			Name: s.name, ScryfallID: s.scryfallID, SetCode: "tst", CollectorNumber: "1", Foil: s.foil, StorageID: s.storageID,
			ManaValue: s.manaValue, Colors: s.colors, CardType: s.cardType, ColorIdentity: s.colors,
		})
		require.NoError(t, err)
		if s.typeLine == "" {
			continue
		}
		_, err = db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, legal_formats, powers, toughnesses) VALUES ($1, $2, $3, $4, $5)`,
			s.scryfallID, s.typeLine, pq.Array(nonNil(s.legal)), pq.Array(nonNil(s.powers)), pq.Array(nonNil(s.toughnesses)))
		require.NoError(t, err)
	}
	return f
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// names lists the names of the fixture user's cards matching filter, sorted
// bytewise so expectations don't depend on the database collation.
func (f criterionFixture) names(t *testing.T, filter CardFilter) []string {
	t.Helper()
	filter.Page, filter.Limit, filter.SortField = 1, 100, "name"
	result, _, err := f.repo.FindAll(context.Background(), f.userID, filter)
	require.NoError(t, err)
	out := make([]string, len(result))
	for i, c := range result {
		out[i] = c.Name
	}
	sort.Strings(out)
	return out
}

// criterionCase states a criterion and the cards it must return. params is the
// criterion written as advanced query parameters (q= may be one of them), q is
// the same criterion in search syntax ("" when it has no search-syntax form).
type criterionCase struct {
	name   string
	params url.Values
	q      string
	want   []string
}

// runCriterionCases runs each row twice against the fixture: once through the
// advanced parameter translation, once through the search syntax.
func runCriterionCases(t *testing.T, f criterionFixture, cases []criterionCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("advanced", func(t *testing.T) {
				filter, err := parseAdvancedFilters(tc.params)
				require.NoError(t, err)
				assert.Equal(t, tc.want, f.names(t, filter))
			})
			if tc.q == "" {
				return
			}
			t.Run("search", func(t *testing.T) {
				node, err := parseSearchQuery(tc.q)
				require.NoError(t, err)
				assert.Equal(t, tc.want, f.names(t, CardFilter{Query: node}))
			})
		})
	}
}

var criterionCases = []criterionCase{
	{"colors exact", url.Values{"colors": {"g"}}, "c=g",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Llanowar Elves", "Tarmogoyf"}},
	{"colors exact multicolored", url.Values{"colors": {"rw"}, "color_mode": {"exact"}}, "c=rw",
		[]string{"Lightning Helix", "Lightning Helix"}},
	{"colors exact colorless", url.Values{"colors": {""}}, "c:c",
		[]string{"Sol Ring", "Solemn Simulacrum"}},
	{"colors include", url.Values{"colors": {"g"}, "color_mode": {"include"}}, "c>=g",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Grist, the Hunger Tide", "Llanowar Elves", "Tarmogoyf"}},
	{"colors include white", url.Values{"colors": {"W"}, "color_mode": {"include"}}, "c>=w",
		[]string{"Half Squirrel", "Lightning Helix", "Lightning Helix"}},
	{"colors within", url.Values{"colors": {"g"}, "color_mode": {"within"}}, "c<=g",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Llanowar Elves", "Sol Ring", "Solemn Simulacrum", "Tarmogoyf"}},
	{"colors within two colors", url.Values{"colors": {"uw"}, "color_mode": {"within"}}, "c<=wu",
		[]string{"Delver of Secrets", "Half Squirrel", "Mystery Elf", "Sol Ring", "Solemn Simulacrum"}},
	{"colors with a search", url.Values{"colors": {"g"}, "color_mode": {"include"}, "q": {"t:creature"}}, "c>=g t:creature",
		[]string{"Eidolon of Blossoms", "Llanowar Elves", "Tarmogoyf"}},
}

func TestPostgresRepository_FindAll_Criteria(t *testing.T) {
	runCriterionCases(t, seedCriterionFixture(t), criterionCases)
}
