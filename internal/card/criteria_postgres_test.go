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
		{name: "Rampant Growth", scryfallID: "e1111111-1111-1111-1111-111111111111", colors: strPtr("G"), cardType: strPtr("Sorcery"),
			typeLine: "Sorcery", manaValue: 2},
		{name: "Forest", scryfallID: "e2222222-2222-2222-2222-222222222222", colors: strPtr(""), cardType: strPtr("Land"),
			typeLine: "Basic Land — Forest"},
		{name: "Dryad Arbor", scryfallID: "e3333333-3333-3333-3333-333333333333", colors: strPtr("G"), cardType: strPtr("Land"),
			typeLine: "Land Creature — Forest Dryad", powers: []string{"1"}, toughnesses: []string{"1"}},
		{name: "Invasion of Zendikar", scryfallID: "e4444444-4444-4444-4444-444444444444", colors: strPtr("G"), cardType: strPtr("Battle"),
			typeLine: "Battle — Siege // Creature — Elemental", manaValue: 4, powers: []string{"", "6"}, toughnesses: []string{"", "6"}},
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
	out := cardNames(result)
	sort.Strings(out)
	return out
}

// criterionCase states a criterion and the cards it must return. params is the
// criterion written as advanced query parameters (q= may be one of them), q is
// the same criterion in search syntax ("" when it has no search-syntax form).
// scope, when set, adds the list scope the criterion runs inside (a storage,
// "open to this deck", stacking) to both forms.
type criterionCase struct {
	name   string
	params url.Values
	q      string
	want   []string
	scope  func(f criterionFixture) CardFilter
}

func (tc criterionCase) withScope(f criterionFixture, filter CardFilter) CardFilter {
	if tc.scope == nil {
		return filter
	}
	scope := tc.scope(f)
	filter.StorageID, filter.OpenToDeck, filter.Stack = scope.StorageID, scope.OpenToDeck, scope.Stack
	return filter
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
				assert.Equal(t, tc.want, f.names(t, tc.withScope(f, filter)))
			})
			if tc.q == "" {
				return
			}
			t.Run("search", func(t *testing.T) {
				node, err := parseSearchQuery(tc.q)
				require.NoError(t, err)
				assert.Equal(t, tc.want, f.names(t, tc.withScope(f, CardFilter{Query: node})))
			})
		})
	}
}

func inBinder(f criterionFixture) CardFilter  { return CardFilter{StorageID: &f.binderID} }
func inDeckbox(f criterionFixture) CardFilter { return CardFilter{StorageID: &f.deckboxID} }
func stacked(criterionFixture) CardFilter     { return CardFilter{Stack: true} }
func openToAllowedDeck(f criterionFixture) CardFilter {
	return CardFilter{OpenToDeck: &f.allowedDeck}
}
func openToOtherDeck(f criterionFixture) CardFilter {
	return CardFilter{OpenToDeck: &f.otherDeck}
}

