package preference

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

type fakeRepository struct {
	stored  map[string]Preferences
	findErr error
	saved   int
}

func (f *fakeRepository) Find(ctx context.Context, userID string) (Preferences, bool, error) {
	if f.findErr != nil {
		return Preferences{}, false, f.findErr
	}
	p, ok := f.stored[userID]
	return p, ok, nil
}

func (f *fakeRepository) Save(ctx context.Context, userID string, p Preferences) error {
	if f.stored == nil {
		f.stored = map[string]Preferences{}
	}
	f.stored[userID] = p
	f.saved++
	return nil
}

func TestService_DefaultsBeforeAnythingIsSaved(t *testing.T) {
	p, err := NewService(&fakeRepository{}).Get(context.Background(), testUserID)

	require.NoError(t, err)
	assert.Equal(t, Preferences{ShowCollectionInDecks: true}, p)
}

func TestService_UpdateOnlyChangesWhatIsGiven(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	off := false

	updated, err := svc.Update(context.Background(), testUserID, Changes{ShowCollectionInDecks: &off})
	require.NoError(t, err)
	assert.False(t, updated.ShowCollectionInDecks)

	unchanged, err := svc.Update(context.Background(), testUserID, Changes{})
	require.NoError(t, err)
	assert.False(t, unchanged.ShowCollectionInDecks)
	assert.Equal(t, 2, repo.saved)
}

func TestService_PropagatesErrors(t *testing.T) {
	svc := NewService(&fakeRepository{findErr: errors.New("db down")})

	_, err := svc.Get(context.Background(), testUserID)
	assert.Error(t, err)
	_, err = svc.Update(context.Background(), testUserID, Changes{})
	assert.Error(t, err)
}

func router(repo *fakeRepository, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	NewHandler(NewService(repo)).RegisterRoutes(r)
	return r
}

func TestHandler_GetAndUpdate(t *testing.T) {
	repo := &fakeRepository{}
	r := router(repo, testUserID)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me/preferences", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"show_collection_in_decks":true}`, w.Body.String())

	req := httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(`{"show_collection_in_decks":false}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"show_collection_in_decks":false}`, w.Body.String())
	assert.False(t, repo.stored[testUserID].ShowCollectionInDecks)
}

func TestHandler_Errors(t *testing.T) {
	w := httptest.NewRecorder()
	router(&fakeRepository{}, "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me/preferences", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	for _, body := range []string{`{}`, `{"show_collection_in_decks":"yes"}`, `nope`} {
		req := httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(&fakeRepository{}, testUserID).ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
}
