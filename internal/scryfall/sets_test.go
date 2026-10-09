package scryfall

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFakeSets(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	original := setsURL
	setsURL = server.URL + "/sets"
	t.Cleanup(func() { setsURL = original })

	withRequestInterval(t, 0)
	return server
}

func TestClient_SetNamesFollowsPages(t *testing.T) {
	var server *httptest.Server
	server = withFakeSets(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `{"data":[{"code":"fem","name":"Fallen Empires"}],"has_more":false}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"code":"MMA","name":"Modern Masters"}],"has_more":true,"next_page":"%s/sets?page=2"}`, server.URL)
	})

	names, err := NewClient().SetNames(context.Background())

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"mma": "Modern Masters", "fem": "Fallen Empires"}, names)
}

func TestClient_SetNamesFailsOnAnError(t *testing.T) {
	withFakeSets(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := NewClient().SetNames(context.Background())

	assert.ErrorContains(t, err, "status 500")
}
