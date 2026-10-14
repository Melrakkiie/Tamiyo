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
	assert.Equal(t, Preferences{ShowCollectionInDecks: true, CardDisplay: "images", CardColumns: 6, ShowManaCost: true, ShowCardDetails: true, CollectionSort: "-added", Theme: "auto", NightStartsAt: 22, NightEndsAt: 6, PriceCurrency: "eur", PlaytestZoom: 100}, p)
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

func TestService_ValidatesCardDisplay(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	text, unknown := "text", "mosaic"
	columns, tooMany, tooFew := 9, 13, 1
	off := false

	updated, err := svc.Update(context.Background(), testUserID, Changes{CardDisplay: &text, CardColumns: &columns, ShowManaCost: &off})
	require.NoError(t, err)
	assert.Equal(t, Preferences{ShowCollectionInDecks: true, CardDisplay: "text", CardColumns: 9, ShowManaCost: false, ShowCardDetails: true, CollectionSort: "-added", Theme: "auto", NightStartsAt: 22, NightEndsAt: 6, PriceCurrency: "eur", PlaytestZoom: 100}, updated)

	for _, display := range []string{"images", "grid", "stacks"} {
		_, err = svc.Update(context.Background(), testUserID, Changes{CardDisplay: &display})
		require.NoError(t, err)
	}
	_, err = svc.Update(context.Background(), testUserID, Changes{CardDisplay: &unknown})
	assert.ErrorIs(t, err, ErrInvalidCardDisplay)
	_, err = svc.Update(context.Background(), testUserID, Changes{CardColumns: &tooMany})
	assert.ErrorIs(t, err, ErrInvalidCardColumns)
	_, err = svc.Update(context.Background(), testUserID, Changes{CardColumns: &tooFew})
	assert.ErrorIs(t, err, ErrInvalidCardColumns)
	assert.Equal(t, 4, repo.saved)
}

func TestService_SavesTheCollectionGroupingAndSort(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	color, sort, unknown := "color", "-mana_value", "rarity"

	updated, err := svc.Update(context.Background(), testUserID, Changes{CollectionGrouping: &color, CollectionSort: &sort})
	require.NoError(t, err)
	assert.Equal(t, &color, updated.CollectionGrouping)
	assert.Equal(t, "-mana_value", updated.CollectionSort)

	updated, err = svc.Update(context.Background(), testUserID, Changes{ClearCollectionGrouping: true})
	require.NoError(t, err)
	assert.Nil(t, updated.CollectionGrouping)
	assert.Equal(t, "-mana_value", updated.CollectionSort)

	_, err = svc.Update(context.Background(), testUserID, Changes{CollectionGrouping: &unknown})
	assert.ErrorIs(t, err, ErrInvalidCollectionGrouping)
	_, err = svc.Update(context.Background(), testUserID, Changes{CollectionSort: &unknown})
	assert.ErrorIs(t, err, ErrInvalidCollectionSort)
}

func TestService_SavesTheThemeAndNightHours(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	night, unknown := "night", "dusk"
	start, end, tooLate, sameAsEnd := 21, 7, 24, 6

	updated, err := svc.Update(context.Background(), testUserID, Changes{Theme: &night})
	require.NoError(t, err)
	assert.Equal(t, "night", updated.Theme)
	assert.Equal(t, 22, updated.NightStartsAt)
	assert.Equal(t, 6, updated.NightEndsAt)

	updated, err = svc.Update(context.Background(), testUserID, Changes{NightStartsAt: &start, NightEndsAt: &end})
	require.NoError(t, err)
	assert.Equal(t, "night", updated.Theme)
	assert.Equal(t, 21, updated.NightStartsAt)
	assert.Equal(t, 7, updated.NightEndsAt)

	_, err = svc.Update(context.Background(), testUserID, Changes{Theme: &unknown})
	assert.ErrorIs(t, err, ErrInvalidTheme)
	_, err = svc.Update(context.Background(), testUserID, Changes{NightStartsAt: &tooLate})
	assert.ErrorIs(t, err, ErrInvalidNightHour)
	_, err = svc.Update(context.Background(), testUserID, Changes{NightEndsAt: &start})
	assert.ErrorIs(t, err, ErrEmptyNight)
	_, err = NewService(&fakeRepository{}).Update(context.Background(), testUserID, Changes{NightStartsAt: &sameAsEnd})
	assert.ErrorIs(t, err, ErrEmptyNight)
	assert.Equal(t, 2, repo.saved)
}

