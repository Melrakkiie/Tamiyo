package bulkimport

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManaBoxCSV_ParsesRows(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
Atraxa Deck,deck,"Atraxa, Praetors' Voice",CMR,bbbbbbbb-0000-0000-0000-000000000000,2,foil,1
`
	rows, err := parseManaBoxCSV(strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, "Main Binder", rows[0].BinderName)
	assert.Equal(t, "binder", rows[0].BinderType)
	assert.Equal(t, "Sol Ring", rows[0].CardName)
	assert.Equal(t, "CMM", rows[0].SetCode)
	assert.Equal(t, "123", rows[0].CollectorNumber)
	assert.False(t, rows[0].Foil)
	assert.Equal(t, 1, rows[0].Quantity)
	assert.Equal(t, 2, rows[0].LineNo)

	assert.Equal(t, "Atraxa Deck", rows[1].BinderName)
	assert.Equal(t, "deck", rows[1].BinderType)
	assert.True(t, rows[1].Foil)
	assert.Equal(t, 3, rows[1].LineNo)
}

func TestParseManaBoxCSV_RejectsMissingColumn(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,123,,1
`
	_, err := parseManaBoxCSV(strings.NewReader(csv))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
	assert.Contains(t, err.Error(), "Scryfall ID")
}

func TestParseManaBoxCSV_RejectsInvalidQuantity(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,not-a-number
`
	_, err := parseManaBoxCSV(strings.NewReader(csv))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
}

func TestParseManaBoxCSV_EmptyFileReturnsNoRows(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
`
	rows, err := parseManaBoxCSV(strings.NewReader(csv))

	require.NoError(t, err)
	assert.Empty(t, rows)
}
