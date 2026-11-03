//go:build integration

package token

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/testdb"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &testDB))
}

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	_, err := testDB.Exec(`
		TRUNCATE TABLE tamiyo.refresh_tokens, tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
}

func seedUser(t *testing.T, db *sqlx.DB) string {
	t.Helper()

	return seedUserWithEmail(t, db, "alice@example.com")
}

func seedUserWithEmail(t *testing.T, db *sqlx.DB, email string) string {
	t.Helper()

	var userID string
	err := db.Get(&userID, `
		INSERT INTO tamiyo.users (email, password_hash)
		VALUES ($1, 'fake-hash')
		RETURNING id
	`, email)
	require.NoError(t, err)

	return userID
}

func TestPostgresRepository_Create_InsertsAndReturnsTokenWithID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, userID, created.UserID)
	assert.Nil(t, created.RevokedAt)
}

func TestPostgresRepository_FindByHash_ReturnsToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	result, err := repo.FindByHash(context.Background(), "fake-hash")

	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
}

func TestPostgresRepository_FindByHash_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByHash(context.Background(), "unknown-hash")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Revoke_SetsRevokedAt(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	err = repo.Revoke(context.Background(), created.ID)
	require.NoError(t, err)

	result, err := repo.FindByHash(context.Background(), "fake-hash")
	require.NoError(t, err)
	assert.NotNil(t, result.RevokedAt)
}

func TestPostgresRepository_FindByID_ReturnsToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), created.ID)

	require.NoError(t, err)
	assert.Equal(t, "fake-hash", result.TokenHash)
	assert.Nil(t, result.ReplacedBy)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Replace_RevokesAndLinksToSuccessor(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	old, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-old", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	successor, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-new", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	replaced, err := repo.Replace(context.Background(), old.ID, successor.ID)
	require.NoError(t, err)
	assert.True(t, replaced)

	result, err := repo.FindByID(context.Background(), old.ID)
	require.NoError(t, err)
	assert.NotNil(t, result.RevokedAt)
	require.NotNil(t, result.ReplacedBy)
	assert.Equal(t, successor.ID, *result.ReplacedBy)
}

func TestPostgresRepository_Replace_AppliesOnceAndReportsFalseTheSecondTime(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	old, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-old", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	first, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-first", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	second, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-second", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	replaced, err := repo.Replace(context.Background(), old.ID, first.ID)
	require.NoError(t, err)
	assert.True(t, replaced)
	replaced, err = repo.Replace(context.Background(), old.ID, second.ID)
	require.NoError(t, err)
	assert.False(t, replaced)

	result, err := repo.FindByID(context.Background(), old.ID)
	require.NoError(t, err)
	require.NotNil(t, result.ReplacedBy)
	assert.Equal(t, first.ID, *result.ReplacedBy)
}

func TestService_Rotate_ConcurrentRotationsLeaveOneUnrevokedToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	service := NewService(NewPostgresRepository(db), time.Hour)

	plaintext, err := service.IssueRefreshToken(context.Background(), userID)
	require.NoError(t, err)

	const callers = 4 // below maxSuccessorHops, so every caller must succeed
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = service.Rotate(context.Background(), plaintext)
		}()
	}
	wg.Wait()

	succeeded := 0
	for _, e := range errs {
		if e == nil {
			succeeded++
		}
		assert.NoError(t, e)
	}
	assert.Equal(t, callers, succeeded)

	var unrevoked int
	require.NoError(t, db.Get(&unrevoked, `SELECT count(*) FROM tamiyo.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, userID))
	assert.Equal(t, 1, unrevoked)
}

