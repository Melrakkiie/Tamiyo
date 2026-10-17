package deck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func groupSizes(groups []EntryGroup) []int {
	sizes := make([]int, 0, len(groups))
	for _, g := range groups {
		sizes = append(sizes, len(g))
	}
	return sizes
}

func TestGroupByPrinting_MergesProxiesWithRealCopiesButKeepsFinishBoardAndCommanderApart(t *testing.T) {
	entries := []DeckEntry{
		{Name: "Atraxa", ScryfallID: "a", Slot: SlotCommander, Board: BoardMain, OwnedCardIDs: []int{1}},
		{Name: "Atraxa", ScryfallID: "a", Board: BoardMain, OwnedCardIDs: []int{2}},
		{Name: "Atraxa", ScryfallID: "A", Board: BoardMain, Proxy: true, PendingIDs: []int{7}, PendingQuantity: 2},
		{Name: "Atraxa", ScryfallID: "a", Board: BoardMain, Foil: true, OwnedCardIDs: []int{3}},
		{Name: "Atraxa", ScryfallID: "a", Board: BoardSideboard, OwnedCardIDs: []int{4}},
		{Name: "Atraxa", ScryfallID: "b", Board: BoardMain, OwnedCardIDs: []int{5}},
	}

	groups := GroupByPrinting(entries)

	assert.Equal(t, []int{1, 2, 1, 1, 1}, groupSizes(groups))
	assert.True(t, groups[0][0].IsCommander())
	assert.Equal(t, 3, groups[1].Quantity())
	assert.Equal(t, 1, groups[1].OwnedQuantity())
	assert.Equal(t, 2, groups[1].PendingQuantity())
}

func TestGroupByCardName_MergesEveryPrintingFinishBoardAndSlotOfANormalizedName(t *testing.T) {
	entries := []DeckEntry{
		{Name: "Fire // Ice", ScryfallID: "a", Slot: SlotCommander, OwnedCardIDs: []int{1}},
		{Name: "Lightning Bolt", ScryfallID: "c", OwnedCardIDs: []int{2}},
		{Name: "fire  / ice", ScryfallID: "b", Foil: true, Proxy: true, Board: BoardSideboard, PendingIDs: []int{7}, PendingQuantity: 3},
	}

	groups := GroupByCardName(entries)

	assert.Equal(t, []int{2, 1}, groupSizes(groups))
	assert.Equal(t, "Fire // Ice", groups[0][0].Name)
	assert.Equal(t, 4, groups[0].Quantity())
	assert.Equal(t, 3, groups[0].PendingQuantity())
	assert.Equal(t, "Lightning Bolt", groups[1][0].Name)
}

func TestGroupByCardName_OfNoEntriesIsEmpty(t *testing.T) {
	assert.Empty(t, GroupByCardName(nil))
}
