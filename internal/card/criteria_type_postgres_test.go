//go:build integration

package card

import (
	"context"
	"net/url"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// seedTypeCards adds to the criterion fixture a card of each primary type it
// lacks, including a land creature stored as a Land and a double-faced battle.
func seedTypeCards(t *testing.T, f criterionFixture) {
	t.Helper()
	for _, s := range []criterionSeed{
		{name: "Rampant Growth", scryfallID: "e1111111-1111-1111-1111-111111111111", colors: strPtr("G"), cardType: strPtr("Sorcery"),
			typeLine: "Sorcery", manaValue: 2},
		{name: "Forest", scryfallID: "e2222222-2222-2222-2222-222222222222", colors: strPtr(""), cardType: strPtr("Land"),
			typeLine: "Basic Land — Forest"},
		{name: "Dryad Arbor", scryfallID: "e3333333-3333-3333-3333-333333333333", colors: strPtr("G"), cardType: strPtr("Land"),
			typeLine: "Land Creature — Forest Dryad", powers: []string{"1"}, toughnesses: []string{"1"}},
		{name: "Invasion of Zendikar", scryfallID: "e4444444-4444-4444-4444-444444444444", colors: strPtr("G"), cardType: strPtr("Battle"),
			typeLine: "Battle — Siege // Creature — Elemental", manaValue: 4, powers: []string{"", "6"}, toughnesses: []string{"", "6"}},
	} {
		_, err := f.repo.Create(context.Background(), f.userID, Card{
			Name: s.name, ScryfallID: s.scryfallID, SetCode: "tst", CollectorNumber: "1",
			ManaValue: s.manaValue, Colors: s.colors, CardType: s.cardType, ColorIdentity: s.colors,
		})
		require.NoError(t, err)
		_, err = f.repo.db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, legal_formats, powers, toughnesses) VALUES ($1, $2, $3, $4, $5)`,
			s.scryfallID, s.typeLine, pq.Array([]string{}), pq.Array(nonNil(s.powers)), pq.Array(nonNil(s.toughnesses)))
		require.NoError(t, err)
	}
}

// typeCriterionCases pin the advanced type filter to the meaning of t:: a match
// anywhere in the printing's type line, or the stored type when the card has no
// printing yet (Mystery Elf).
var typeCriterionCases = []criterionCase{
	{"type Creature includes artifact, enchantment and land creatures", url.Values{"type": {"Creature"}}, "t:creature",
		[]string{"Delver of Secrets", "Dryad Arbor", "Eidolon of Blossoms", "Half Squirrel", "Invasion of Zendikar", "Llanowar Elves",
			"Mystery Elf", "Solemn Simulacrum", "Tarmogoyf"}},
	{"type Artifact", url.Values{"type": {"Artifact"}}, "t:artifact",
		[]string{"Esika's Chariot", "Sol Ring", "Solemn Simulacrum"}},
	{"type Enchantment", url.Values{"type": {"Enchantment"}}, "t:enchantment",
		[]string{"Eidolon of Blossoms"}},
	{"type Land", url.Values{"type": {"Land"}}, "t:land",
		[]string{"Dryad Arbor", "Forest"}},
	{"type Planeswalker", url.Values{"type": {"Planeswalker"}}, "t:planeswalker",
		[]string{"Grist, the Hunger Tide"}},
	{"type Battle", url.Values{"type": {"Battle"}}, "t:battle",
		[]string{"Invasion of Zendikar"}},
	{"type Instant", url.Values{"type": {"Instant"}}, "t:instant",
		[]string{"Lightning Helix", "Lightning Helix"}},
	{"type Sorcery", url.Values{"type": {"Sorcery"}}, "t:sorcery",
		[]string{"Rampant Growth"}},
	{"type with a search", url.Values{"type": {"Creature"}, "q": {"t:artifact"}}, "t:creature t:artifact",
		[]string{"Solemn Simulacrum"}},
}

func TestPostgresRepository_FindAll_TypeCriteria(t *testing.T) {
	f := seedCriterionFixture(t)
	seedTypeCards(t, f)
	runCriterionCases(t, f, typeCriterionCases)
}
