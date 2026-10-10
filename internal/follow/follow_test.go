package follow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	alice = "11111111-1111-1111-1111-111111111111"
	bob   = "22222222-2222-2222-2222-222222222222"
	carol = "33333333-3333-3333-3333-333333333333"
)

type fakeRepo struct {
	users   map[string]bool
	follows map[[2]string]time.Time
	err     error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[string]bool{alice: true, bob: true, carol: true}, follows: map[[2]string]time.Time{}}
}

func (f *fakeRepo) UserExists(ctx context.Context, userID string) (bool, error) {
	return f.users[userID], f.err
}

func (f *fakeRepo) Follow(ctx context.Context, followerID string, followedID string) error {
	if _, done := f.follows[[2]string{followerID, followedID}]; !done {
		f.follows[[2]string{followerID, followedID}] = time.Date(2026, 10, 9, 12, len(f.follows), 0, 0, time.UTC)
	}
	return nil
}

func (f *fakeRepo) Unfollow(ctx context.Context, followerID string, followedID string) error {
	delete(f.follows, [2]string{followerID, followedID})
	return nil
}

func (f *fakeRepo) Status(ctx context.Context, viewerID string, userID string) (Status, error) {
	var s Status
	for pair := range f.follows {
		if pair[1] == userID {
			s.Followers++
		}
		if pair[0] == userID {
			s.Following++
		}
	}
	_, s.FollowedByMe = f.follows[[2]string{viewerID, userID}]
	_, s.FollowsMe = f.follows[[2]string{userID, viewerID}]
	return s, nil
}

func (f *fakeRepo) list(viewerID string, userID string, followers bool) []Connection {
	var out []Connection
	for pair, since := range f.follows {
		owner, other := pair[0], pair[1]
		if followers {
			owner, other = pair[1], pair[0]
		}
		if owner == userID {
			_, mine := f.follows[[2]string{viewerID, other}]
			out = append(out, Connection{ID: other, Since: since, FollowedByMe: mine})
		}
	}
	return out
}

func (f *fakeRepo) Followers(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	out := f.list(viewerID, userID, true)
	return out, len(out), nil
}

func (f *fakeRepo) Following(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	out := f.list(viewerID, userID, false)
	return out, len(out), nil
}

func TestService_FollowAndUnfollow(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	status, err := svc.Follow(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, Status{Followers: 1, FollowedByMe: true}, status)

	_, err = svc.Follow(ctx, alice, bob)
	require.NoError(t, err, "following twice changes nothing")
	_, err = svc.Follow(ctx, bob, alice)
	require.NoError(t, err)

	status, err = svc.Status(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, Status{Followers: 1, Following: 1, FollowedByMe: true, FollowsMe: true}, status)

	status, err = svc.Unfollow(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, Status{Following: 1, FollowsMe: true}, status)
	_, err = svc.Unfollow(ctx, alice, bob)
	require.NoError(t, err, "unfollowing twice changes nothing")
}

func TestService_RefusesSelfAndUnknownUsers(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()
	unknown := "44444444-4444-4444-4444-444444444444"

	_, err := svc.Follow(ctx, alice, alice)
	assert.ErrorIs(t, err, ErrSelfFollow)
	_, err = svc.Follow(ctx, alice, unknown)
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = svc.Unfollow(ctx, alice, unknown)
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = svc.Status(ctx, alice, unknown)
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, _, err = svc.Followers(ctx, alice, unknown, Page{Number: 1, Limit: 10})
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, _, err = svc.Following(ctx, alice, unknown, Page{Number: 1, Limit: 10})
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestService_PropagatesRepositoryErrors(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("db down")

	_, err := NewService(repo).Status(context.Background(), alice, bob)

	assert.EqualError(t, err, "db down")
}

func router(t *testing.T, repo *fakeRepo, viewer string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if viewer != "" {
			c.Set("user_id", viewer)
		}
		c.Next()
	})
	NewHandler(NewService(repo)).RegisterRoutes(r)
	return r
}

func call(r *gin.Engine, method string, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestHandler_FollowStatusAndUnfollow(t *testing.T) {
	repo := newFakeRepo()
	r := router(t, repo, alice)

	w := call(r, http.MethodPut, "/users/"+bob+"/follow")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"followers_count":1,"following_count":0,"followed_by_me":true,"follows_me":false}`, w.Body.String())

	w = call(r, http.MethodGet, "/users/"+bob+"/follow")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"followed_by_me":true`)

	w = call(r, http.MethodDelete, "/users/"+bob+"/follow")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"followers_count":0`)
}

func TestHandler_ListsConnections(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	_, _ = svc.Follow(ctx, carol, bob)
	_, _ = svc.Follow(ctx, alice, carol)
	r := router(t, repo, alice)

	w := call(r, http.MethodGet, "/users/"+bob+"/followers?page=1&limit=10")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []struct {
			ID           string `json:"id"`
			FollowedByMe bool   `json:"followed_by_me"`
			Since        string `json:"since"`
		} `json:"data"`
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, carol, body.Data[0].ID)
	assert.True(t, body.Data[0].FollowedByMe)
	assert.NotEmpty(t, body.Data[0].Since)
	assert.Equal(t, 1, body.Page)
	assert.Equal(t, 10, body.Limit)
	assert.Equal(t, 1, body.Total)
	assert.Equal(t, 1, body.TotalPages)

	w = call(r, http.MethodGet, "/users/"+carol+"/following")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), bob)
	assert.Contains(t, w.Body.String(), `"limit":24`)

	w = call(r, http.MethodGet, "/users/"+alice+"/followers")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}

func TestHandler_Errors(t *testing.T) {
	r := router(t, newFakeRepo(), alice)
	unknown := "44444444-4444-4444-4444-444444444444"

	cases := map[string]struct {
		method string
		path   string
		want   int
	}{
		"self":          {http.MethodPut, "/users/" + alice + "/follow", http.StatusBadRequest},
		"unknown user":  {http.MethodPut, "/users/" + unknown + "/follow", http.StatusNotFound},
		"unknown list":  {http.MethodGet, "/users/" + unknown + "/followers", http.StatusNotFound},
		"invalid id":    {http.MethodGet, "/users/nope/follow", http.StatusBadRequest},
		"invalid page":  {http.MethodGet, "/users/" + bob + "/followers?page=0", http.StatusBadRequest},
		"invalid limit": {http.MethodGet, "/users/" + bob + "/following?limit=500", http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, call(r, tc.method, tc.path).Code)
		})
	}

	assert.Equal(t, http.StatusUnauthorized, call(router(t, newFakeRepo(), ""), http.MethodGet, "/users/"+bob+"/follow").Code)
}
