package emailchange

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/user"
)

// takenUsers holds alice, and bob under his lowercased address, like the database after the migration.
type takenUsers struct{ user.Repository }

func (takenUsers) FindByID(context.Context, string) (user.User, error) {
	return user.User{ID: testUserID, Email: "alice@example.com", PasswordHash: ""}, nil
}

func (takenUsers) FindByEmail(_ context.Context, email string) (user.User, error) {
	if email == "bob@mail.fr" {
		return user.User{ID: "bob"}, nil
	}
	return user.User{}, user.ErrNotFound
}

func TestHandler_RequestEmailChange_RejectsACaseVariantOfATakenAddress(t *testing.T) {
	service := &fakeChangeService{}
	accounts := &fakeAccounts{currentEmail: "alice@example.com"}
	router := setupRouter(service, caseAccounts{fakeAccounts: accounts, users: user.NewService(takenUsers{})}, &fakeMailer{})

	w := post(router, "/auth/email", `{"current_password": "supersecret", "new_email": " Bob@Mail.FR"}`, true)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Empty(t, service.issuedForUser)
}

type caseAccounts struct {
	*fakeAccounts
	users *user.Service
}

func (c caseAccounts) EmailTaken(ctx context.Context, email string) (bool, error) {
	return c.users.EmailTaken(ctx, email)
}
