//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f contentFixture) storePrinting(t *testing.T, p printing, gameChanger bool) {
	t.Helper()
	_, err := f.db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, game_changer) VALUES ($1, 'Instant', $2)`, p.scryfallID, gameChanger)
	require.NoError(t, err)
}

func TestFindCardsByDeckID_FlagsTheCardsWhosePrintingIsAGameChanger(t *testing.T) {
	f := newContentFixture(t)
	f.storePrinting(t, bolt, true)
	f.storePrinting(t, island, false)
	f.ownedCopy(t, bolt, copyOptions{board: BoardSideboard})
	f.ownedCopy(t, island, copyOptions{})
	f.ownedCopy(t, tymna, copyOptions{})

	cards, err := f.repo.FindCardsByDeckID(context.Background(), f.userID, contentDeckID, "name", false)

	require.NoError(t, err)
	flags := map[string]bool{}
	for _, c := range cards {
		flags[c.Name] = c.GameChanger
	}
	assert.Equal(t, map[string]bool{bolt.name: true, island.name: false, tymna.name: false}, flags)
}

func TestPendingCards_FlagTheCardsWhosePrintingIsAGameChanger(t *testing.T) {
	f := newContentFixture(t)
	f.storePrinting(t, bolt, true)
	f.storePrinting(t, island, false)
	created, err := f.repo.CreatePendingCard(context.Background(), f.userID, PendingCard{
		DeckID: contentDeckID, Name: bolt.name, ScryfallID: bolt.scryfallID, SetCode: bolt.setCode, CollectorNumber: bolt.collectorNumber,
		Board: BoardConsidering, Quantity: 1, ManaValue: 1,
	})
	require.NoError(t, err)
	f.pendingCard(t, island, 1, copyOptions{})
	f.pendingCard(t, tymna, 1, copyOptions{})

	pending, err := f.repo.FindPendingCards(context.Background(), f.userID, contentDeckID)

	require.NoError(t, err)
	assert.True(t, created.GameChanger)
	flags := map[string]bool{}
	for _, p := range pending {
		flags[p.Name] = p.GameChanger
	}
	assert.Equal(t, map[string]bool{bolt.name: true, island.name: false, tymna.name: false}, flags)
}
