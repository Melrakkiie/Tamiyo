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
