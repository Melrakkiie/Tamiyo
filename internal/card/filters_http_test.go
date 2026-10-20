package card

import (
	"encoding/json"
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
	require.NotNil(t, f.OpenToDeck)
	assert.Equal(t, "0a0a0a0a-0000-0000-0000-000000000001", *f.OpenToDeck)
	assert.Equal(t, "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", f.ScryfallID)
}

func TestHandler_GetCards_AdvancedFilterDefaults(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastFilter.Query)
}

func TestHandler_GetCards_RejectsInvalidAdvancedFilters(t *testing.T) {
	const tooLong = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, tc := range []struct{ query, message string }{
		{"colors=X", "colors must only contain the letters W, U, B, R and G"},
		{"colors=W&color_mode=some", "color_mode must be one of: exact, include, within"},
		{"colors=W&color_mode=Exact", "color_mode must be one of: exact, include, within"},
		{"mana_value=abc", "mana_value must be a number between 0 and 1000000"},
		{"mana_value=-1", "mana_value must be a number between 0 and 1000000"},
		{"mana_value=2&mana_value_op=ne", "mana_value_op must be one of: eq, lt, lte, gt, gte"},
		{"power=*", "power must be a number between -1000000 and 1000000"},
		{"power=-1000001", "power must be a number between -1000000 and 1000000"},
		{"power=2&power_op=ne", "power_op must be one of: eq, lt, lte, gt, gte"},
		{"toughness=X", "toughness must be a number between -1000000 and 1000000"},
		{"toughness=1000001", "toughness must be a number between -1000000 and 1000000"},
		{"toughness=2&toughness_op=ne", "toughness_op must be one of: eq, lt, lte, gt, gte"},
		{"type=Other", "type must be one of: Creature, Planeswalker, Battle, Instant, Sorcery, Artifact, Enchantment, Land"},
		{"type=creature", "type must be one of: Creature, Planeswalker, Battle, Instant, Sorcery, Artifact, Enchantment, Land"},
		{"legal_in=nope", "legal_in must be a Scryfall format, such as commander or modern"},
		{"legal_in=Modern", "legal_in must be a Scryfall format, such as commander or modern"},
		{"color_count=6", "color_count must be an integer between 0 and 5"},
		{"color_count=-1", "color_count must be an integer between 0 and 5"},
		{"color_count=two", "color_count must be an integer between 0 and 5"},
		{"foil=maybe", "foil must be true or false"},
		{"open_to_deck=nope", "open_to_deck must be a deck id"},
		{"scryfall_id=nope", "scryfall_id must be a Scryfall card id"},
		{"scryfall_id=bd8fa327-dd41-4737-8f19", "scryfall_id must be a Scryfall card id"},
		{"subtype=" + tooLong, "subtype must have at most 50 characters"},
		{"storage_type=" + tooLong, "storage_type must have at most 50 characters"},
		{"q=foo:bar", `q: unknown keyword "foo"`},
		{"q=(t:elf", "q: missing closing parenthesis"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			router := setupRouter(&fakeService{})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?"+tc.query, nil))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var body struct{ Error string }
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.message, body.Error)
		})
	}
}

func advancedQuery(t *testing.T, query string) *SearchNode {
	t.Helper()
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?"+query, nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return service.lastFilter.Query
}

func TestHandler_GetCards_TranslatesColorsIntoCriteria(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"colors=gw", `color="WG"`},
		{"colors=gw&color_mode=exact", `color="WG"`},
		{"colors=", `color=""`},
		{"colors=g&color_mode=include", `color>="G"`},
		{"colors=uw&color_mode=within", `color<="WU"`},
		{"colors=g&q=t%3Aelf%20c%3Aw", `(color="G" and (type="elf" and color>="W"))`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			node := advancedQuery(t, tc.query)
			require.NotNil(t, node)
			assert.Equal(t, tc.want, describe(node))
		})
	}
}

func TestHandler_GetCards_TranslatesColorCountIntoCriteria(t *testing.T) {
	assert.Equal(t, `color_count=2`, describe(advancedQuery(t, "color_count=2")))

	node := advancedQuery(t, "color_count=0")
	require.NotNil(t, node)
	assert.Equal(t, SearchCriterion{Field: fieldColorCount, Op: "=", Number: 0}, node.Term)
}

