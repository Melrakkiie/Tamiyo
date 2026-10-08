package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tagDeckID = "00000000-0000-0000-0000-000000000042"

func taggedRepo() *fakeRepository {
	return &fakeRepository{
		findByIDDeck: Deck{ID: tagDeckID, Name: "Kess"},
		getDeckCards: []DeckCard{{ID: 1, Name: "Sol Ring"}, {ID: 2, Name: "Fire // Ice"}, {ID: 3, Name: "Sol Ring"}},
		pending:      []PendingCard{{ID: 9, Name: "Counterspell"}},
		cardTags: []CardTag{
			{CardName: "sol ring", Tag: "Ramp"},
			{CardName: "sol ring", Tag: "artefact"},
			{CardName: "fire / ice", Tag: "Removal"},
			{CardName: "counterspell", Tag: "Contresort"},
			{CardName: "cultivate", Tag: "Ramp"},
			{CardName: "cultivate", Tag: "Terrains"},
		},
	}
}

func TestCardNameKey(t *testing.T) {
	assert.Equal(t, "fire / ice", CardNameKey("Fire // Ice"))
	assert.Equal(t, "fire / ice", CardNameKey("  fire  /  ICE "))
}

func TestGetCardTags_GroupsTagsByCardInTheDeck(t *testing.T) {
	svc := NewService(taggedRepo())

	tags, err := svc.GetCardTags(context.Background(), testUserID, tagDeckID)

	require.NoError(t, err)
	assert.Equal(t, []string{"artefact", "Contresort", "Ramp", "Removal"}, tags.Tags)
	require.Len(t, tags.Cards, 3)
	assert.Equal(t, TaggedCard{Name: "Counterspell", Tags: []string{"Contresort"}}, tags.Cards[0])
	assert.Equal(t, TaggedCard{Name: "Fire // Ice", Tags: []string{"Removal"}}, tags.Cards[1])
	assert.Equal(t, TaggedCard{Name: "Sol Ring", Tags: []string{"artefact", "Ramp"}}, tags.Cards[2])
	assert.Equal(t, []string{"Removal"}, tags.ByCardName()["fire / ice"])
}

func TestGetCardTags_UnknownDeck(t *testing.T) {
	svc := NewService(&fakeRepository{findByIDErr: ErrNotFound})

	_, err := svc.GetCardTags(context.Background(), testUserID, tagDeckID)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSetCardTags_CleansDedupesAndReusesTheDecksSpelling(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	tagged, err := svc.SetCardTags(context.Background(), testUserID, tagDeckID, "fire / ice", []string{"  removal ", "Pioche", "PIOCHE", "ramp"})

	require.NoError(t, err)
	assert.Equal(t, "fire / ice", repo.replacedName)
	assert.Equal(t, []string{"Removal", "Pioche", "Ramp"}, repo.replacedTags)
	assert.Equal(t, TaggedCard{Name: "Fire // Ice", Tags: []string{"Pioche", "Ramp", "Removal"}}, tagged)
}

func TestSetCardTags_EmptyListClearsTheCard(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	tagged, err := svc.SetCardTags(context.Background(), testUserID, tagDeckID, "Counterspell", []string{})

	require.NoError(t, err)
	assert.Equal(t, "counterspell", repo.replacedName)
	assert.Empty(t, repo.replacedTags)
	assert.Empty(t, tagged.Tags)
}

func TestSetCardTags_Rejections(t *testing.T) {
	tooMany := make([]string, maxTagsPerCard+1)
	for i := range tooMany {
		tooMany[i] = strings.Repeat("x", i+1)
	}
	cases := map[string]struct {
		name string
		tags []string
		want error
	}{
		"card not in deck": {"Cultivate", []string{"Ramp"}, ErrCardNotInDeck},
		"blank tag":        {"Sol Ring", []string{"   "}, ErrInvalidTag},
		"too long":         {"Sol Ring", []string{strings.Repeat("é", maxTagLength+1)}, ErrInvalidTag},
		"too many":         {"Sol Ring", tooMany, ErrTooManyTags},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := taggedRepo()
			svc := NewService(repo)

			_, err := svc.SetCardTags(context.Background(), testUserID, tagDeckID, tc.name, tc.tags)

			assert.ErrorIs(t, err, tc.want)
			assert.Zero(t, repo.tagRepoCalls)
		})
	}
}

func TestSetCardTags_AcceptsTheLongestTag(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	_, err := svc.SetCardTags(context.Background(), testUserID, tagDeckID, "Sol Ring", []string{strings.Repeat("é", maxTagLength)})

	require.NoError(t, err)
}

func TestRenameTag(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	require.NoError(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "ramp", " Accélération "))
	assert.Equal(t, "Ramp", repo.renamedFrom)
	assert.Equal(t, "Accélération", repo.renamedTo)
}

func TestRenameTag_IntoAnExistingTagMergesWithItsSpelling(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	require.NoError(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "Terrains", "ramp"))
	assert.Equal(t, "Terrains", repo.renamedFrom)
	assert.Equal(t, "Ramp", repo.renamedTo)
}

