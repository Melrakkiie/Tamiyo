package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type folderStore struct {
	folders       []Folder
	nextFolderID  int
	deckFolders   map[string]*int
	favorites     map[string]bool
	deletedFolder int
	unknownDeck   bool
}

func (f *folderStore) FindFolders(ctx context.Context, userID string) ([]Folder, error) {
	return f.folders, nil
}

func (f *folderStore) FindPublicFolders(ctx context.Context, ownerID string) ([]Folder, error) {
	return f.folders, nil
}

func (f *folderStore) hasFolder(id *int) bool {
	if id == nil {
		return true
	}
	for _, folder := range f.folders {
		if folder.ID == *id {
			return true
		}
	}
	return false
}

func (f *folderStore) CreateFolder(ctx context.Context, userID string, folder Folder) (Folder, error) {
	if !f.hasFolder(folder.ParentID) {
		return Folder{}, ErrParentFolderNotFound
	}
	f.nextFolderID++
	folder.ID = f.nextFolderID
	f.folders = append(f.folders, folder)
	return folder, nil
}

func (f *folderStore) UpdateFolder(ctx context.Context, userID string, folder Folder) (Folder, error) {
	for i := range f.folders {
		if f.folders[i].ID == folder.ID {
			f.folders[i] = folder
			return folder, nil
		}
	}
	return Folder{}, ErrFolderNotFound
}

func (f *folderStore) DeleteFolder(ctx context.Context, userID string, id int) error {
	if !f.hasFolder(&id) {
		return ErrFolderNotFound
	}
	f.deletedFolder = id
	return nil
}

func (f *folderStore) SetDeckFolder(ctx context.Context, userID string, deckID string, folderID *int) error {
	if f.unknownDeck {
		return ErrNotFound
	}
	if !f.hasFolder(folderID) {
		return ErrTargetFolderNotFound
	}
	if f.deckFolders == nil {
		f.deckFolders = map[string]*int{}
	}
	f.deckFolders[deckID] = folderID
	return nil
}

func (f *folderStore) SetFavorite(ctx context.Context, userID string, deckID string, favorite bool) error {
	if f.unknownDeck {
		return ErrNotFound
	}
	if f.favorites == nil {
		f.favorites = map[string]bool{}
	}
	f.favorites[deckID] = favorite
	return nil
}

func folderTree() *fakeRepository {
	return &fakeRepository{folderStore: folderStore{
		nextFolderID: 3,
		folders: []Folder{
			{ID: 1, Name: "Commander"},
			{ID: 2, Name: "Tribal", ParentID: intPtr(1)},
			{ID: 3, Name: "Elfes", ParentID: intPtr(2)},
		},
	}}
}

func TestCreateFolder_TrimsTheNameAndChecksIt(t *testing.T) {
	repo := folderTree()
	svc := NewService(repo)

	created, err := svc.CreateFolder(context.Background(), testUserID, "  Modern  ", intPtr(1))
	require.NoError(t, err)
	assert.Equal(t, "Modern", created.Name)
	assert.Equal(t, intPtr(1), created.ParentID)

	_, err = svc.CreateFolder(context.Background(), testUserID, "   ", nil)
	assert.ErrorIs(t, err, ErrInvalidFolderName)
	_, err = svc.CreateFolder(context.Background(), testUserID, strings.Repeat("é", 101), nil)
	assert.ErrorIs(t, err, ErrInvalidFolderName)
	_, err = svc.CreateFolder(context.Background(), testUserID, strings.Repeat("é", 100), nil)
	assert.NoError(t, err)
}

func TestUpdateFolder_RenamesCollapsesAndMoves(t *testing.T) {
	repo := folderTree()
	svc := NewService(repo)

	name := " Elves "
	collapsed := true
	updated, err := svc.UpdateFolder(context.Background(), testUserID, 3, FolderChanges{Name: &name, Collapsed: &collapsed, ParentID: intPtr(1)})
	require.NoError(t, err)
	assert.Equal(t, Folder{ID: 3, Name: "Elves", ParentID: intPtr(1), Collapsed: true}, updated)

	updated, err = svc.UpdateFolder(context.Background(), testUserID, 3, FolderChanges{ClearParent: true})
	require.NoError(t, err)
	assert.Nil(t, updated.ParentID)
	assert.Equal(t, "Elves", updated.Name)
}