func TestHandler_GetCards_TranslatesFoilIntoCriteria(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"foil=true", `is="foil"`},
		{"foil=1", `is="foil"`},
		{"foil=false", `is="nonfoil"`},
		{"foil=F", `is="nonfoil"`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			node := advancedQuery(t, tc.query)
			require.NotNil(t, node)
			assert.Equal(t, tc.want, describe(node))
		})
	}
}

func TestHandler_GetCards_TranslatesManaValuePowerAndToughnessIntoCriteria(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"mana_value=3", `mana_value=3`},
		{"mana_value=3&mana_value_op=eq", `mana_value=3`},
		{"mana_value=2.5&mana_value_op=lt", `mana_value<2.5`},
		{"mana_value=3&mana_value_op=lte", `mana_value<=3`},
		{"mana_value=3&mana_value_op=gt", `mana_value>3`},
		{"mana_value=3&mana_value_op=gte", `mana_value>=3`},
		{"power=-1&power_op=gte", `power>=-1`},
		{"power=4", `power=4`},
		{"toughness=.5&toughness_op=lt", `toughness<0.5`},
		{"toughness=2&toughness_op=gt", `toughness>2`},
		{"mana_value=3&mana_value_op=lte&power=2&toughness=1&toughness_op=gte", `(mana_value<=3 and power=2 and toughness>=1)`},
		{"colors=g&mana_value=2&q=t%3Aelf", `(color="G" and mana_value=2 and type="elf")`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			node := advancedQuery(t, tc.query)
			require.NotNil(t, node)
			assert.Equal(t, tc.want, describe(node))
		})
	}

	node := advancedQuery(t, "power=0&power_op=lte")
	require.NotNil(t, node)
	assert.Equal(t, SearchCriterion{Field: fieldPower, Op: "<=", Number: 0}, node.Term)
}

func TestHandler_GetCards_TranslatesSubtypeIntoAnInternalCriterion(t *testing.T) {
	assert.Equal(t, `subtype="Elf"`, describe(advancedQuery(t, "subtype=%20Elf%20")))
	assert.Equal(t, `(subtype="vehicle" and type="artifact")`, describe(advancedQuery(t, "subtype=vehicle&q=t%3Aartifact")))
	assert.Nil(t, advancedQuery(t, "subtype=%20%20"))
}

func TestHandler_GetCards_TranslatesStorageTypeIntoAnInternalCriterion(t *testing.T) {
	assert.Equal(t, `storage_type="deckbox"`, describe(advancedQuery(t, "storage_type=%20deckbox%20")))
	assert.Equal(t, `(subtype="elf" and storage_type="Binder" and type="creature")`,
		describe(advancedQuery(t, "storage_type=Binder&subtype=elf&q=t%3Acreature")))
	assert.Nil(t, advancedQuery(t, "storage_type=%20"))
}

func TestParseSearchQuery_HasNoKeywordForInternalCriteria(t *testing.T) {
	for _, query := range []string{"subtype:elf", "storagetype:deckbox", "storage:deckbox"} {
		_, err := parseSearchQuery(query)
		assert.ErrorIs(t, err, errInvalidQuery, query)
	}

	// A keyword is letters only, so this is a name search, not a criterion.
	node, err := parseSearchQuery("storage_type:deckbox")
	require.NoError(t, err)
	assert.Equal(t, `name="storage_type:deckbox"`, describe(node))
}

func TestHandler_GetCards_TranslatesLegalFormatIntoCriteria(t *testing.T) {
	assert.Equal(t, `format="commander"`, describe(advancedQuery(t, "legal_in=commander")))
}

func TestHandler_GetCards_TranslatesTypeIntoCriteria(t *testing.T) {
	assert.Equal(t, `type="Creature"`, describe(advancedQuery(t, "type=Creature")))
	assert.Equal(t, `(type="Artifact" and type="elf")`, describe(advancedQuery(t, "type=Artifact&q=t%3Aelf")))
}

func TestHandler_GetCards_JoinsAdvancedCriteriaWithTheSearchQuery(t *testing.T) {
	node := advancedQuery(t, "colors=gw&color_mode=within&legal_in=commander&color_count=2&foil=true&q=t%3Aelf%20or%20t%3Awarrior")
	require.NotNil(t, node)
	assert.Equal(t, `(color<="WG" and format="commander" and color_count=2 and is="foil" and (type="elf" or type="warrior"))`, describe(node))
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