func TestRenameTag_ChangingOnlyTheCaseIsARename(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	require.NoError(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "artefact", "Artefact"))
	assert.Equal(t, "artefact", repo.renamedFrom)
	assert.Equal(t, "Artefact", repo.renamedTo)
}

func TestRenameTag_SameNameDoesNothing(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	require.NoError(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "Ramp", "Ramp"))
	assert.Zero(t, repo.tagRepoCalls)
}

func TestRenameTag_Errors(t *testing.T) {
	svc := NewService(taggedRepo())

	assert.ErrorIs(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "Nope", "Other"), ErrTagNotFound)
	assert.ErrorIs(t, svc.RenameTag(context.Background(), testUserID, tagDeckID, "Ramp", " "), ErrInvalidTag)
}

func TestDeleteTag(t *testing.T) {
	repo := taggedRepo()
	svc := NewService(repo)

	require.NoError(t, svc.DeleteTag(context.Background(), testUserID, tagDeckID, "removal"))
	assert.Equal(t, "Removal", repo.deletedTag)
	assert.ErrorIs(t, svc.DeleteTag(context.Background(), testUserID, tagDeckID, "Nope"), ErrTagNotFound)
}

func TestHandler_GetDeckTags(t *testing.T) {
	service := &fakeService{deckTags: DeckTags{
		Tags:  []string{"Ramp"},
		Cards: []TaggedCard{{Name: "Sol Ring", Tags: []string{"Ramp"}}},
	}}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/"+tagDeckID+"/tags", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"tags":["Ramp"],"cards":[{"name":"Sol Ring","tags":["Ramp"]}]}`, w.Body.String())
	assert.Equal(t, tagDeckID, service.lastDeckID)
}

func TestHandler_GetDeckTags_EmptyListsAreArrays(t *testing.T) {
	router := setupRouter(&fakeService{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/"+tagDeckID+"/tags", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"tags":[],"cards":[]}`, w.Body.String())
}

func TestHandler_SetCardTags(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(http.MethodPut, "/deck/"+tagDeckID+"/tags/cards", `{"name":"Sol Ring","tags":["Ramp","Mana"]}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Sol Ring", service.lastTagName)
	assert.Equal(t, []string{"Ramp", "Mana"}, service.lastTags)
	var body taggedCardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, []string{"Ramp", "Mana"}, body.Tags)
}

func TestHandler_SetCardTags_EmptyListIsAllowed(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(http.MethodPut, "/deck/"+tagDeckID+"/tags/cards", `{"name":"Sol Ring","tags":[]}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"name":"Sol Ring","tags":[]}`, w.Body.String())
}

func TestHandler_RenameAndDeleteTag(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(http.MethodPatch, "/deck/"+tagDeckID+"/tags", `{"from":"Ramp","to":"Mana"}`))
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "Ramp", service.lastTag)
	assert.Equal(t, "Mana", service.lastRenameTo)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/deck/"+tagDeckID+"/tags?tag=Pioche%20%26%20co", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "Pioche & co", service.lastTag)
}

func TestHandler_TagErrors(t *testing.T) {
	cases := map[string]struct {
		method string
		path   string
		body   string
		err    error
		want   int
	}{
		"invalid deck id":    {http.MethodGet, "/deck/nope/tags", "", nil, http.StatusBadRequest},
		"unknown deck":       {http.MethodGet, "/deck/" + tagDeckID + "/tags", "", ErrNotFound, http.StatusNotFound},
		"missing tags":       {http.MethodPut, "/deck/" + tagDeckID + "/tags/cards", `{"name":"Sol Ring"}`, nil, http.StatusBadRequest},
		"card not in deck":   {http.MethodPut, "/deck/" + tagDeckID + "/tags/cards", `{"name":"X","tags":["A"]}`, ErrCardNotInDeck, http.StatusNotFound},
		"invalid tag":        {http.MethodPut, "/deck/" + tagDeckID + "/tags/cards", `{"name":"X","tags":[""]}`, ErrInvalidTag, http.StatusBadRequest},
		"too many tags":      {http.MethodPut, "/deck/" + tagDeckID + "/tags/cards", `{"name":"X","tags":["A"]}`, ErrTooManyTags, http.StatusBadRequest},
		"rename without to":  {http.MethodPatch, "/deck/" + tagDeckID + "/tags", `{"from":"Ramp"}`, nil, http.StatusBadRequest},
		"rename unknown tag": {http.MethodPatch, "/deck/" + tagDeckID + "/tags", `{"from":"A","to":"B"}`, ErrTagNotFound, http.StatusNotFound},
		"delete without tag": {http.MethodDelete, "/deck/" + tagDeckID + "/tags", "", nil, http.StatusBadRequest},
		"delete unknown tag": {http.MethodDelete, "/deck/" + tagDeckID + "/tags?tag=A", "", ErrTagNotFound, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := setupRouter(&fakeService{tagErr: tc.err})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, jsonRequest(tc.method, tc.path, tc.body))

			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