func TestService_SavesThePricePreferences(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	on := true
	usd, unknown := "usd", "gbp"

	updated, err := svc.Update(context.Background(), testUserID, Changes{ShowPrices: &on})
	require.NoError(t, err)
	assert.True(t, updated.ShowPrices)
	assert.Equal(t, "eur", updated.PriceCurrency)

	updated, err = svc.Update(context.Background(), testUserID, Changes{PriceCurrency: &usd})
	require.NoError(t, err)
	assert.True(t, updated.ShowPrices)
	assert.Equal(t, "usd", updated.PriceCurrency)

	_, err = svc.Update(context.Background(), testUserID, Changes{PriceCurrency: &unknown})
	assert.ErrorIs(t, err, ErrInvalidPriceCurrency)
	assert.Equal(t, 2, repo.saved)
}

func TestService_SavesThePlaytestZoom(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	zoom, tooSmall, tooLarge := 130, 49, 201

	updated, err := svc.Update(context.Background(), testUserID, Changes{PlaytestZoom: &zoom})
	require.NoError(t, err)
	assert.Equal(t, 130, updated.PlaytestZoom)
	assert.Equal(t, "eur", updated.PriceCurrency)

	_, err = svc.Update(context.Background(), testUserID, Changes{PlaytestZoom: &tooSmall})
	assert.ErrorIs(t, err, ErrInvalidPlaytestZoom)
	_, err = svc.Update(context.Background(), testUserID, Changes{PlaytestZoom: &tooLarge})
	assert.ErrorIs(t, err, ErrInvalidPlaytestZoom)
	assert.Equal(t, 1, repo.saved)
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
	assert.JSONEq(t, `{"show_collection_in_decks":true,"card_display":"images","card_columns":6,"show_mana_cost":true,"show_card_details":true,"collection_grouping":null,"collection_sort":"-added","theme":"auto","night_starts_at":22,"night_ends_at":6,"show_prices":false,"price_currency":"eur","playtest_zoom":100}`, w.Body.String())

	req := httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(`{"show_collection_in_decks":false}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"show_collection_in_decks":false,"card_display":"images","card_columns":6,"show_mana_cost":true,"show_card_details":true,"collection_grouping":null,"collection_sort":"-added","theme":"auto","night_starts_at":22,"night_ends_at":6,"show_prices":false,"price_currency":"eur","playtest_zoom":100}`, w.Body.String())
	assert.False(t, repo.stored[testUserID].ShowCollectionInDecks)

	req = httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(`{"show_prices":true,"price_currency":"usd"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"show_collection_in_decks":false,"card_display":"images","card_columns":6,"show_mana_cost":true,"show_card_details":true,"collection_grouping":null,"collection_sort":"-added","theme":"auto","night_starts_at":22,"night_ends_at":6,"show_prices":true,"price_currency":"usd","playtest_zoom":100}`, w.Body.String())

	req = httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(`{"card_display":"text","card_columns":4,"show_mana_cost":false,"show_card_details":false,"collection_grouping":null,"collection_sort":"-added","theme":"auto","night_starts_at":22,"night_ends_at":6}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"show_collection_in_decks":false,"card_display":"text","card_columns":4,"show_mana_cost":false,"show_card_details":false,"collection_grouping":null,"collection_sort":"-added","theme":"auto","night_starts_at":22,"night_ends_at":6,"show_prices":true,"price_currency":"usd","playtest_zoom":100}`, w.Body.String())
}

func TestHandler_Errors(t *testing.T) {
	w := httptest.NewRecorder()
	router(&fakeRepository{}, "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me/preferences", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	for _, body := range []string{`{}`, `{"show_collection_in_decks":"yes"}`, `nope`, `{"card_display":"mosaic"}`, `{"card_columns":13}`, `{"collection_sort":"rarity"}`, `{"collection_grouping":"storage"}`, `{"theme":"dusk"}`, `{"night_starts_at":24}`, `{"night_ends_at":-1}`, `{"night_starts_at":6}`, `{"price_currency":"gbp"}`, `{"show_prices":"yes"}`, `{"playtest_zoom":49}`, `{"playtest_zoom":201}`} {
		req := httptest.NewRequest(http.MethodPatch, "/auth/me/preferences", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(&fakeRepository{}, testUserID).ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
}
