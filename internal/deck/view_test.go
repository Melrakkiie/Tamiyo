package deck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const viewDeckID = "00000000-0000-0000-0000-000000000051"

func strPtr(s string) *string { return &s }

func TestGetView_DefaultsToTypeGroupingAndManaValueSort(t *testing.T) {
	svc := NewService(&fakeRepository{findByIDDeck: Deck{ID: viewDeckID}})

	v, err := svc.GetView(context.Background(), testUserID, viewDeckID)

	require.NoError(t, err)
	assert.Equal(t, View{Grouping: strPtr("type"), Sort: "mana_value", CollapsedBoards: []string{"considering"}}, v)
}

func TestGetView_ReturnsTheSavedView(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: viewDeckID}, view: View{Sort: "-added"}, viewFound: true}
	svc := NewService(repo)

	v, err := svc.GetView(context.Background(), testUserID, viewDeckID)

	require.NoError(t, err)
	assert.Equal(t, View{Sort: "-added"}, v)
}

func TestGetView_UnknownDeck(t *testing.T) {
	svc := NewService(&fakeRepository{findByIDErr: ErrNotFound})

	_, err := svc.GetView(context.Background(), testUserID, viewDeckID)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSetView_SavesAValidView(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: viewDeckID}}
	svc := NewService(repo)

	v, err := svc.SetView(context.Background(), testUserID, viewDeckID, View{Grouping: strPtr("tag"), Sort: "-name", CollapsedBoards: []string{}})

	require.NoError(t, err)
	assert.Equal(t, View{Grouping: strPtr("tag"), Sort: "-name", CollapsedBoards: []string{}}, v)
	require.NotNil(t, repo.savedView)
	assert.Equal(t, v, *repo.savedView)

	_, err = svc.SetView(context.Background(), testUserID, viewDeckID, View{Sort: "name"})
	require.NoError(t, err)
	assert.Nil(t, repo.savedView.Grouping)
	assert.Equal(t, []string{"considering"}, repo.savedView.CollapsedBoards)
}

func TestSetView_NormalizesTheCollapsedBoards(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: viewDeckID}}
	svc := NewService(repo)

	v, err := svc.SetView(context.Background(), testUserID, viewDeckID, View{Sort: "name", CollapsedBoards: []string{"considering", "sideboard", "considering"}})

	require.NoError(t, err)
	assert.Equal(t, []string{"sideboard", "considering"}, v.CollapsedBoards)
}

func TestSetView_RejectsInvalidValues(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: viewDeckID}}
	svc := NewService(repo)

	_, err := svc.SetView(context.Background(), testUserID, viewDeckID, View{Grouping: strPtr("rarity"), Sort: "name"})
	assert.ErrorIs(t, err, ErrInvalidViewGrouping)
	_, err = svc.SetView(context.Background(), testUserID, viewDeckID, View{Sort: "color"})
	assert.ErrorIs(t, err, ErrInvalidViewSort)
	_, err = svc.SetView(context.Background(), testUserID, viewDeckID, View{Sort: "name", CollapsedBoards: []string{"main"}})
	assert.ErrorIs(t, err, ErrInvalidViewBoards)
	assert.Nil(t, repo.savedView)
}

func TestHandler_GetView(t *testing.T) {
	router := setupRouter(&fakeService{view: View{Grouping: strPtr("type"), Sort: "mana_value", CollapsedBoards: []string{"considering"}}})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/"+viewDeckID+"/view", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"grouping":"type","sort":"mana_value","collapsed_boards":["considering"]}`, w.Body.String())
}

func TestHandler_SetView(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(http.MethodPut, "/deck/"+viewDeckID+"/view", `{"grouping":null,"sort":"-added","collapsed_boards":["sideboard"]}`))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"grouping":null,"sort":"-added","collapsed_boards":["sideboard"]}`, w.Body.String())
	assert.Equal(t, View{Sort: "-added", CollapsedBoards: []string{"sideboard"}}, service.lastView)
}

func TestHandler_ViewErrors(t *testing.T) {
	cases := map[string]struct {
		method string
		body   string
		err    error
		want   int
	}{
		"unknown deck":     {http.MethodGet, "", ErrNotFound, http.StatusNotFound},
		"missing sort":     {http.MethodPut, `{"grouping":"type"}`, nil, http.StatusBadRequest},
		"invalid grouping": {http.MethodPut, `{"grouping":"x","sort":"name"}`, ErrInvalidViewGrouping, http.StatusBadRequest},
		"invalid sort":     {http.MethodPut, `{"sort":"x"}`, ErrInvalidViewSort, http.StatusBadRequest},
		"invalid boards":   {http.MethodPut, `{"sort":"name","collapsed_boards":["main"]}`, ErrInvalidViewBoards, http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := setupRouter(&fakeService{viewErr: tc.err})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, jsonRequest(tc.method, "/deck/"+viewDeckID+"/view", tc.body))

			assert.Equal(t, tc.want, w.Code)
		})
	}
}
