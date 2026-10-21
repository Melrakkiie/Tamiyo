//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unknownDeckID = "00000000-0000-0000-0000-0000000000ff"

func TestReadDeck_FindsTheDeckOnlyWithinTheReach(t *testing.T) {
	type viewer int
	const (
		owner viewer = iota
		otherUser
		anonymous
	)
	type outcome int
	const (
		notFound outcome = iota
		readAsMine
		readAsSomeoneElses
	)
	cases := []struct {
		reach      Reach
		visibility string
		viewer     viewer
		want       outcome
	}{
		{ReachOwn, VisibilityPrivate, owner, readAsMine},
		{ReachOwn, VisibilityPrivate, otherUser, notFound},
		{ReachOwn, VisibilityPrivate, anonymous, notFound},
		{ReachOwn, VisibilityUnlisted, owner, readAsMine},
		{ReachOwn, VisibilityUnlisted, otherUser, notFound},
		{ReachOwn, VisibilityUnlisted, anonymous, notFound},
		{ReachOwn, VisibilityPublic, owner, readAsMine},
		{ReachOwn, VisibilityPublic, otherUser, notFound},
		{ReachOwn, VisibilityPublic, anonymous, notFound},

		{ReachShared, VisibilityPrivate, owner, notFound},
		{ReachShared, VisibilityPrivate, otherUser, notFound},
		{ReachShared, VisibilityPrivate, anonymous, notFound},
		{ReachShared, VisibilityUnlisted, owner, readAsMine},
		{ReachShared, VisibilityUnlisted, otherUser, readAsSomeoneElses},
		{ReachShared, VisibilityUnlisted, anonymous, readAsSomeoneElses},
		{ReachShared, VisibilityPublic, owner, readAsMine},
		{ReachShared, VisibilityPublic, otherUser, readAsSomeoneElses},
		{ReachShared, VisibilityPublic, anonymous, readAsSomeoneElses},

		{ReachVisible, VisibilityPrivate, owner, readAsMine},
		{ReachVisible, VisibilityPrivate, otherUser, notFound},
		{ReachVisible, VisibilityPrivate, anonymous, notFound},
		{ReachVisible, VisibilityUnlisted, owner, readAsMine},
		{ReachVisible, VisibilityUnlisted, otherUser, readAsSomeoneElses},
		{ReachVisible, VisibilityUnlisted, anonymous, readAsSomeoneElses},
		{ReachVisible, VisibilityPublic, owner, readAsMine},
		{ReachVisible, VisibilityPublic, otherUser, readAsSomeoneElses},
		{ReachVisible, VisibilityPublic, anonymous, readAsSomeoneElses},
	}
	reachNames := map[Reach]string{ReachOwn: "own", ReachShared: "shared", ReachVisible: "visible"}
	viewerNames := map[viewer]string{owner: "owner", otherUser: "other user", anonymous: "anonymous"}

	for _, tc := range cases {
		t.Run(reachNames[tc.reach]+"/"+tc.visibility+"/"+viewerNames[tc.viewer], func(t *testing.T) {
			f := newContentFixture(t)
			bob := seedUser(t, f.db, "bob@example.com")
			f.setVisibility(t, tc.visibility)
			viewerID := map[viewer]string{owner: f.userID, otherUser: bob, anonymous: ""}[tc.viewer]

			read, err := f.service.ReadDeck(context.Background(), viewerID, contentDeckID, tc.reach)

			if tc.want == notFound {
				assert.ErrorIs(t, err, ErrNotFound)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, f.userID, read.OwnerID)
			assert.Equal(t, tc.want == readAsMine, read.Mine)
			assert.Equal(t, contentDeckID, read.Deck.ID)
		})
	}
}

func TestReadDeck_EntriesHoldPendingCardsAndTheCommanderSlotsForAVisitor(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 2, copyOptions{})
	partner := f.ownedCopy(t, tymna, copyOptions{})
	f.setCommanders(t, nil, &commander, &partner, nil)
	f.ownedCopy(t, bolt, copyOptions{})
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, island, 1, copyOptions{board: BoardSideboard})
	bob := f.sharedWithBob(t)

	read, err := f.service.ReadDeck(context.Background(), bob, contentDeckID, ReachShared)

	require.NoError(t, err)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, pending: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, owned: 1, pending: 2, quantity: 3},
		{name: island.name, board: BoardSideboard, pending: 1, quantity: 1},
	}, summarize(read.Entries))
}

func TestReadDeck_TagsAttachByCardNameAcrossPrintingsAndPendingCopies(t *testing.T) {
	f := newContentFixture(t)
	boltA25 := printing{bolt.name, "e3285e6b-3e79-4d7c-bf96-d920f973b122", "a25", "141"}
	f.ownedCopy(t, bolt, copyOptions{})
	f.pendingCard(t, boltA25, 2, copyOptions{board: BoardSideboard})
	f.pendingCard(t, atraxa, 1, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	ctx := context.Background()
	_, err := f.service.SetCardTags(ctx, f.userID, contentDeckID, "lightning  BOLT", []string{"cheap", "Burn"})
	require.NoError(t, err)
	_, err = f.service.SetCardTags(ctx, f.userID, contentDeckID, atraxa.name, []string{"Ramp", "card draw"})
	require.NoError(t, err)
	_, err = f.service.SetCardTags(ctx, f.userID, contentDeckID, island.name, []string{"Land"})
	require.NoError(t, err)
	_, err = f.db.Exec(`DELETE FROM tamiyo.card_deck WHERE deck_id = $1 AND card_id IN (SELECT id FROM tamiyo.cards WHERE name = $2)`, contentDeckID, island.name)
	require.NoError(t, err)
	bob := f.sharedWithBob(t)

	read, err := f.service.ReadDeck(ctx, bob, contentDeckID, ReachVisible)

	require.NoError(t, err)
	assert.Equal(t, map[string][]string{
		"lightning bolt":          {"Burn", "cheap"},
		"atraxa, praetors' voice": {"card draw", "Ramp"},
	}, read.Tags, "a card no longer in the deck keeps no tags")
}

func TestReadDeck_AnUnknownDeckIsNotFoundWhateverTheReachAndViewer(t *testing.T) {
	f := newContentFixture(t)
	bob := seedUser(t, f.db, "bob@example.com")

	for _, reach := range []Reach{ReachOwn, ReachShared, ReachVisible} {
		for _, viewerID := range []string{f.userID, bob, ""} {
			_, err := f.service.ReadDeck(context.Background(), viewerID, unknownDeckID, reach)

			assert.ErrorIs(t, err, ErrNotFound, "reach %d, viewer %q", reach, viewerID)
		}
	}
}
