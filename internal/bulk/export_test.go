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
	err := svc.ExportManaBox(context.Background(), testUserID, nil, &buf)

	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity")
	assert.Contains(t, out, "Main Binder,binder,Sol Ring,CMM,aaaa,123,,2")
	assert.Equal(t, 1, len(splitCSVLines(out))-1) // the two identical copies collapse into a single row
}

func TestExportManaBox_UsesDeckBinderTypeForDeckboxStorages(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", StorageID: ptr(9)},
		{Name: "Island", SetCode: "NEO", CollectorNumber: "294", StorageID: ptr(10)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{
		{ID: 9, Name: "Atraxa Deck", Type: "deckbox"},
		{ID: 10, Name: "Old Deck", Type: "Deck"},
	}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, nil, &buf))

	assert.Contains(t, buf.String(), "Atraxa Deck,deck,Sol Ring,CMM,,123,,1")
	assert.Contains(t, buf.String(), "Old Deck,deck,Island,NEO,,294,,1")
}

func TestExportManaBox_GroupsCardsWithNilStorageUnderUnsortedBucketSortedLast(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Lightning Bolt", SetCode: "CMM", CollectorNumber: "456", StorageID: nil},
		{Name: "Sol Ring", SetCode: "CMM", CollectorNumber: "123", StorageID: ptr(7)},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Main Binder", Type: "binder"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, nil, &buf))

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
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, nil, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], ",123,,1")
	assert.Contains(t, lines[2], ",123,foil,1")
}

func TestExportManaBox_PropagatesCardLoadError(t *testing.T) {
	cards := &fakeCardService{getAllErr: errors.New("db down")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportManaBox(context.Background(), testUserID, nil, &bytes.Buffer{})

	require.Error(t, err)
}

func TestExportManaBox_PropagatesStorageLoadError(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{Name: "Sol Ring", StorageID: ptr(7)}}}
	storages := &fakeStorageService{getAllErr: errors.New("db down")}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportManaBox(context.Background(), testUserID, nil, &bytes.Buffer{})

	require.Error(t, err)
}

func TestExportManaBox_EmptyCollectionProducesHeaderOnly(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, nil, &buf))

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
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, nil, &buf))

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
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, nil, &buf))

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
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, nil, &buf))

	lines := splitCSVLines(buf.String())
	require.Len(t, lines, 4) // header + 3 rows
	assert.Contains(t, lines[1], "Counterspell,clb,,1")
	assert.Contains(t, lines[2], "Counterspell,clb,,2")
	assert.Contains(t, lines[3], "1,Sol Ring,sld,foil,1011")
}

func TestExportMoxfieldCollection_PropagatesCardLoadError(t *testing.T) {
	cards := &fakeCardService{getAllErr: errors.New("db down")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportMoxfieldCollection(context.Background(), testUserID, nil, &bytes.Buffer{})

	require.Error(t, err)
}

// --- storage filter --------------------------------------------------------

func TestExportCollection_OnlyExportsTheGivenStorage(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{Name: "Sol Ring", SetCode: "SLD", ScryfallID: "aaaa", CollectorNumber: "1011", StorageID: ptr(7)},
		{Name: "Counterspell", SetCode: "CLB", ScryfallID: "bbbb", CollectorNumber: "1", StorageID: ptr(8)},
		{Name: "Lightning Bolt", SetCode: "CMM", ScryfallID: "cccc", CollectorNumber: "2"},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{
		{ID: 7, Name: "Main Binder", Type: "binder"},
		{ID: 8, Name: "Box", Type: "box"},
	}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var manabox bytes.Buffer
	require.NoError(t, svc.ExportManaBox(context.Background(), testUserID, ptr(7), &manabox))
	lines := splitCSVLines(manabox.String())
	require.Len(t, lines, 2)
	assert.Equal(t, "Main Binder,binder,Sol Ring,SLD,aaaa,1011,,1", lines[1])

	var moxfield bytes.Buffer
	require.NoError(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, ptr(8), &moxfield))
	lines = splitCSVLines(moxfield.String())
	require.Len(t, lines, 2)
	assert.Equal(t, "1,Counterspell,clb,,1", lines[1])
}

func TestExportCollection_UnknownStorageIsAnError(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	assert.ErrorIs(t, svc.ExportManaBox(context.Background(), testUserID, ptr(3), &bytes.Buffer{}), ErrTargetStorageNotFound)
	assert.ErrorIs(t, svc.ExportMoxfieldCollection(context.Background(), testUserID, ptr(3), &bytes.Buffer{}), ErrTargetStorageNotFound)
}

// --- ExportDeck ------------------------------------------------------------

func TestExportDeck_MoxfieldCommanderLineIsWrittenFirst(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Atraxa", CommanderID: ptr(100)}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {
				{ID: 100, Name: "Atraxa, Praetors' Voice", SetCode: "CMR", CollectorNumber: "1"},
				{ID: 101, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	err := svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", DeckExportMoxfield, &buf)

	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "1 Atraxa, Praetors' Voice (CMR) 1", lines[0])
	assert.Equal(t, "1 Sol Ring (SLD) 1011", lines[1])
}