// criterionCases is the one table of criteria and the cards they must return.
// Rows without a search-syntax form (subtype, storage type) have q == "".
var criterionCases = []criterionCase{
	{"colors exact", url.Values{"colors": {"g"}}, "c=g",
		[]string{"Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Invasion of Zendikar", "Llanowar Elves", "Rampant Growth", "Tarmogoyf"}, nil},
	{"colors exact multicolored", url.Values{"colors": {"rw"}, "color_mode": {"exact"}}, "c=rw",
		[]string{"Lightning Helix", "Lightning Helix"}, nil},
	{"colors exact colorless", url.Values{"colors": {""}}, "c:c",
		[]string{"Forest", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"colors include", url.Values{"colors": {"g"}, "color_mode": {"include"}}, "c>=g",
		[]string{"Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Grist, the Hunger Tide", "Invasion of Zendikar", "Llanowar Elves", "Rampant Growth", "Tarmogoyf"}, nil},
	{"colors include white", url.Values{"colors": {"W"}, "color_mode": {"include"}}, "c>=w",
		[]string{"Half Squirrel", "Lightning Helix", "Lightning Helix"}, nil},
	{"colors within", url.Values{"colors": {"g"}, "color_mode": {"within"}}, "c<=g",
		[]string{"Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Forest", "Invasion of Zendikar", "Llanowar Elves", "Rampant Growth", "Sol Ring", "Solemn Simulacrum", "Tarmogoyf"}, nil},
	{"colors within two colors", url.Values{"colors": {"uw"}, "color_mode": {"within"}}, "c<=wu",
		[]string{"Delver of Secrets", "Forest", "Half Squirrel", "Mystery Elf", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"colors with a search", url.Values{"colors": {"g"}, "color_mode": {"include"}, "q": {"t:creature"}}, "c>=g t:creature",
		[]string{"Dryad Arbor", "Eidolon of Blossoms", "Invasion of Zendikar", "Llanowar Elves", "Tarmogoyf"}, nil},
	{"color count colorless", url.Values{"color_count": {"0"}}, "c:0",
		[]string{"Forest", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"color count one", url.Values{"color_count": {"1"}}, "c:1",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Half Squirrel", "Invasion of Zendikar", "Llanowar Elves", "Mystery Elf", "Rampant Growth", "Tarmogoyf"}, nil},
	{"color count two", url.Values{"color_count": {"2"}}, "c:2",
		[]string{"Grist, the Hunger Tide", "Lightning Helix", "Lightning Helix"}, nil},
	{"foil", url.Values{"foil": {"true"}}, "is:foil",
		[]string{"Esika's Chariot", "Lightning Helix"}, nil},
	{"nonfoil", url.Values{"foil": {"false"}}, "is:nonfoil",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Forest", "Grist, the Hunger Tide", "Half Squirrel", "Invasion of Zendikar", "Lightning Helix", "Llanowar Elves", "Mystery Elf", "Rampant Growth", "Sol Ring", "Solemn Simulacrum", "Tarmogoyf", "Unknown"}, nil},
	{"legal format", url.Values{"legal_in": {"pioneer"}}, "f:pioneer",
		[]string{"Delver of Secrets", "Eidolon of Blossoms", "Esika's Chariot"}, nil},
	{"legal format commander", url.Values{"legal_in": {"commander"}}, "f:commander",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Lightning Helix", "Lightning Helix", "Llanowar Elves",
			"Sol Ring", "Solemn Simulacrum", "Tarmogoyf"}, nil},
	{"colors, color count, foil and format with a search", url.Values{
		"colors": {"w"}, "color_mode": {"include"}, "color_count": {"2"}, "foil": {"true"}, "legal_in": {"modern"}, "q": {"t:instant"},
	}, "c>=w c:2 is:foil f:modern t:instant",
		[]string{"Lightning Helix"}, nil},
	{"format and nonfoil with a search", url.Values{"legal_in": {"modern"}, "foil": {"false"}, "q": {"t:creature or t:planeswalker"}},
		"f:modern is:nonfoil (t:creature or t:planeswalker)",
		[]string{"Delver of Secrets", "Eidolon of Blossoms", "Grist, the Hunger Tide", "Llanowar Elves", "Solemn Simulacrum", "Tarmogoyf"}, nil},
	{"mana value eq", url.Values{"mana_value": {"2"}, "mana_value_op": {"eq"}}, "mv=2",
		[]string{"Lightning Helix", "Lightning Helix", "Rampant Growth", "Tarmogoyf"}, nil},
	{"mana value lt", url.Values{"mana_value": {"3"}, "mana_value_op": {"lt"}}, "mv<3",
		[]string{"Delver of Secrets", "Dryad Arbor", "Forest", "Half Squirrel", "Lightning Helix", "Lightning Helix", "Llanowar Elves", "Rampant Growth", "Sol Ring", "Tarmogoyf"}, nil},
	{"mana value lte", url.Values{"mana_value": {"3"}, "mana_value_op": {"lte"}}, "mv<=3",
		[]string{"Delver of Secrets", "Dryad Arbor", "Forest", "Grist, the Hunger Tide", "Half Squirrel", "Lightning Helix", "Lightning Helix", "Llanowar Elves", "Rampant Growth", "Sol Ring", "Tarmogoyf"}, nil},
	{"mana value gt", url.Values{"mana_value": {"4"}, "mana_value_op": {"gt"}}, "mv>4",
		[]string{"Mystery Elf", "Unknown"}, nil},
	{"mana value gte", url.Values{"mana_value": {"4"}, "mana_value_op": {"gte"}}, "mv>=4",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Invasion of Zendikar", "Mystery Elf", "Solemn Simulacrum", "Unknown"}, nil},
	{"mana value defaults to eq", url.Values{"mana_value": {"1"}}, "mv:1",
		[]string{"Delver of Secrets", "Half Squirrel", "Llanowar Elves", "Sol Ring"}, nil},
	{"power eq", url.Values{"power": {"1"}, "power_op": {"eq"}}, "pow=1",
		[]string{"Delver of Secrets", "Dryad Arbor", "Llanowar Elves"}, nil},
	{"power on the back face", url.Values{"power": {"3"}}, "pow:3",
		[]string{"Delver of Secrets"}, nil},
	{"power lt", url.Values{"power": {"1"}, "power_op": {"lt"}}, "pow<1",
		[]string{"Half Squirrel"}, nil},
	{"power lte", url.Values{"power": {"2"}, "power_op": {"lte"}}, "pow<=2",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Half Squirrel", "Llanowar Elves", "Solemn Simulacrum"}, nil},
	{"power gt", url.Values{"power": {"2"}, "power_op": {"gt"}}, "pow>2",
		[]string{"Delver of Secrets", "Esika's Chariot", "Invasion of Zendikar"}, nil},
	{"power gte", url.Values{"power": {"3"}, "power_op": {"gte"}}, "pow>=3",
		[]string{"Delver of Secrets", "Esika's Chariot", "Invasion of Zendikar"}, nil},
	{"power never matches a star or a missing printing", url.Values{"power": {"0"}}, "pow=0",
		[]string{}, nil},
	{"toughness eq on the back face", url.Values{"toughness": {"2"}, "toughness_op": {"eq"}}, "tou=2",
		[]string{"Delver of Secrets", "Eidolon of Blossoms", "Solemn Simulacrum"}, nil},
	{"toughness lt", url.Values{"toughness": {"0"}, "toughness_op": {"lt"}}, "tou<0",
		[]string{"Half Squirrel"}, nil},
	{"toughness lte", url.Values{"toughness": {"1"}, "toughness_op": {"lte"}}, "tou<=1",
		[]string{"Delver of Secrets", "Dryad Arbor", "Half Squirrel", "Llanowar Elves"}, nil},
	{"toughness gt", url.Values{"toughness": {"1"}, "toughness_op": {"gt"}}, "tou>1",
		[]string{"Delver of Secrets", "Eidolon of Blossoms", "Esika's Chariot", "Invasion of Zendikar", "Solemn Simulacrum"}, nil},
	{"toughness gte", url.Values{"toughness": {"4"}, "toughness_op": {"gte"}}, "tou>=4",
		[]string{"Esika's Chariot", "Invasion of Zendikar"}, nil},
	{"power and toughness of different faces", url.Values{"power": {"3"}, "toughness": {"1"}}, "pow=3 tou=1",
		[]string{"Delver of Secrets"}, nil},
	{"mana value with a search", url.Values{"mana_value": {"4"}, "mana_value_op": {"gte"}, "q": {"t:creature"}}, "mv>=4 t:creature",
		[]string{"Eidolon of Blossoms", "Invasion of Zendikar", "Mystery Elf", "Solemn Simulacrum"}, nil},
	{"subtype", url.Values{"subtype": {"elf"}}, "",
		[]string{"Llanowar Elves"}, nil},
	{"subtype ignores case and surrounding spaces", url.Values{"subtype": {" GOLEM "}}, "",
		[]string{"Solemn Simulacrum"}, nil},
	{"subtype of the front face", url.Values{"subtype": {"human"}}, "",
		[]string{"Delver of Secrets"}, nil},
	{"subtype stops at the second dash", url.Values{"subtype": {"insect"}}, "",
		[]string{}, nil},
	{"subtype keeps pattern wildcards", url.Values{"subtype": {"e%f"}}, "",
		[]string{"Llanowar Elves"}, nil},
	{"subtype with a search", url.Values{"subtype": {"vehicle"}, "q": {"t:artifact"}}, "",
		[]string{"Esika's Chariot"}, nil},
	{"subtype with mana value", url.Values{"subtype": {"o"}, "mana_value": {"4"}, "mana_value_op": {"gte"}}, "",
		[]string{"Solemn Simulacrum"}, nil},
	{"storage type", url.Values{"storage_type": {"deckbox"}}, "",
		[]string{"Lightning Helix", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"storage type ignores case and surrounding spaces", url.Values{"storage_type": {" DeckBox "}}, "",
		[]string{"Lightning Helix", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"storage type binder", url.Values{"storage_type": {"binder"}}, "",
		[]string{"Eidolon of Blossoms", "Grist, the Hunger Tide", "Lightning Helix", "Llanowar Elves"}, nil},
	{"storage type is an equality", url.Values{"storage_type": {"box"}}, "",
		[]string{}, nil},
	{"storage type with format", url.Values{"storage_type": {"deckbox"}, "legal_in": {"modern"}}, "",
		[]string{"Lightning Helix", "Solemn Simulacrum"}, nil},
	{"storage type with foil and a search", url.Values{"storage_type": {"deckbox"}, "foil": {"true"}, "q": {"t:instant"}}, "",
		[]string{"Lightning Helix"}, nil},
	{"type Creature includes artifact, enchantment and land creatures", url.Values{"type": {"Creature"}}, "t:creature",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Half Squirrel", "Invasion of Zendikar", "Llanowar Elves",
			"Mystery Elf", "Solemn Simulacrum", "Tarmogoyf"}, nil},
	{"type Artifact", url.Values{"type": {"Artifact"}}, "t:artifact",
		[]string{"Esika's Chariot", "Sol Ring", "Solemn Simulacrum"}, nil},
	{"type Enchantment", url.Values{"type": {"Enchantment"}}, "t:enchantment",
		[]string{"Eidolon of Blossoms"}, nil},
	{"type Land", url.Values{"type": {"Land"}}, "t:land",
		[]string{"Dryad Arbor", "Forest"}, nil},
	{"type Planeswalker", url.Values{"type": {"Planeswalker"}}, "t:planeswalker",
		[]string{"Grist, the Hunger Tide"}, nil},
	{"type Battle", url.Values{"type": {"Battle"}}, "t:battle",
		[]string{"Invasion of Zendikar"}, nil},
	{"type Instant", url.Values{"type": {"Instant"}}, "t:instant",
		[]string{"Lightning Helix", "Lightning Helix"}, nil},
	{"type Sorcery", url.Values{"type": {"Sorcery"}}, "t:sorcery",
		[]string{"Rampant Growth"}, nil},
	{"type with a search", url.Values{"type": {"Creature"}, "q": {"t:artifact"}}, "t:creature t:artifact",
		[]string{"Solemn Simulacrum"}, nil},

	// List scope: the same criteria apply inside a storage, and "open to this
	// deck" hides copies in storages locked against the deck.
	{"storage scope alone", url.Values{}, "",
		[]string{"Eidolon of Blossoms", "Grist, the Hunger Tide", "Lightning Helix", "Llanowar Elves"}, inBinder},
	{"storage scope with colors", url.Values{"colors": {"g"}, "color_mode": {"include"}}, "c>=g",
		[]string{"Eidolon of Blossoms", "Grist, the Hunger Tide", "Llanowar Elves"}, inBinder},
	{"storage scope with a search", url.Values{"q": {"t:creature"}}, "t:creature",
		[]string{"Eidolon of Blossoms", "Llanowar Elves"}, inBinder},
	{"storage scope with foil", url.Values{"foil": {"true"}}, "is:foil",
		[]string{"Lightning Helix"}, inDeckbox},
	{"storage scope with nonfoil", url.Values{"foil": {"false"}}, "is:nonfoil",
		[]string{"Sol Ring", "Solemn Simulacrum"}, inDeckbox},
	{"storage scope with a type the storage lacks", url.Values{"type": {"Land"}}, "t:land",
		[]string{}, inBinder},
	{"storage scope with a subtype", url.Values{"subtype": {"elf"}}, "",
		[]string{"Llanowar Elves"}, inBinder},
	{"open to the allowed deck sees the locked storage", url.Values{"colors": {""}}, "c:c",
		[]string{"Forest", "Sol Ring", "Solemn Simulacrum"}, openToAllowedDeck},
	{"open to another deck hides the locked storage", url.Values{"colors": {""}}, "c:c",
		[]string{"Forest"}, openToOtherDeck},
	{"open to another deck without other filters", url.Values{}, "",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Forest", "Grist, the Hunger Tide",
			"Half Squirrel", "Invasion of Zendikar", "Lightning Helix", "Llanowar Elves", "Mystery Elf", "Rampant Growth", "Tarmogoyf", "Unknown"}, openToOtherDeck},
	{"open to the allowed deck without other filters", url.Values{}, "",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Forest", "Grist, the Hunger Tide",
			"Half Squirrel", "Invasion of Zendikar", "Lightning Helix", "Lightning Helix", "Llanowar Elves", "Mystery Elf", "Rampant Growth",
			"Sol Ring", "Solemn Simulacrum", "Tarmogoyf", "Unknown"}, openToAllowedDeck},
	{"open to another deck with a format", url.Values{"legal_in": {"commander"}}, "f:commander",
		[]string{"Eidolon of Blossoms", "Esika's Chariot", "Lightning Helix", "Llanowar Elves", "Tarmogoyf"}, openToOtherDeck},
	{"open to another deck, stacked, with colors", url.Values{"colors": {"g"}, "color_mode": {"include"}}, "c>=g",
		[]string{"Dryad Arbor", "Eidolon of Blossoms", "Esika's Chariot", "Grist, the Hunger Tide", "Invasion of Zendikar", "Llanowar Elves", "Rampant Growth", "Tarmogoyf"},
		func(f criterionFixture) CardFilter { return CardFilter{OpenToDeck: &f.otherDeck, Stack: true} }},
	{"open to another deck in a storage", url.Values{}, "",
		[]string{}, func(f criterionFixture) CardFilter {
			return CardFilter{OpenToDeck: &f.otherDeck, StorageID: &f.deckboxID}
		}},
	{"open to the allowed deck in a storage", url.Values{"foil": {"true"}}, "is:foil",
		[]string{"Lightning Helix"},
		func(f criterionFixture) CardFilter {
			return CardFilter{OpenToDeck: &f.allowedDeck, StorageID: &f.deckboxID}
		}},

	// Stacking merges copies of the same card after the criteria filtered them.
	{"stacked copies in different storages stay apart", url.Values{"colors": {"rw"}}, "c=rw",
		[]string{"Lightning Helix", "Lightning Helix"}, stacked},
	{"stacked with a format and a search", url.Values{"legal_in": {"modern"}, "q": {"t:instant"}}, "f:modern t:instant",
		[]string{"Lightning Helix", "Lightning Helix"}, stacked},
	{"stacked with a criterion that selects one copy", url.Values{"foil": {"true"}, "colors": {"rw"}}, "is:foil c=rw",
		[]string{"Lightning Helix"}, stacked},
}

func TestPostgresRepository_FindAll_Criteria(t *testing.T) {
	runCriterionCases(t, seedCriterionFixture(t), criterionCases)
}

func TestPostgresRepository_FindAll_StorageTypeStaysWithinTheUser(t *testing.T) {
	f := seedCriterionFixture(t)
	bob := criterionFixture{repo: f.repo, userID: f.otherUserID}

	deckbox, err := parseAdvancedFilters(url.Values{"storage_type": {"deckbox"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"Bob's Bolt"}, bob.names(t, deckbox))

	binder, err := parseAdvancedFilters(url.Values{"storage_type": {"binder"}})
	require.NoError(t, err)
	assert.Equal(t, []string{}, bob.names(t, binder))
	binder.Stack = true
	assert.Equal(t, []string{}, bob.names(t, binder))
}

// A criterion narrows the cards before the list shape is applied: the same
// filtered cards stack, group, sort and page as an unfiltered list would.
func TestPostgresRepository_FindAll_ListShapeWithCriteria(t *testing.T) {
	f := seedCriterionFixture(t)
	// A second, identical copy of Llanowar Elves so that stacking has something to merge.
	_, err := f.repo.Create(context.Background(), f.userID, Card{
		Name: "Llanowar Elves", ScryfallID: "11111111-1111-1111-1111-111111111111", SetCode: "tst", CollectorNumber: "1",
		StorageID: &f.binderID, ManaValue: 1, Colors: strPtr("G"), CardType: strPtr("Creature"), ColorIdentity: strPtr("G"),
	})
	require.NoError(t, err)

	greens, err := parseAdvancedFilters(url.Values{"colors": {"g"}, "color_mode": {"include"}})
	require.NoError(t, err)

	list := func(t *testing.T, shape CardFilter) ([]Card, int) {
		t.Helper()
		filter := greens
		filter.Stack, filter.GroupBy, filter.SortField, filter.SortDesc = shape.Stack, shape.GroupBy, shape.SortField, shape.SortDesc
		filter.Page, filter.Limit = shape.Page, shape.Limit
		if filter.Limit == 0 {
			filter.Page, filter.Limit = 1, 25
		}
		cards, total, err := f.repo.FindAll(context.Background(), f.userID, filter)
		require.NoError(t, err)
		return cards, total
	}

	t.Run("sorted by name, descending, one page", func(t *testing.T) {
		cards, total := list(t, CardFilter{SortField: "name", SortDesc: true, Page: 2, Limit: 4})
		assert.Equal(t, 9, total)
		assert.Equal(t, []string{"Invasion of Zendikar", "Grist, the Hunger Tide", "Esika's Chariot", "Eidolon of Blossoms"}, cardNames(cards))
	})
	t.Run("sorted by mana value", func(t *testing.T) {
		cards, total := list(t, CardFilter{SortField: "mana_value"})
		assert.Equal(t, 9, total)
		assert.Equal(t, []string{"Dryad Arbor", "Llanowar Elves", "Llanowar Elves", "Tarmogoyf", "Rampant Growth",
			"Grist, the Hunger Tide", "Esika's Chariot", "Eidolon of Blossoms", "Invasion of Zendikar"}, cardNames(cards))
	})
	t.Run("stacked", func(t *testing.T) {
		cards, total := list(t, CardFilter{Stack: true, SortField: "name"})
		assert.Equal(t, 8, total)
		require.Len(t, cards, 8)
		for _, c := range cards {
			want := 1
			if c.Name == "Llanowar Elves" {
				want = 2
			}
			assert.Len(t, c.CopyIDs, want, c.Name)
		}
	})
	t.Run("stacked and paged", func(t *testing.T) {
		cards, total := list(t, CardFilter{Stack: true, SortField: "name", Page: 2, Limit: 3})
		assert.Equal(t, 8, total)
		assert.Equal(t, []string{"Grist, the Hunger Tide", "Invasion of Zendikar", "Llanowar Elves"}, cardNames(cards))
	})
	t.Run("grouped by subtype", func(t *testing.T) {
		cards, total := list(t, CardFilter{Stack: true, GroupBy: "subtype", SortField: "name"})
		rows := make([]string, len(cards))
		for i, c := range cards {
			subtype := "-"
			if c.Subtype != nil {
				subtype = *c.Subtype
			}
			rows[i] = subtype + ": " + c.Name
		}
		assert.Equal(t, len(rows), total)
		assert.Equal(t, []string{
			"Druid: Llanowar Elves", "Dryad: Dryad Arbor", "Elemental: Invasion of Zendikar", "Elf: Llanowar Elves",
			"Forest: Dryad Arbor", "Grist: Grist, the Hunger Tide", "Lhurgoyf: Tarmogoyf", "Siege: Invasion of Zendikar",
			"Spirit: Eidolon of Blossoms", "Vehicle: Esika's Chariot", "-: Rampant Growth",
		}, rows)
	})
}
