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
		"/cards?colors=gw&color_mode=within&mana_value=3&mana_value_op=lte&type=Creature&subtype=%20Elf%20&legal_in=commander&color_count=2&foil=true&storage_type=deckbox", nil))

	require.Equal(t, http.StatusOK, w.Code)
	f := service.lastFilter
	require.NotNil(t, f.Colors)
	assert.Equal(t, "WG", *f.Colors)
	assert.Equal(t, ColorModeWithin, f.ColorMode)
	require.NotNil(t, f.ManaValue)
	assert.Equal(t, 3.0, *f.ManaValue)
	assert.Equal(t, "lte", f.ManaValueOp)
	assert.Equal(t, "Creature", f.Type)
	assert.Equal(t, "Elf", f.Subtype)
	assert.Equal(t, "commander", f.LegalIn)
	require.NotNil(t, f.ColorCount)
	assert.Equal(t, 2, *f.ColorCount)
	require.NotNil(t, f.Foil)
	assert.True(t, *f.Foil)
	assert.Equal(t, "deckbox", f.StorageType)
}

func TestHandler_GetCards_AdvancedFilterDefaults(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?colors=&mana_value=2", nil))

	require.Equal(t, http.StatusOK, w.Code)
	f := service.lastFilter
	require.NotNil(t, f.Colors)
	assert.Equal(t, "", *f.Colors)
	assert.Equal(t, ColorModeExact, f.ColorMode)
	assert.Equal(t, "eq", f.ManaValueOp)
	assert.Nil(t, f.ColorCount)
	assert.Nil(t, f.Foil)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastFilter.Colors)
	assert.Nil(t, service.lastFilter.ManaValue)
}

func TestHandler_GetCards_RejectsInvalidAdvancedFilters(t *testing.T) {
	for _, query := range []string{
		"colors=X",
		"colors=W&color_mode=some",
		"mana_value=abc",
		"mana_value=-1",
		"mana_value=2&mana_value_op=ne",
		"type=Other",
		"type=creature",
		"legal_in=nope",
		"color_count=6",
		"foil=maybe",
		"subtype=" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		t.Run(query, func(t *testing.T) {
			router := setupRouter(&fakeService{})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?"+query, nil))

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}
