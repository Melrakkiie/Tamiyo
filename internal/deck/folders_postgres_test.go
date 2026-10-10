//go:build integration

package deck

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func folderNames(folders []Folder) []string {
	names := make([]string, 0, len(folders))
	for _, f := range folders {
		names = append(names, f.Name)
	}
	return names
}

func TestPostgresRepository_Folders(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, alice)
	ctx := context.Background()

	commander, err := repo.CreateFolder(ctx, alice, Folder{Name: "commander"})
	require.NoError(t, err)
	tribal, err := repo.CreateFolder(ctx, alice, Folder{Name: "Tribal", ParentID: &commander.ID})
	require.NoError(t, err)
	elves, err := repo.CreateFolder(ctx, alice, Folder{Name: "Elfes", ParentID: &tribal.ID})
	require.NoError(t, err)
	_, err = repo.CreateFolder(ctx, bob, Folder{Name: "Volé", ParentID: &commander.ID})
	assert.ErrorIs(t, err, ErrParentFolderNotFound)

	folders, err := repo.FindFolders(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, []string{"commander", "Elfes", "Tribal"}, folderNames(folders))
	bobFolders, err := repo.FindFolders(ctx, bob)
	require.NoError(t, err)
	assert.Empty(t, bobFolders)

	elves.Collapsed = true
	elves.Name = "Elves"
	updated, err := repo.UpdateFolder(ctx, alice, elves)
	require.NoError(t, err)
	assert.True(t, updated.Collapsed)
	assert.Equal(t, "Elves", updated.Name)
	_, err = repo.UpdateFolder(ctx, bob, elves)
	assert.ErrorIs(t, err, ErrFolderNotFound)

	deckID := "00000000-0000-0000-0000-000000000001"
	before, err := repo.FindByID(ctx, alice, deckID)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.SetDeckFolder(ctx, alice, deckID, &tribal.ID))
	require.NoError(t, repo.SetFavorite(ctx, alice, deckID, true))
	after, err := repo.FindByID(ctx, alice, deckID)
	require.NoError(t, err)
	assert.Equal(t, &tribal.ID, after.FolderID)
	assert.True(t, after.Favorite)
	assert.Equal(t, before.Updated, after.Updated)

	all, _, err := repo.FindAll(ctx, alice, defaultFilter())
	require.NoError(t, err)
	byID := map[string]Deck{}
	for _, d := range all {
		byID[d.ID] = d
	}
	assert.Equal(t, &tribal.ID, byID[deckID].FolderID)
	assert.Nil(t, byID["00000000-0000-0000-0000-000000000002"].FolderID)

	assert.ErrorIs(t, repo.SetDeckFolder(ctx, bob, deckID, &tribal.ID), ErrNotFound)
	assert.ErrorIs(t, repo.SetFavorite(ctx, bob, deckID, false), ErrNotFound)
	assert.ErrorIs(t, repo.SetDeckFolder(ctx, alice, deckID, intPtr(999999)), ErrTargetFolderNotFound)
	bobFolder, err := repo.CreateFolder(ctx, bob, Folder{Name: "Bob"})
	require.NoError(t, err)
	assert.ErrorIs(t, repo.SetDeckFolder(ctx, alice, deckID, &bobFolder.ID), ErrTargetFolderNotFound)

	copied, err := repo.Create(ctx, alice, Deck{Name: "Copie", Format: "modern", FolderID: &elves.ID})
	require.NoError(t, err)
	assert.Equal(t, &elves.ID, copied.FolderID)
	assert.False(t, copied.Favorite)
	_, err = repo.Create(ctx, alice, Deck{Name: "Copie", Format: "modern", FolderID: &bobFolder.ID})
	assert.ErrorIs(t, err, ErrTargetFolderNotFound)

	renamed, err := repo.Update(ctx, alice, after)
	require.NoError(t, err)
	assert.Equal(t, &tribal.ID, renamed.FolderID)
	assert.True(t, renamed.Favorite)

	require.NoError(t, repo.DeleteFolder(ctx, alice, tribal.ID))
	moved, err := repo.FindByID(ctx, alice, deckID)
	require.NoError(t, err)
	assert.Equal(t, &commander.ID, moved.FolderID)
	folders, err = repo.FindFolders(ctx, alice)
	require.NoError(t, err)
	require.Len(t, folders, 2)
	assert.Equal(t, "Elves", folders[1].Name)
	assert.Equal(t, &commander.ID, folders[1].ParentID)
	assert.ErrorIs(t, repo.DeleteFolder(ctx, alice, tribal.ID), ErrFolderNotFound)
	assert.ErrorIs(t, repo.DeleteFolder(ctx, bob, commander.ID), ErrFolderNotFound)

	require.NoError(t, repo.DeleteFolder(ctx, alice, commander.ID))
	moved, err = repo.FindByID(ctx, alice, deckID)
	require.NoError(t, err)
	assert.Nil(t, moved.FolderID)
}

func TestPostgresRepository_FindPublicFolders(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	commander, err := repo.CreateFolder(ctx, alice, Folder{Name: "Commander"})
	require.NoError(t, err)
	tribal, err := repo.CreateFolder(ctx, alice, Folder{Name: "Tribal", ParentID: &commander.ID})
	require.NoError(t, err)
	secret, err := repo.CreateFolder(ctx, alice, Folder{Name: "Secret"})
	require.NoError(t, err)
	_, err = repo.CreateFolder(ctx, alice, Folder{Name: "Vide", ParentID: &commander.ID})
	require.NoError(t, err)

	_, err = repo.Create(ctx, alice, Deck{Name: "Elfes", Format: "commander", Visibility: VisibilityPublic, FolderID: &tribal.ID})
	require.NoError(t, err)
	_, err = repo.Create(ctx, alice, Deck{Name: "Privé", Format: "commander", Visibility: VisibilityPrivate, FolderID: &secret.ID})
	require.NoError(t, err)
	_, err = repo.Create(ctx, alice, Deck{Name: "Lien", Format: "commander", Visibility: VisibilityUnlisted, FolderID: &secret.ID})
	require.NoError(t, err)

	folders, err := repo.FindPublicFolders(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, []string{"Commander", "Tribal"}, folderNames(folders))
	assert.Equal(t, &commander.ID, folders[1].ParentID)
}
