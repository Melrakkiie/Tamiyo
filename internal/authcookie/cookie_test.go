package authcookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ScopesCookieToAuthRoutesUnderBasePath(t *testing.T) {
	assert.Equal(t, "/api/auth", New("/api", true, time.Hour).Path)
	assert.Equal(t, "/auth", New("", true, time.Hour).Path)
}

func TestSet_WritesHardenedCookie(t *testing.T) {
	c := New("/api", true, 30*24*time.Hour)
	w := httptest.NewRecorder()

	c.Set(w, "a-refresh-token")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, DefaultName, cookie.Name)
	assert.Equal(t, "a-refresh-token", cookie.Value)
	assert.Equal(t, "/api/auth", cookie.Path)
	assert.Equal(t, 30*24*60*60, cookie.MaxAge)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
}

func TestSet_OmitsSecureFlagWhenDisabled(t *testing.T) {
	c := New("", false, time.Hour)
	w := httptest.NewRecorder()

	c.Set(w, "a-refresh-token")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.False(t, cookies[0].Secure)
}

func TestClear_ExpiresCookieOnSamePath(t *testing.T) {
	c := New("/api", true, time.Hour)
	w := httptest.NewRecorder()

	c.Clear(w)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, DefaultName, cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, "/api/auth", cookies[0].Path)
	assert.Less(t, cookies[0].MaxAge, 0)
}

func TestRead_ReturnsCookieValue(t *testing.T) {
	c := New("/api", true, time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: DefaultName, Value: "from-cookie"})

	assert.Equal(t, "from-cookie", c.Read(req))
}

func TestRead_ReturnsEmptyWhenCookieMissing(t *testing.T) {
	c := New("/api", true, time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)

	assert.Equal(t, "", c.Read(req))
}