func TestUpdateFolder_RefusesToGoInsideItself(t *testing.T) {
	svc := NewService(folderTree())

	_, err := svc.UpdateFolder(context.Background(), testUserID, 1, FolderChanges{ParentID: intPtr(3)})
	assert.ErrorIs(t, err, ErrFolderCycle)
	_, err = svc.UpdateFolder(context.Background(), testUserID, 2, FolderChanges{ParentID: intPtr(2)})
	assert.ErrorIs(t, err, ErrFolderCycle)
	_, err = svc.UpdateFolder(context.Background(), testUserID, 2, FolderChanges{ParentID: intPtr(9)})
	assert.ErrorIs(t, err, ErrParentFolderNotFound)
	_, err = svc.UpdateFolder(context.Background(), testUserID, 9, FolderChanges{})
	assert.ErrorIs(t, err, ErrFolderNotFound)
	_, err = svc.UpdateFolder(context.Background(), testUserID, 3, FolderChanges{ParentID: intPtr(1)})
	assert.NoError(t, err)
}

func TestHandler_Folders(t *testing.T) {
	repo := folderTree()
	router := setupRouter(NewService(repo))

	w := sendDeckJSON(router, http.MethodGet, "/deck-folders", "")
	require.Equal(t, http.StatusOK, w.Code)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Len(t, listed, 3)
	assert.Equal(t, "Tribal", listed[1]["name"])
	assert.EqualValues(t, 1, listed[1]["parent_id"])
	assert.Nil(t, listed[0]["parent_id"])
	assert.Equal(t, false, listed[0]["collapsed"])

	w = sendDeckJSON(router, http.MethodPost, "/deck-folders", `{"name": "Modern", "parent_id": 2}`)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"Modern"`)

	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPost, "/deck-folders", `{"name": ""}`).Code)
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPost, "/deck-folders", `{"name": "A", "parent_id": 0}`).Code)
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPost, "/deck-folders", `{"name": "A", "parent_id": 42}`).Code)

	w = sendDeckJSON(router, http.MethodPatch, "/deck-folders/3", `{"collapsed": true, "clear_parent": true}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"collapsed":true`)
	assert.Contains(t, w.Body.String(), `"parent_id":null`)
	w = sendDeckJSON(router, http.MethodPatch, "/deck-folders/1", `{"parent_id": 2}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), ErrFolderCycle.Error())
	assert.Equal(t, http.StatusNotFound, sendDeckJSON(router, http.MethodPatch, "/deck-folders/77", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPatch, "/deck-folders/abc", `{}`).Code)

	assert.Equal(t, http.StatusNoContent, sendDeckJSON(router, http.MethodDelete, "/deck-folders/2", "").Code)
	assert.Equal(t, 2, repo.deletedFolder)
	assert.Equal(t, http.StatusNotFound, sendDeckJSON(router, http.MethodDelete, "/deck-folders/77", "").Code)
}

func TestHandler_MoveDeckAndFavorite(t *testing.T) {
	repo := folderTree()
	router := setupRouter(NewService(repo))
	deckID := "00000000-0000-0000-0000-000000000042"

	assert.Equal(t, http.StatusNoContent, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/folder", `{"folder_id": 2}`).Code)
	assert.Equal(t, intPtr(2), repo.deckFolders[deckID])
	assert.Equal(t, http.StatusNoContent, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/folder", `{"folder_id": null}`).Code)
	assert.Nil(t, repo.deckFolders[deckID])
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/folder", `{"folder_id": 99}`).Code)
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/folder", `{"folder_id": -1}`).Code)

	assert.Equal(t, http.StatusNoContent, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/favorite", "").Code)
	assert.True(t, repo.favorites[deckID])
	assert.Equal(t, http.StatusNoContent, sendDeckJSON(router, http.MethodDelete, "/deck/"+deckID+"/favorite", "").Code)
	assert.False(t, repo.favorites[deckID])

	repo.unknownDeck = true
	assert.Equal(t, http.StatusNotFound, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/favorite", "").Code)
	assert.Equal(t, http.StatusNotFound, sendDeckJSON(router, http.MethodPut, "/deck/"+deckID+"/folder", `{"folder_id": null}`).Code)
}

func TestHandler_UserPublicFolders(t *testing.T) {
	router := setupRouter(NewService(folderTree()))

	w := sendDeckJSON(router, http.MethodGet, "/users/22222222-2222-2222-2222-222222222222/deck-folders", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "collapsed")
	assert.Contains(t, w.Body.String(), `"name":"Elfes"`)
	assert.Equal(t, http.StatusBadRequest, sendDeckJSON(router, http.MethodGet, "/users/nope/deck-folders", "").Code)
}
