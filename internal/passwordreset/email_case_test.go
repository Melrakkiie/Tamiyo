package passwordreset

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/user"
)

// storedUsers holds one account under its lowercased address, like the database after the migration.
type storedUsers struct{ user.Repository }

func (storedUsers) FindByEmail(_ context.Context, email string) (user.User, error) {
	if email != "bob@mail.fr" {
		return user.User{}, user.ErrNotFound
	}
	return user.User{ID: "user-1", Email: email}, nil
}

func (storedUsers) SetPassword(context.Context, string, string) error { return nil }

func TestHandler_ForgotPassword_FindsTheAccountWhateverTheCase(t *testing.T) {
	mailer := &fakeMailer{}
	router := setupRouter(&fakeResetService{issueToken: "reset-token"}, user.NewService(storedUsers{}), &fakeSessionRevoker{}, mailer)

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"email":" Bob@Mail.FR "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "bob@mail.fr", mailer.sentToEmail)
	assert.Equal(t, "reset-token", mailer.sentToken)
}
