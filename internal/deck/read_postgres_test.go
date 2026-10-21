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
