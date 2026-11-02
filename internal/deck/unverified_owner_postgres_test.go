//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setVerified(t *testing.T, userID string, verified bool) {
	t.Helper()
	query := `UPDATE tamiyo.users SET email_verified_at = NULL WHERE id = $1`
	if verified {
		query = `UPDATE tamiyo.users SET email_verified_at = now() WHERE id = $1`
	}
	_, err := testDB.Exec(query, userID)
	require.NoError(t, err)
}

func TestUnverifiedOwner_DecksAreVisibleToTheirOwnerOnly(t *testing.T) {
	ctx := context.Background()
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	folder, err := repo.CreateFolder(ctx, alice, Folder{Name: "Tribal"})
	require.NoError(t, err)
	public, err := repo.Create(ctx, alice, Deck{Name: "Public", Format: "commander", Visibility: VisibilityPublic, FolderID: &folder.ID})
	require.NoError(t, err)
	unlisted, err := repo.Create(ctx, alice, Deck{Name: "Unlisted", Format: "commander", Visibility: VisibilityUnlisted})
	require.NoError(t, err)
	require.NoError(t, repo.Like(ctx, bob, public.ID))

	browse := func() []PublicDeck {
		decks, _, err := repo.FindPublic(ctx, PublicFilter{Page: 1, Limit: 10})
		require.NoError(t, err)
		return decks
	}
	profile := func() []Deck {
		decks, _, err := repo.FindAll(ctx, alice, Filter{Page: 1, Limit: 10, Visibility: VisibilityPublic})
		require.NoError(t, err)
		return decks
	}
	liked := func() []PublicDeck {
		decks, _, err := repo.FindLiked(ctx, bob, 1, 10)
		require.NoError(t, err)
		return decks
	}
	folders := func() []Folder {
		f, err := repo.FindPublicFolders(ctx, alice, bob)
		require.NoError(t, err)
		return f
	}

	setVerified(t, alice, false)

	for _, id := range []string{public.ID, unlisted.ID} {
		_, _, err := repo.FindShared(ctx, id)
		assert.ErrorIs(t, err, ErrNotFound)
	}
	assert.Empty(t, browse())
	assert.Empty(t, profile())
	assert.Empty(t, liked())
	assert.Empty(t, folders())

	own, _, err := repo.FindAll(ctx, alice, Filter{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, own, 2)
	_, err = repo.FindByID(ctx, alice, public.ID)
	assert.NoError(t, err)

	setVerified(t, alice, true)

	for _, id := range []string{public.ID, unlisted.ID} {
		_, _, err := repo.FindShared(ctx, id)
		assert.NoError(t, err)
	}
	assert.Len(t, browse(), 1)
	assert.Len(t, profile(), 1)
	assert.Len(t, liked(), 1)
	assert.Len(t, folders(), 1)
}

func TestUnverifiedOwner_ReadsTheirOwnProfileAndSharedPath(t *testing.T) {
	ctx := context.Background()
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	folder, err := repo.CreateFolder(ctx, alice, Folder{Name: "Tribal"})
	require.NoError(t, err)
	public, err := repo.Create(ctx, alice, Deck{Name: "Public", Format: "commander", Visibility: VisibilityPublic, FolderID: &folder.ID})
	require.NoError(t, err)
	private, err := repo.Create(ctx, alice, Deck{Name: "Private", Format: "commander", Visibility: VisibilityPrivate})
	require.NoError(t, err)
	setVerified(t, alice, false)

	profile := func(viewer string) []Deck {
		decks, _, err := repo.FindAll(ctx, alice, Filter{Page: 1, Limit: 10, Visibility: VisibilityPublic, ViewerID: viewer})
		require.NoError(t, err)
		return decks
	}
	assert.Len(t, profile(alice), 1)
	assert.Empty(t, profile(bob))
	assert.Empty(t, profile(""))

	folders := func(viewer string) []Folder {
		f, err := repo.FindPublicFolders(ctx, alice, viewer)
		require.NoError(t, err)
		return f
	}
	assert.Len(t, folders(alice), 1)
	assert.Empty(t, folders(bob))
	assert.Empty(t, folders(""))

	service := NewService(repo)
	_, err = service.ReadDeck(ctx, alice, public.ID, ReachShared)
	assert.NoError(t, err)
	_, err = service.ReadDeck(ctx, alice, private.ID, ReachShared)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = service.ReadDeck(ctx, bob, public.ID, ReachShared)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = service.ReadDeck(ctx, "", public.ID, ReachShared)
	assert.ErrorIs(t, err, ErrNotFound)
}
