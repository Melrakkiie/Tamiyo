package token

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePlaintext_Is32RandomBytesInURLSafeBase64(t *testing.T) {
	first, err := GeneratePlaintext()
	require.NoError(t, err)
	second, err := GeneratePlaintext()
	require.NoError(t, err)

	decoded, err := base64.RawURLEncoding.DecodeString(first)
	require.NoError(t, err)
	assert.Len(t, decoded, 32)
	assert.NotEqual(t, first, second)
}

// Pins the stored format: changing it would invalidate every issued session and link.
func TestHash_IsHexSHA256(t *testing.T) {
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Hash("abc"))
}