func TestExportDeck_MoxfieldCommanderLineIncludesFullQuantityOfThatPrinting(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Mono Mountain", CommanderID: ptr(200)}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {
				{ID: 200, Name: "Mountain", SetCode: "WOE", CollectorNumber: "265"},
				{ID: 201, Name: "Mountain", SetCode: "WOE", CollectorNumber: "265"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", DeckExportMoxfield, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t, "2 Mountain (WOE) 265", lines[0])
}

func TestExportDeck_MoxfieldNoCommanderSortsAlphabeticallyWithNoSpecialFirstLine(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Pile"}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {
				{ID: 1, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"},
				{ID: 2, Name: "Lightning Bolt", SetCode: "CMM", CollectorNumber: "456"},
			},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", DeckExportMoxfield, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "1 Lightning Bolt (CMM) 456", lines[0])
	assert.Equal(t, "1 Sol Ring (SLD) 1011", lines[1])
}

func TestExportDeck_MoxfieldFoilSuffix(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Pile"}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {{ID: 1, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011", Foil: true}},
		},
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", DeckExportMoxfield, &buf))

	assert.Equal(t, "1 Sol Ring (SLD) 1011 *F*\n", buf.String())
}

func TestExportDeck_MoxfieldUnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999", DeckExportMoxfield, &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestExportDeck_MoxfieldPropagatesGetDeckCardsError(t *testing.T) {
	decks := &fakeDeckService{
		decks:           []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Pile"}},
		getDeckCardsErr: errors.New("db down"),
	}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", DeckExportMoxfield, &bytes.Buffer{})

	require.Error(t, err)
}

func exportDeckWith(t *testing.T, decks *fakeDeckService, format string) string {
	t.Helper()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})
	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", format, &buf))
	return buf.String()
}

func mixedDeck() *fakeDeckService {
	return &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Maeve", CommanderID: ptr(100)}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {
				{ID: 100, Name: "Maeve, Insidious Singer", SetCode: "GN3", CollectorNumber: "2", Foil: true},
				{ID: 101, Name: "Island", SetCode: "MOM", CollectorNumber: "278"},
				{ID: 102, Name: "Island", SetCode: "ONE", CollectorNumber: "263"},
				{ID: 103, Name: "Fire / Ice", SetCode: "MH2", CollectorNumber: "290"},
			},
		},
		pending: []deck.PendingCard{{ID: 7, Name: "Island", SetCode: "MOM", CollectorNumber: "278", Quantity: 30}},
	}
}

func TestExportDeck_MoxfieldIncludesPendingCards(t *testing.T) {
	out := exportDeckWith(t, mixedDeck(), DeckExportMoxfield)

	assert.Equal(t, "1 Maeve, Insidious Singer (GN3) 2 *F*\n"+
		"1 Fire / Ice (MH2) 290\n"+
		"31 Island (MOM) 278\n"+
		"1 Island (ONE) 263\n", out)
}

func TestExportDeck_PlainListGroupsPrintingsByName(t *testing.T) {
	out := exportDeckWith(t, mixedDeck(), DeckExportPlain)

	assert.Equal(t, "1 Maeve, Insidious Singer\n1 Fire / Ice\n32 Island\n", out)
}

func TestExportDeck_ArenaHasCommanderAndDeckSections(t *testing.T) {
	out := exportDeckWith(t, mixedDeck(), DeckExportArena)

	assert.Equal(t, "Commander\n1 Maeve, Insidious Singer\n\nDeck\n1 Fire // Ice\n32 Island\n", out)
}

func TestExportDeck_ArenaWithoutCommanderOnlyHasTheDeckSection(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Pile"}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {{ID: 1, Name: "Sol Ring", SetCode: "SLD", CollectorNumber: "1011"}},
		},
	}

	assert.Equal(t, "Deck\n1 Sol Ring\n", exportDeckWith(t, decks, DeckExportArena))
}

func TestExportDeck_APendingCommanderComesFirst(t *testing.T) {
	decks := &fakeDeckService{
		decks: []deck.Deck{{ID: "00000000-0000-0000-0000-000000000001", Name: "Maeve", CommanderPendingID: ptr(7)}},
		cardsByDeck: map[string][]deck.DeckCard{
			"00000000-0000-0000-0000-000000000001": {{ID: 1, Name: "Arcane Signet", SetCode: "FIC", CollectorNumber: "332"}},
		},
		pending: []deck.PendingCard{{ID: 7, Name: "Maeve, Insidious Singer", SetCode: "GN3", CollectorNumber: "2", Quantity: 1}},
	}

	assert.Equal(t, "1 Maeve, Insidious Singer\n1 Arcane Signet\n", exportDeckWith(t, decks, DeckExportPlain))
}

func TestExportDeck_RejectsAnUnknownFormat(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, mixedDeck(), &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", "mtgo", &bytes.Buffer{})

	assert.ErrorIs(t, err, ErrUnknownExportFormat)
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
