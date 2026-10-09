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

func TestParseMoxfieldDeckList_ReadsMoxfieldTags(t *testing.T) {
	list := "1 Academy Manufactor (BLC) 264 #acceleration #advantage #food generator\n" +
		"1 Sol Ring (SLD) 1011 *F* #!Ramp #ramp\n" +
		"2 Lightning Bolt #removal\n" +
		"1 Island (NEO) 294\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(list))

	require.NoError(t, err)
	require.Len(t, lines, 4)
	assert.Equal(t, "Academy Manufactor", lines[0].CardName)
	assert.Equal(t, "BLC", lines[0].SetCode)
	assert.Equal(t, "264", lines[0].CollectorNumber)
	assert.Equal(t, []string{"acceleration", "advantage", "food generator"}, lines[0].Tags)
	assert.True(t, lines[1].Foil)
	assert.Equal(t, "1011", lines[1].CollectorNumber)
	assert.Equal(t, []string{"Ramp"}, lines[1].Tags)
	assert.Equal(t, "Lightning Bolt", lines[2].CardName)
	assert.Equal(t, 2, lines[2].Quantity)
	assert.Equal(t, []string{"removal"}, lines[2].Tags)
	assert.Nil(t, lines[3].Tags)
}

func TestParseMoxfieldDeckList_ParsesTheListAndEtchedCards(t *testing.T) {
	deck := "1 Cryptic Command (PLST) IMA-48\n" +
		"1 Heartstone (PLST) H09-26\n" +
		"1 Sol Ring (CMM) 464 *E*\n" +
		"1 Lórien Revealed (HOC) 179\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(deck))

	require.NoError(t, err)
	require.Len(t, lines, 4)
	assert.Equal(t, "PLST", lines[0].SetCode)
	assert.Equal(t, "IMA-48", lines[0].CollectorNumber)
	assert.False(t, lines[0].Foil)
	assert.Equal(t, "H09-26", lines[1].CollectorNumber)
	assert.Equal(t, "464", lines[2].CollectorNumber)
	assert.True(t, lines[2].Foil)
	assert.Equal(t, "Lórien Revealed", lines[3].CardName)
}

func TestParseMoxfieldDeckList_ParsesAWholeExport(t *testing.T) {
	lines, err := parseMoxfieldDeckList(strings.NewReader(maeveDecklist))

	require.NoError(t, err)
	require.Len(t, lines, 67)
	total := 0
	for _, line := range lines {
		total += line.Quantity
	}
	assert.Equal(t, 100, total)
	assert.Equal(t, "Maeve, Insidious Singer", lines[0].CardName)
	assert.True(t, lines[0].Foil)
}

func TestParseMoxfieldDeckList_ParsesAPlainList(t *testing.T) {
	deck := "4 Lightning Bolt\n" +
		"1x Sol Ring\n" +
		"1 Fire // Ice\n" +
		"2 Island *F*\n" +
		"1 Counterspell (DMR) 45\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(deck))

	require.NoError(t, err)
	require.Len(t, lines, 5)
	assert.Equal(t, moxfieldDeckLine{LineNo: 1, Quantity: 4, CardName: "Lightning Bolt", Board: "main"}, lines[0])
	assert.Equal(t, moxfieldDeckLine{LineNo: 2, Quantity: 1, CardName: "Sol Ring", Board: "main"}, lines[1])
	assert.Equal(t, "Fire // Ice", lines[2].CardName)
	assert.Equal(t, moxfieldDeckLine{LineNo: 4, Quantity: 2, CardName: "Island", Foil: true, Board: "main"}, lines[3])
	assert.Equal(t, "DMR", lines[4].SetCode)
	assert.Equal(t, "45", lines[4].CollectorNumber)
}

func TestParseMoxfieldDeckList_SkipsSectionHeaders(t *testing.T) {
	deck := "Commander\n1 Maeve, Insidious Singer\n\nDeck\n32 Island\nSideboard:\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(deck))

	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "Maeve, Insidious Singer", lines[0].CardName)
	assert.Equal(t, 32, lines[1].Quantity)
}

func TestParseMoxfieldDeckList_AssignsSectionsToBoards(t *testing.T) {
	list := "1 Maeve, Insidious Singer\n\nSIDEBOARD:\n1 Duress\n\nMaybeboard\n1 Thoughtseize\n\nConsidering:\n1 Opt\nDeck\n1 Island\n"

	lines, err := parseMoxfieldDeckList(strings.NewReader(list))

	require.NoError(t, err)
	require.Len(t, lines, 5)
	boards := make([]string, 0, len(lines))
	for _, line := range lines {
		boards = append(boards, line.Board)
	}
	assert.Equal(t, []string{"main", "sideboard", "considering", "considering", "main"}, boards)
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
