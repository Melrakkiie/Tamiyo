package deckshare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

func (f *fakeDeckStore) LikeDeck(ctx context.Context, userID string, deckID string) error {
	if f.likes == nil {
		f.likes = map[[2]string]bool{}
	}
	f.likes[[2]string{userID, deckID}] = true
	return nil
}

func (f *fakeDeckStore) UnlikeDeck(ctx context.Context, userID string, deckID string) error {
	delete(f.likes, [2]string{userID, deckID})
	return nil
}

func (f *fakeDeckStore) GetLikeStatus(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	var status deck.LikeStatus
	for like := range f.likes {
		if like[1] == deckID {
			status.Count++
			status.LikedByMe = status.LikedByMe || like[0] == userID
		}
	}
	return status, nil
}

func (f *fakeDeckStore) GetLikedDecks(ctx context.Context, userID string, page int, limit int) ([]deck.PublicDeck, int, error) {
	var liked []deck.PublicDeck
	for like := range f.likes {
		if like[0] == userID {
			liked = append(liked, deck.PublicDeck{ID: like[1]})
		}
	}
	return liked, len(liked), nil
}

func TestService_LikeSomeoneElsesSharedDeck(t *testing.T) {
	store := compareStore()
	svc := NewService(store, compareUsers(), &fakeInsights{})
	ctx := context.Background()

	status, err := svc.LikeDeck(ctx, viewerID, theirDeck)
	require.NoError(t, err)
	assert.Equal(t, deck.LikeStatus{Count: 1, LikedByMe: true}, status)

	status, err = svc.LikeDeck(ctx, viewerID, theirDeck)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Count)

	status, err = svc.LikeStatus(ctx, strangerID, theirDeck)
	require.NoError(t, err)
	assert.Equal(t, deck.LikeStatus{Count: 1}, status)

	liked, total, err := svc.LikedDecks(ctx, viewerID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, theirDeck, liked[0].ID)

	status, err = svc.UnlikeDeck(ctx, viewerID, theirDeck)
	require.NoError(t, err)
	assert.Equal(t, deck.LikeStatus{}, status)
}

func TestService_LikeRules(t *testing.T) {
	svc := NewService(compareStore(), compareUsers(), &fakeInsights{})
	ctx := context.Background()

	_, err := svc.LikeDeck(ctx, viewerID, myDeckID)
	assert.ErrorIs(t, err, ErrLikeOwnDeck)
	status, err := svc.LikeStatus(ctx, viewerID, myDeckID)
	require.NoError(t, err, "an owner sees the likes of their own deck, even private")
	assert.Equal(t, deck.LikeStatus{}, status)

	_, err = svc.LikeDeck(ctx, viewerID, privateID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = svc.UnlikeDeck(ctx, viewerID, privateID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = svc.LikeStatus(ctx, viewerID, privateID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLikeHandlers(t *testing.T) {
	service := &fakeSharedService{likeStatus: deck.LikeStatus{Count: 3, LikedByMe: true}}
	router := setupProtectedRouter(service, viewerID)

	for method, action := range map[string]string{http.MethodGet: "status", http.MethodPut: "like", http.MethodDelete: "unlike"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/deck/"+theirDeck+"/like", nil))
		require.Equal(t, http.StatusOK, w.Code, method)
		assert.JSONEq(t, `{"likes_count":3,"liked_by_me":true}`, w.Body.String())
		assert.Equal(t, action, service.lastLike)
		assert.Equal(t, theirDeck, service.lastDeckID)
		assert.Equal(t, viewerID, service.lastUserID)
	}

	cases := map[string]struct {
		userID string
		path   string
		err    error
		want   int
	}{
		"unauthenticated": {"", "/deck/" + theirDeck + "/like", nil, http.StatusUnauthorized},
		"invalid id":      {viewerID, "/deck/nope/like", nil, http.StatusNotFound},
		"not visible":     {viewerID, "/deck/" + privateID + "/like", ErrNotFound, http.StatusNotFound},
		"own deck":        {viewerID, "/deck/" + myDeckID + "/like", ErrLikeOwnDeck, http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			setupProtectedRouter(&fakeSharedService{err: tc.err}, tc.userID).ServeHTTP(w, httptest.NewRequest(http.MethodPut, tc.path, nil))
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestLikedDecksHandler(t *testing.T) {
	likedAt := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	service := &fakeSharedService{liked: []deck.PublicDeck{{ID: theirDeck, Name: "Theirs", Format: "commander", LikesCount: 4, LikedAt: &likedAt, OwnerID: strangerID}}}
	router := setupProtectedRouter(service, viewerID)

	w := get(router, "/auth/me/liked-decks?page=2&limit=10")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, service.lastPage)
	assert.Equal(t, 10, service.lastLimit)
	var body struct {
		Data []map[string]any `json:"data"`
		Page int              `json:"page"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "Theirs", body.Data[0]["name"])
	assert.Equal(t, 4.0, body.Data[0]["likes_count"])
	assert.Equal(t, "2026-10-10 14:00:00", body.Data[0]["liked_at"])
	assert.Equal(t, strangerID, body.Data[0]["owner"].(map[string]any)["id"])

	assert.Equal(t, http.StatusBadRequest, get(router, "/auth/me/liked-decks?limit=0").Code)
}
