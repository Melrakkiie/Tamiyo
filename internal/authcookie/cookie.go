package authcookie

import (
	"net/http"
	"time"
)

const DefaultName = "tamiyo_refresh_token"

type RefreshCookie struct {
	Name   string
	Path   string
	Secure bool
	MaxAge time.Duration
}

func New(basePath string, secure bool, maxAge time.Duration) RefreshCookie {
	return RefreshCookie{
		Name:   DefaultName,
		Path:   basePath + "/auth",
		Secure: secure,
		MaxAge: maxAge,
	}
}

func (c RefreshCookie) Set(w http.ResponseWriter, token string) {
	http.SetCookie(w, c.cookie(token, int(c.MaxAge.Seconds())))
}

func (c RefreshCookie) Clear(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie("", -1))
}

func (c RefreshCookie) Read(r *http.Request) string {
	cookie, err := r.Cookie(c.Name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (c RefreshCookie) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     c.Name,
		Value:    value,
		Path:     c.Path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteStrictMode,
	}
}
