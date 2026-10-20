//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otherDeckID = "00000000-0000-0000-0000-000000000002"

type collectionCopy struct {
	printing  printing
	foil      bool
	proxy     bool
	storageID *int
}

func (f contentFixture) collectionCopy(t *testing.T, c collectionCopy) int {
	t.Helper()
	var id int
	err := f.db.Get(&id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1)
		RETURNING id
	`, f.userID, c.printing.name, c.printing.scryfallID, c.printing.setCode, c.printing.collectorNumber, c.foil, c.proxy, c.storageID)
	require.NoError(t, err)
	return id
}

func (f contentFixture) putInDeck(t *testing.T, cardID int, deckID string) {
	t.Helper()
	_, err := f.db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id, board) VALUES ($1, $2, 'main')`, cardID, deckID)
	require.NoError(t, err)
}

func (f contentFixture) storage(t *testing.T, name string, locked bool, openTo ...string) *int {
	t.Helper()
	var id int
	require.NoError(t, f.db.Get(&id, `INSERT INTO tamiyo.storage (user_id, name, type, locked) VALUES ($1, $2, 'box', $3) RETURNING id`, f.userID, name, locked))
	for _, deckID := range openTo {
		_, err := f.db.Exec(`INSERT INTO tamiyo.storage_allowed_deck (storage_id, deck_id) VALUES ($1, $2)`, id, deckID)
		require.NoError(t, err)
	}
	return &id
}

func (f contentFixture) ownedCopies(t *testing.T, wanted CopiesWanted) []OwnedCopy {
	t.Helper()
	copies, err := findOwnedCopies(context.Background(), f.repo, f.userID, contentDeckID, wanted)
	require.NoError(t, err)
	return copies
}

func TestFindOwnedCopies_ReturnsFreeCopiesOfTheWantedPrintings(t *testing.T) {
	f := newContentFixture(t)
	nonFoil := f.collectionCopy(t, collectionCopy{printing: bolt})
	foil := f.collectionCopy(t, collectionCopy{printing: bolt, foil: true, proxy: true})
	f.collectionCopy(t, collectionCopy{printing: island})

	copies := f.ownedCopies(t, CopiesWanted{ScryfallIDs: []string{bolt.scryfallID}})

	assert.Equal(t, []OwnedCopy{
		{ID: nonFoil, Name: bolt.name, ScryfallID: bolt.scryfallID},
		{ID: foil, Name: bolt.name, ScryfallID: bolt.scryfallID, Foil: true, Proxy: true},
	}, copies)
}

var (
	fireIce   = printing{"Fire // Ice", "f3c2b1a0-1111-4c3d-9f2a-7e5b6c8d9a10", "mh2", "290"}
	fireIceUK = printing{"Fire // Ice", "f3c2b1a0-2222-4c3d-9f2a-7e5b6c8d9a10", "dmr", "211"}
)

func TestFindOwnedCopies_MatchesCardNamesOnFullOrFrontFaceName(t *testing.T) {
	f := newContentFixture(t)
	first := f.collectionCopy(t, collectionCopy{printing: fireIce})
	second := f.collectionCopy(t, collectionCopy{printing: fireIceUK})
	bolts := f.collectionCopy(t, collectionCopy{printing: bolt})

	byFullName := f.ownedCopies(t, CopiesWanted{Names: []string{"fire  /  ice"}})
	byFrontFace := f.ownedCopies(t, CopiesWanted{Names: []string{"FIRE"}})
	byBoth := f.ownedCopies(t, CopiesWanted{ScryfallIDs: []string{fireIce.scryfallID}, Names: []string{bolt.name}})

	assert.Equal(t, []int{first, second}, copyIDs(byFullName))
	assert.Equal(t, []int{first, second}, copyIDs(byFrontFace))
	assert.Equal(t, []int{first, bolts}, copyIDs(byBoth))
}

func TestFindOwnedCopies_FlagsCopiesInAnotherDeckAndInTheTargetDeck(t *testing.T) {
	f := newContentFixture(t)
	free := f.collectionCopy(t, collectionCopy{printing: bolt})
	elsewhere := f.collectionCopy(t, collectionCopy{printing: bolt})
	f.putInDeck(t, elsewhere, otherDeckID)
	inTarget := f.collectionCopy(t, collectionCopy{printing: bolt})
	f.putInDeck(t, inTarget, contentDeckID)

	copies := f.ownedCopies(t, CopiesWanted{ScryfallIDs: []string{bolt.scryfallID}})

	assert.Equal(t, []OwnedCopy{
		{ID: free, Name: bolt.name, ScryfallID: bolt.scryfallID},
		{ID: elsewhere, Name: bolt.name, ScryfallID: bolt.scryfallID, InDeck: true},
		{ID: inTarget, Name: bolt.name, ScryfallID: bolt.scryfallID, InDeck: true, InTargetDeck: true},
	}, copies)
}

func TestFindOwnedCopies_FlagsCopiesInStoragesLockedAgainstTheTargetDeck(t *testing.T) {
	f := newContentFixture(t)
	unlocked := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Binder", false)})
	locked := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Trades", true)})
	openToOther := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Reserved", true, otherDeckID)})
	openToTarget := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Cube", true, contentDeckID)})

	copies := f.ownedCopies(t, CopiesWanted{ScryfallIDs: []string{bolt.scryfallID}})

	locks := make(map[int]bool, len(copies))
	for _, c := range copies {
		locks[c.ID] = c.LockedAgainstTarget
	}
	assert.Equal(t, map[int]bool{unlocked: false, locked: true, openToOther: true, openToTarget: false}, locks)
}

func TestFindOwnedCopies_IgnoresAnotherUsersCopies(t *testing.T) {
	f := newContentFixture(t)
	mine := f.collectionCopy(t, collectionCopy{printing: bolt})
	other := contentFixture{db: f.db, repo: f.repo, service: f.service, userID: seedUser(t, f.db, "bob@example.com")}
	other.collectionCopy(t, collectionCopy{printing: bolt})
	other.collectionCopy(t, collectionCopy{printing: fireIce})

	copies := f.ownedCopies(t, CopiesWanted{ScryfallIDs: []string{bolt.scryfallID}, Names: []string{"Fire"}})

	assert.Equal(t, []int{mine}, copyIDs(copies))
}

func TestFindOwnedCopies_ReturnsNothingWhenNothingIsWanted(t *testing.T) {
	f := newContentFixture(t)
	f.collectionCopy(t, collectionCopy{printing: bolt})

	assert.Empty(t, f.ownedCopies(t, CopiesWanted{}))
}

func copyIDs(copies []OwnedCopy) []int {
	ids := make([]int, 0, len(copies))
	for _, c := range copies {
		ids = append(ids, c.ID)
	}
	return ids
}
