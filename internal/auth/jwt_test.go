package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken_ThenParseToken_ReturnsSameUserID(t *testing.T) {
	token, err := GenerateToken("test-secret", "11111111-1111-1111-1111-111111111111", time.Hour)
	require.NoError(t, err)

	userID, err := ParseToken("test-secret", token)

	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", userID)
}

func TestParseToken_ReturnsErrorOnWrongSecret(t *testing.T) {
	token, err := GenerateToken("test-secret", "11111111-1111-1111-1111-111111111111", time.Hour)
	require.NoError(t, err)

	_, err = ParseToken("wrong-secret", token)

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestParseToken_ReturnsErrorOnExpiredToken(t *testing.T) {
	token, err := GenerateToken("test-secret", "11111111-1111-1111-1111-111111111111", -time.Hour) // already expired
	require.NoError(t, err)

	_, err = ParseToken("test-secret", token)

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestParseToken_ReturnsErrorOnGarbageToken(t *testing.T) {
	_, err := ParseToken("test-secret", "not-a-valid-jwt")

	assert.ErrorIs(t, err, ErrInvalidToken)
}