func TestPostgresRepository_RevokeAllForUser_RevokesOnlyThatUsersActiveTokens(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db)
	userB := seedUserWithEmail(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	_, err := repo.Create(context.Background(), RefreshToken{UserID: userA, TokenHash: "hash-a1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), RefreshToken{UserID: userA, TokenHash: "hash-a2", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), RefreshToken{UserID: userB, TokenHash: "hash-b1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	err = repo.RevokeAllForUser(context.Background(), userA)
	require.NoError(t, err)

	a1, err := repo.FindByHash(context.Background(), "hash-a1")
	require.NoError(t, err)
	assert.NotNil(t, a1.RevokedAt)

	a2, err := repo.FindByHash(context.Background(), "hash-a2")
	require.NoError(t, err)
	assert.NotNil(t, a2.RevokedAt)

	b1, err := repo.FindByHash(context.Background(), "hash-b1")
	require.NoError(t, err)
	assert.Nil(t, b1.RevokedAt)
}

func TestPostgresRepository_Create_DeletesOnlyThisUsersExpiredTokens(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db)
	userB := seedUserWithEmail(t, db, "bob@example.com")
	_, err := db.Exec(`
		INSERT INTO tamiyo.refresh_tokens (user_id, token_hash, expires_at, revoked_at) VALUES
		($1, 'a-expired', now() - interval '1 hour', NULL),
		($1, 'a-fresh', now() + interval '1 hour', NULL),
		($1, 'a-revoked', now() + interval '1 hour', now()),
		($2, 'b-expired', now() - interval '1 hour', NULL)
	`, userA, userB)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO tamiyo.refresh_tokens (user_id, token_hash, expires_at, revoked_at, replaced_by)
		SELECT $1, 'a-used', now() + interval '1 hour', now(), id FROM tamiyo.refresh_tokens WHERE token_hash = 'a-fresh'
	`, userA)
	require.NoError(t, err)
	repo := NewPostgresRepository(db)

	_, err = repo.Create(context.Background(), RefreshToken{UserID: userA, TokenHash: "a-new", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	_, err = repo.FindByHash(context.Background(), "a-expired")
	assert.ErrorIs(t, err, ErrNotFound)
	for _, h := range []string{"a-fresh", "a-revoked", "a-used", "a-new", "b-expired"} {
		_, err = repo.FindByHash(context.Background(), h)
		assert.NoError(t, err, h)
	}
}

// ageRevocation moves a token's revocation out of the reuse grace period.
func ageRevocation(t *testing.T, db *sqlx.DB, plaintext string) {
	t.Helper()

	_, err := db.Exec(`UPDATE tamiyo.refresh_tokens SET revoked_at = now() - interval '1 minute' WHERE token_hash = $1`, Hash(plaintext))
	require.NoError(t, err)
}

func TestService_Rotate_ReuseEndsOnlyTheReusedTokensSession(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	service := NewService(NewPostgresRepository(db), time.Hour)
	ctx := context.Background()

	first, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	other, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	_, second, err := service.Rotate(ctx, first)
	require.NoError(t, err)
	ageRevocation(t, db, first)

	_, _, err = service.Rotate(ctx, first)
	assert.ErrorIs(t, err, ErrInvalid)

	_, _, err = service.Rotate(ctx, second)
	assert.ErrorIs(t, err, ErrInvalid, "the session's active token is revoked")
	_, _, err = service.Rotate(ctx, other)
	assert.NoError(t, err, "another session of the same user still refreshes")
}

func TestService_Rotate_ReplayingASignedOutTokenIsRefusedWithoutOtherEffect(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	service := NewService(NewPostgresRepository(db), time.Hour)
	ctx := context.Background()

	signedOut, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	other, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	require.NoError(t, service.Revoke(ctx, signedOut))

	_, _, err = service.Rotate(ctx, signedOut)
	assert.ErrorIs(t, err, ErrInvalid)

	_, _, err = service.Rotate(ctx, other)
	assert.NoError(t, err)
}

func TestService_Rotate_ReuseWalkEndsOnAChainWithADiscardedToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	service := NewService(repo, time.Hour)
	ctx := context.Background()

	first, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	other, err := service.IssueRefreshToken(ctx, userID)
	require.NoError(t, err)
	_, second, err := service.Rotate(ctx, first)
	require.NoError(t, err)
	// A token discarded after a lost refresh race: revoked, linked to nothing.
	discarded, err := repo.Create(ctx, RefreshToken{UserID: userID, TokenHash: "hash-discarded", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	require.NoError(t, repo.Revoke(ctx, discarded.ID))
	_, third, err := service.Rotate(ctx, second)
	require.NoError(t, err)
	ageRevocation(t, db, first)

	_, _, err = service.Rotate(ctx, first)
	assert.ErrorIs(t, err, ErrInvalid)

	_, _, err = service.Rotate(ctx, third)
	assert.ErrorIs(t, err, ErrInvalid)
	_, _, err = service.Rotate(ctx, other)
	assert.NoError(t, err)
}

func TestService_Rotate_ReuseNeverTouchesAnotherUsersSessions(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db)
	bob := seedUserWithEmail(t, db, "bob@example.com")
	service := NewService(NewPostgresRepository(db), time.Hour)
	ctx := context.Background()

	first, err := service.IssueRefreshToken(ctx, alice)
	require.NoError(t, err)
	bobToken, err := service.IssueRefreshToken(ctx, bob)
	require.NoError(t, err)
	_, _, err = service.Rotate(ctx, first)
	require.NoError(t, err)
	ageRevocation(t, db, first)

	_, _, err = service.Rotate(ctx, first)
	assert.ErrorIs(t, err, ErrInvalid)

	_, _, err = service.Rotate(ctx, bobToken)
	assert.NoError(t, err)
}
