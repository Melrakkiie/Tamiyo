package bulk

import (
	"fmt"
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

func TestParseManaBoxCSV_QuantityMaximum(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1000
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1001
`
	_, err := parseManaBoxCSV(strings.NewReader(csv))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
	assert.Contains(t, err.Error(), "line 3")

	rows, err := parseManaBoxCSV(strings.NewReader(strings.Split(csv, "\n")[0] + "\n" + strings.Split(csv, "\n")[1] + "\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 1000, rows[0].Quantity)
}

func TestParseManaBoxCSV_EmptyFileReturnsNoRows(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
`
	rows, err := parseManaBoxCSV(strings.NewReader(csv))

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestParseManaBoxCSV_TotalCopiesLimit(t *testing.T) {
	const header = "Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity\n"
	const line = "Main,binder,Plains,TST,aaaaaaaa-0000-0000-0000-000000000000,1,,%d\n"
	file := func(last int) string {
		return header + strings.Repeat(fmt.Sprintf(line, 1000), maxImportCopies/1000-1) + fmt.Sprintf(line, last)
	}

	rows, err := parseManaBoxCSV(strings.NewReader(file(1000)))
	require.NoError(t, err)
	assert.Len(t, rows, maxImportCopies/1000)

	_, err = parseManaBoxCSV(strings.NewReader(header + strings.Repeat(fmt.Sprintf(line, 1000), maxImportCopies/1000) + fmt.Sprintf(line, 1)))
	assert.ErrorIs(t, err, ErrInvalidFile)
}
