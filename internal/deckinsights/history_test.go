package deckinsights

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupHistory_OrdersDaysNewestFirstThenBoardsThenNames(t *testing.T) {
	rows := []HistoryRow{
		{Date: "2026-10-28", Board: "main", Name: "Island", Count: 1},
		{Date: "2026-10-29", Board: "considering", Name: "Sol Ring", Count: -1},
		{Date: "2026-10-29", Board: "main", Name: "Sol Ring", Count: -1},
		{Date: "2026-10-29", Board: "main", Name: "Island", Count: 2},
		{Date: "2026-10-29", Board: "sideboard", Name: "Opt", Count: 1},
	}

	got := groupHistory(rows)

	assert.Equal(t, DeckHistory{Days: []HistoryDay{
		{Date: "2026-10-29", Boards: []HistoryBoard{
			{Board: "main", Changes: []HistoryChange{{Name: "Island", Count: 2}, {Name: "Sol Ring", Count: -1}}},
			{Board: "sideboard", Changes: []HistoryChange{{Name: "Opt", Count: 1}}},
			{Board: "considering", Changes: []HistoryChange{{Name: "Sol Ring", Count: -1}}},
		}},
		{Date: "2026-10-28", Boards: []HistoryBoard{
			{Board: "main", Changes: []HistoryChange{{Name: "Island", Count: 1}}},
		}},
	}}, got)
}
