package card

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_GetCards_PassesAdvancedFilters(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/cards?colors=gw&color_mode=within&mana_value=3&mana_value_op=lte&power=-1&power_op=gte&toughness=.5&toughness_op=lt&type=Creature&subtype=%20Elf%20&legal_in=commander&color_count=2&foil=true&storage_type=deckbox&open_to_deck=0A0A0A0A-0000-0000-0000-000000000001&scryfall_id=BD8FA327-DD41-4737-8F19-2CF5EB1F7CDD", nil))

	require.Equal(t, http.StatusOK, w.Code)
	f := service.lastFilter
	require.NotNil(t, f.Colors)
	assert.Equal(t, "WG", *f.Colors)
	assert.Equal(t, ColorModeWithin, f.ColorMode)
	require.NotNil(t, f.ManaValue)
	assert.Equal(t, 3.0, *f.ManaValue)
	assert.Equal(t, "lte", f.ManaValueOp)
	require.NotNil(t, f.Power)
	assert.Equal(t, -1.0, *f.Power)
	assert.Equal(t, "gte", f.PowerOp)
	require.NotNil(t, f.Toughness)
	assert.Equal(t, 0.5, *f.Toughness)
	assert.Equal(t, "lt", f.ToughnessOp)
	assert.Equal(t, "Creature", f.Type)
	assert.Equal(t, "Elf", f.Subtype)
	assert.Equal(t, "commander", f.LegalIn)
	require.NotNil(t, f.ColorCount)
	assert.Equal(t, 2, *f.ColorCount)
	require.NotNil(t, f.Foil)
	assert.True(t, *f.Foil)
	assert.Equal(t, "deckbox", f.StorageType)
	require.NotNil(t, f.OpenToDeck)
	assert.Equal(t, "0a0a0a0a-0000-0000-0000-000000000001", *f.OpenToDeck)
	assert.Equal(t, "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", f.ScryfallID)
}

func TestHandler_GetCards_AdvancedFilterDefaults(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?colors=&mana_value=2&power=3&toughness=4", nil))

	require.Equal(t, http.StatusOK, w.Code)
	f := service.lastFilter
	require.NotNil(t, f.Colors)
	assert.Equal(t, "", *f.Colors)
	assert.Equal(t, ColorModeExact, f.ColorMode)
	assert.Equal(t, "eq", f.ManaValueOp)
	assert.Equal(t, "eq", f.PowerOp)
	assert.Equal(t, "eq", f.ToughnessOp)
	assert.Nil(t, f.ColorCount)
	assert.Nil(t, f.Foil)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastFilter.Colors)
	assert.Nil(t, service.lastFilter.ManaValue)
	assert.Nil(t, service.lastFilter.Power)
	assert.Nil(t, service.lastFilter.Toughness)
}

func TestHandler_GetCards_RejectsInvalidAdvancedFilters(t *testing.T) {
	for _, query := range []string{
		"colors=X",
		"colors=W&color_mode=some",
		"mana_value=abc",
		"mana_value=-1",
		"mana_value=2&mana_value_op=ne",
		"power=*",
		"power=-1000001",
		"power=2&power_op=ne",
		"toughness=X",
		"toughness=1000001",
		"toughness=2&toughness_op=ne",
		"type=Other",
		"type=creature",
		"legal_in=nope",
		"color_count=6",
		"foil=maybe",
		"open_to_deck=nope",
		"scryfall_id=nope",
		"scryfall_id=bd8fa327-dd41-4737-8f19",
		"subtype=" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"q=foo:bar",
		"q=(t:elf",
	} {
		t.Run(query, func(t *testing.T) {
			router := setupRouter(&fakeService{})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?"+query, nil))

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandler_GetCards_ParsesTheSearchQuery(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?q=t%3Aelf%20c%3Ag", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastFilter.Query)
	assert.Equal(t, `(type="elf" and color>="G")`, describe(service.lastFilter.Query))

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?q=%20", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastFilter.Query)
}

func TestHandler_GetCards_ExplainsAnInvalidSearchQuery(t *testing.T) {
	router := setupRouter(&fakeService{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?q=foo%3Abar", nil))

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error": "q: unknown keyword \"foo\""}`, w.Body.String())
}
