package bulk

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

func ptr(i int) *int { return &i }

// --- ExportManaBox --------------------------------------------------------

func TestExportManaBox_GroupsIdenticalCardsByStorageIntoOneRowWithQuantity(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "CMM", ScryfallID: "aaaa", CollectorNumber: "123", StorageID: ptr(7)},
		{Name: "Sol Ring", SetCode: "CMM", ScryfallID: "aaaa", CollectorNumber: "123", StorageID: ptr(7)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Main Binder", Type: "binder"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	err := svc.ExportManaBox(context.Background(), testUserID, &buf)

	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity")
	assert.Contains(t, out, "Main Binder,binder,Sol Ring,CMM,aaaa,123,,2")
	assert.Equal(t, 1, len(splitCSVLines(out))-1) // the two identical copies collapse into a single row
}

func TestExportManaBox_UsesDeckBinderTypeForDeckStorages(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", StorageID: ptr(9)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 9, Name: "Atraxa Deck", Type: "deck"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, &buf))

	assert.Contains(t, buf.String(), "Atraxa Deck,deck,Sol Ring,CMM,,123,,1")
}

func TestExportManaBox_GroupsCardsWithNilStorageUnderUnsortedBucketSortedLast(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Lightning Bolt", SetCode: "CMM", CollectorNumber: "456", StorageID: nil},
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", StorageID: ptr(7)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Main Binder", Type: "binder"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 3) // header + 2 rows
	assert.Contains(t, lines[1], "Main Binder")
	assert.Contains(t, lines[2], "Unsorted,binder,Lightning Bolt")
}

func TestExportManaBox_DistinguishesFoilFromNonFoilAsSeparateRows(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", Foil: false, StorageID: ptr(7)},
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", Foil: true, StorageID: ptr(7)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Main Binder", Type: "binder"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], ",123,,1")
	assert.Contains(t, lines[2], ",123,foil,1")
}

func TestExportManaBox_PropagatesCardLoadError(t *testing.T) {
	cards := &fakeCardService{getAllErr: errors.New("db down")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportManaBox(context.Background(), testUserID, &bytes.Buffer{})

	require.Error(t, err)
}

func TestExportManaBox_PropagatesStorageLoadError(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{Name: "Sol Ring", StorageID: ptr(7)}}}
	storages := &fakeStorageService{getAllErr: errors.New("db down")}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportManaBox(context.Background(), testUserID, &bytes.Buffer{})

	require.Error(t, err)
}

func TestExportManaBox_EmptyCollectionProducesHeaderOnly(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, &buf))

	assert.Equal(t, 1, len(splitCSVLines(buf.String())))
}

// --- ExportMoxfieldCollection ---------------------------------------------

func TestExportMoxfieldCollection_GroupsAcrossStoragesRegardlessOfBucket(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011", StorageID: ptr(7)},
		{Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011", StorageID: nil},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 2) // header + one merged row
	assert.Equal(t, "Count,Name,Edition,Foil,Collector Number", lines[0])
	assert.Contains(t, lines[1], "2,Sol Ring,sld,,1011")
}

func TestExportMoxfieldCollection_LowercasesSetCode(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, &buf))

	assert.Contains(t, buf.String(), ",sld,")
}

func TestExportMoxfieldCollection_MarksFoilCardsAndSortsByNameThenSetThenCollectorNumber(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011", Foil: true},
		{Name: "Counterspell", SetCode: "CLB", CollectorNumber: "2"},
		{Name: "Counterspell", SetCode: "CLB", CollectorNumber: "1"},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 4) // header + 3 rows
	assert.Contains(t, lines[1], "Counterspell,clb,,1")
	assert.Contains(t, lines[2], "Counterspell,clb,,2")
	assert.Contains(t, lines[3], "1,Sol Ring,sld,foil,1011")
}

func TestExportMoxfieldCollection_PropagatesCardLoadError(t *testing.T) {
	cards := &fakeCardService{getAllErr: errors.New("db down")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportMoxfieldCollection(context.Background(), testUserID, &bytes.Buffer{})

	require.Error(t, err)
}

// --- ExportMoxfieldDeck ----------------------------------------------------

func TestExportMoxfieldDeck_CommanderLineIsWrittenFirst(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: 1, Name: "Atraxa", CommanderID: ptr(100)}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 100, Name: "Atraxa, Praetors' Voice", SetCode: "CMR", CollectorNumber: "1"},
				{ID: 101, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	err := svc.ExportMoxfieldDeck(context.Background(), testUserID, 1, &buf)

	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "1 Atraxa, Praetors' Voice (CMR) 1", lines[0])
	assert.Equal(t, "1 Sol Ring (SLD) 1011", lines[1])
}

func TestExportMoxfieldDeck_CommanderLineIncludesFullQuantityOfThatPrinting(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: 1, Name: "Mono Mountain", CommanderID: ptr(200)}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 200, Name: "Mountain", SetCode: "WOE", CollectorNumber: "265"},
				{ID: 201, Name: "Mountain", SetCode: "WOE", CollectorNumber: "265"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldDeck(context.Background(), testUserID, 1, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t, "2 Mountain (WOE) 265", lines[0])
}

func TestExportMoxfieldDeck_NoCommanderSortsAlphabeticallyWithNoSpecialFirstLine(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: 1, Name: "Pile"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 1, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"},
				{ID: 2, Name: "Lightning Bolt", SetCode: "CMM", CollectorNumber: "456"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldDeck(context.Background(), testUserID, 1, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "1 Lightning Bolt (CMM) 456", lines[0])
	assert.Equal(t, "1 Sol Ring (SLD) 1011", lines[1])
}

func TestExportMoxfieldDeck_FoilSuffix(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: 1, Name: "Pile"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 1, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011", Foil: true}},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldDeck(context.Background(), testUserID, 1, &buf))

	assert.Equal(t, "1 Sol Ring (SLD) 1011 *F*\n", buf.String())
}

func TestExportMoxfieldDeck_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportMoxfieldDeck(context.Background(), testUserID, 999, &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestExportMoxfieldDeck_PropagatesGetDeckCardsError(t *testing.T) {
	decks := &fakeDeckService{
		decks:           []deck.Deck{{ID: 1, Name: "Pile"}},
		getDeckCardsErr: errors.New("db down"),
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	err := svc.ExportMoxfieldDeck(context.Background(), testUserID, 1, &bytes.Buffer{})

	require.Error(t, err)
}

// splitCSVLines splits encoding/csv's output into lines, dropping the
// trailing empty line left after the final record's newline.
func splitCSVLines(s string) []string {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
