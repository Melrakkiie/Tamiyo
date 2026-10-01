package bulk

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMoxfieldCollectionCSV_ParsesRows(t *testing.T) {
	csv := `Count,Tradelist Count,Name,Edition,Condition,Language,Foil,Tags,Last Modified,Collector Number,Alter,Proxy,Purchase Price
1,1,Abrade,blc,Near Mint,French,,,2025-12-22 23:25:53.763000,191,False,False,0.20
2,2,Abzan Devotee,tdm,Near Mint,French,foil,,2025-12-22 23:25:53.763000,68,False,False,0.06
`
	rows, err := parseMoxfieldCollectionCSV(strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, "Abrade", rows[0].CardName)
	assert.Equal(t, "blc", rows[0].SetCode)
	assert.Equal(t, "191", rows[0].CollectorNumber)
	assert.False(t, rows[0].Foil)
	assert.Equal(t, 1, rows[0].Quantity)
	assert.Equal(t, 2, rows[0].LineNo)

	assert.Equal(t, "Abzan Devotee", rows[1].CardName)
	assert.True(t, rows[1].Foil)
	assert.Equal(t, 2, rows[1].Quantity)
}

func TestParseMoxfieldCollectionCSV_ColumnOrderDoesNotMatter(t *testing.T) {
	csv := `Name,Count,Collector Number,Edition,Foil
Sol Ring,1,1011,sld,foil
`
	rows, err := parseMoxfieldCollectionCSV(strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Sol Ring", rows[0].CardName)
	assert.Equal(t, "sld", rows[0].SetCode)
	assert.Equal(t, "1011", rows[0].CollectorNumber)
	assert.True(t, rows[0].Foil)
}

func TestParseMoxfieldCollectionCSV_RejectsMissingColumn(t *testing.T) {
	csv := `Count,Name,Edition,Foil
1,Sol Ring,sld,
`
	_, err := parseMoxfieldCollectionCSV(strings.NewReader(csv))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
	assert.Contains(t, err.Error(), "Collector Number")
}

func TestParseMoxfieldDeckList_ParsesLines(t *testing.T) {
	deck := "1 Terra, Herald of Hope (FIC) 186 *F*\n" +
		"1 Agadeem's Awakening / Agadeem, the Undercrypt (ZNR) 90\n" +
		"3 Mountain (WOE) 265 *F*\n" +
		"1 Celestine, the Living Saint (40K) 10★ *F*\n" +
		"\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(deck))

	require.NoError(t, err)
	require.Len(t, lines, 4)

	assert.Equal(t, 1, lines[0].Quantity)
	assert.Equal(t, "Terra, Herald of Hope", lines[0].CardName)
	assert.Equal(t, "FIC", lines[0].SetCode)
	assert.Equal(t, "186", lines[0].CollectorNumber)
	assert.True(t, lines[0].Foil)
	assert.Equal(t, 1, lines[0].LineNo)

	assert.Equal(t, "Agadeem's Awakening / Agadeem, the Undercrypt", lines[1].CardName)
	assert.Equal(t, "ZNR", lines[1].SetCode)
	assert.Equal(t, "90", lines[1].CollectorNumber)
	assert.False(t, lines[1].Foil)

	assert.Equal(t, 3, lines[2].Quantity)
	assert.Equal(t, "Mountain", lines[2].CardName)

	assert.Equal(t, "10★", lines[3].CollectorNumber)
	assert.True(t, lines[3].Foil)
}

func TestParseMoxfieldDeckList_RejectsUnrecognizedLine(t *testing.T) {
	_, err := parseMoxfieldDeckList(strings.NewReader("not a valid decklist line\n"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
}

func TestParseMoxfieldDeckList_RejectsEmptyFile(t *testing.T) {
	_, err := parseMoxfieldDeckList(strings.NewReader("\n\n"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
}
