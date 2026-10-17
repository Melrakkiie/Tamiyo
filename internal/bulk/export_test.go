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

func contentDeck(d deck.Deck, entries ...deck.DeckEntry) *fakeDeckService {
	d.ID = fixtureDeckID
	return &fakeDeckService{decks: []deck.Deck{d}, entries: map[string][]deck.DeckEntry{fixtureDeckID: entries}}
}

func printingEntry(name, setCode, number string) deck.DeckEntry {
	return deck.DeckEntry{Name: name, ScryfallID: setCode + "/" + number, SetCode: setCode, CollectorNumber: number, Board: deck.BoardMain}
}

func ownedExportEntry(name, setCode, number string, ids ...int) deck.DeckEntry {
	e := printingEntry(name, setCode, number)
	e.OwnedCardIDs = ids
	return e
}

func pendingExportEntry(name, setCode, number string, id, quantity int) deck.DeckEntry {
	e := printingEntry(name, setCode, number)
	e.PendingIDs = []int{id}
	e.PendingQuantity = quantity
	return e
}

func asCommander(e deck.DeckEntry) deck.DeckEntry {
	e.Slot = deck.SlotCommander
	return e
}

func onBoard(e deck.DeckEntry, board string) deck.DeckEntry {
	e.Board = board
	return e
}

func exportDeckWith(t *testing.T, decks *fakeDeckService, format string) string {
	t.Helper()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})
	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: format}, &buf))
	return buf.String()
}

func TestExportDeck_MoxfieldCommanderLineIsWrittenFirst(t *testing.T) {
	decks := contentDeck(deck.Deck{Name: "Atraxa", CommanderID: ptr(100)},
		asCommander(ownedExportEntry("Atraxa, Praetors' Voice", "CMR", "1", 100)),
		ownedExportEntry("Sol Ring", "SLD", "1011", 101),
	)

	assert.Equal(t, "1 Atraxa, Praetors' Voice (CMR) 1\n1 Sol Ring (SLD) 1011\n", exportDeckWith(t, decks, DeckExportMoxfield))
}

func commanderWithAnIdenticalMainBoardCopy() *fakeDeckService {
	return contentDeck(deck.Deck{Name: "Mono Mountain", CommanderID: ptr(200)},
		asCommander(ownedExportEntry("Mountain", "WOE", "265", 200)),
		ownedExportEntry("Mountain", "WOE", "265", 201),
	)
}

func TestExportDeck_MoxfieldListsTheCommanderApartFromAnIdenticalMainBoardCopy(t *testing.T) {
	out := exportDeckWith(t, commanderWithAnIdenticalMainBoardCopy(), DeckExportMoxfield)

	assert.Equal(t, "1 Mountain (WOE) 265\n1 Mountain (WOE) 265\n", out)
}

func TestExportDeck_PlainListsTheCommanderApartFromAnIdenticalMainBoardCopy(t *testing.T) {
	out := exportDeckWith(t, commanderWithAnIdenticalMainBoardCopy(), DeckExportPlain)

	assert.Equal(t, "1 Mountain\n1 Mountain\n", out)
}

func TestExportDeck_ArenaListsTheCommanderApartFromAnIdenticalMainBoardCopy(t *testing.T) {
	out := exportDeckWith(t, commanderWithAnIdenticalMainBoardCopy(), DeckExportArena)

	assert.Equal(t, "Commander\n1 Mountain\n\nDeck\n1 Mountain\n", out)
}

func TestExportDeck_MoxfieldMergesProxiesWithRealCopiesOfAPrinting(t *testing.T) {
	proxy := ownedExportEntry("Sol Ring", "SLD", "1011", 2)
	proxy.Proxy = true
	decks := contentDeck(deck.Deck{Name: "Pile"}, ownedExportEntry("Sol Ring", "SLD", "1011", 1), proxy)

	assert.Equal(t, "2 Sol Ring (SLD) 1011\n", exportDeckWith(t, decks, DeckExportMoxfield))
}

func TestExportDeck_MoxfieldNoCommanderKeepsTheDeckContentOrder(t *testing.T) {
	decks := contentDeck(deck.Deck{Name: "Pile"},
		ownedExportEntry("Lightning Bolt", "CMM", "456", 2),
		ownedExportEntry("Sol Ring", "SLD", "1011", 1),
	)

	assert.Equal(t, "1 Lightning Bolt (CMM) 456\n1 Sol Ring (SLD) 1011\n", exportDeckWith(t, decks, DeckExportMoxfield))
}

func TestExportDeck_MoxfieldFoilSuffix(t *testing.T) {
	foil := ownedExportEntry("Sol Ring", "SLD", "1011", 1)
	foil.Foil = true
	decks := contentDeck(deck.Deck{Name: "Pile"}, foil)

	assert.Equal(t, "1 Sol Ring (SLD) 1011 *F*\n", exportDeckWith(t, decks, DeckExportMoxfield))
}

func TestExportDeck_MoxfieldUnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999", DeckExportOptions{Format: DeckExportMoxfield}, &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestExportDeck_MoxfieldPropagatesADeckContentError(t *testing.T) {
	decks := contentDeck(deck.Deck{Name: "Pile"})
	decks.getContentErr = errors.New("db down")
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield}, &bytes.Buffer{})

	assert.ErrorIs(t, err, decks.getContentErr)
}

func mixedIsland() deck.DeckEntry {
	e := ownedExportEntry("Island", "MOM", "278", 101)
	e.PendingIDs = []int{7}
	e.PendingQuantity = 30
	return e
}

func mixedDeck() *fakeDeckService {
	maeve := asCommander(ownedExportEntry("Maeve, Insidious Singer", "GN3", "2", 100))
	maeve.Foil = true
	return contentDeck(deck.Deck{Name: "Maeve", CommanderID: ptr(100)},
		maeve,
		ownedExportEntry("Fire / Ice", "MH2", "290", 103),
		mixedIsland(),
		ownedExportEntry("Island", "ONE", "263", 102),
	)
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
	decks := contentDeck(deck.Deck{Name: "Pile"}, ownedExportEntry("Sol Ring", "SLD", "1011", 1))

	assert.Equal(t, "Deck\n1 Sol Ring\n", exportDeckWith(t, decks, DeckExportArena))
}

func TestExportDeck_APendingCommanderExportsLikeAnOwnedOne(t *testing.T) {
	signet := ownedExportEntry("Arcane Signet", "FIC", "332", 1)
	owned := contentDeck(deck.Deck{Name: "Maeve", CommanderID: ptr(5)},
		asCommander(ownedExportEntry("Maeve, Insidious Singer", "GN3", "2", 5)),
		signet,
		ownedExportEntry("Maeve, Insidious Singer", "GN3", "2", 6, 8),
	)
	pending := contentDeck(deck.Deck{Name: "Maeve", CommanderPendingID: ptr(7)},
		asCommander(pendingExportEntry("Maeve, Insidious Singer", "GN3", "2", 7, 1)),
		signet,
		pendingExportEntry("Maeve, Insidious Singer", "GN3", "2", 7, 2),
	)

	for _, format := range []string{DeckExportMoxfield, DeckExportPlain, DeckExportArena} {
		assert.Equal(t, exportDeckWith(t, owned, format), exportDeckWith(t, pending, format), format)
	}
	assert.Equal(t, "Commander\n1 Maeve, Insidious Singer\n\nDeck\n1 Arcane Signet\n2 Maeve, Insidious Singer\n", exportDeckWith(t, pending, DeckExportArena))
}

func TestExportDeck_RejectsAnUnknownFormat(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, mixedDeck(), &fakeResolver{})

	err := svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: "mtgo"}, &bytes.Buffer{})

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

func boardedDeck() *fakeDeckService {
	decks := mixedDeck()
	duress := onBoard(ownedExportEntry("Duress", "M19", "94", 104), deck.BoardSideboard)
	duress.PendingIDs = []int{8}
	duress.PendingQuantity = 1
	decks.entries[fixtureDeckID] = append(decks.entries[fixtureDeckID],
		duress,
		onBoard(ownedExportEntry("Island", "MOM", "278", 105), deck.BoardSideboard),
		onBoard(pendingExportEntry("Fire / Ice", "MH2", "290", 9, 1), deck.BoardConsidering),
	)
	return decks
}

func TestExportDeck_MoxfieldWritesSideboardAndMaybeboardSections(t *testing.T) {
	out := exportDeckWith(t, boardedDeck(), DeckExportMoxfield)

	assert.Equal(t, "1 Maeve, Insidious Singer (GN3) 2 *F*\n"+
		"1 Fire / Ice (MH2) 290\n"+
		"31 Island (MOM) 278\n"+
		"1 Island (ONE) 263\n"+
		"\nSIDEBOARD:\n"+
		"2 Duress (M19) 94\n"+
		"1 Island (MOM) 278\n"+
		"\nMAYBEBOARD:\n"+
		"1 Fire / Ice (MH2) 290\n", out)
}

func TestExportDeck_PlainListWritesSideboardAndMaybeboardSections(t *testing.T) {
	out := exportDeckWith(t, boardedDeck(), DeckExportPlain)

	assert.Equal(t, "1 Maeve, Insidious Singer\n1 Fire / Ice\n32 Island\n\nSideboard\n2 Duress\n1 Island\n\nMaybeboard\n1 Fire / Ice\n", out)
}

func TestExportDeck_ArenaWritesTheSideboardButNotTheMaybeboard(t *testing.T) {
	out := exportDeckWith(t, boardedDeck(), DeckExportArena)

	assert.Equal(t, "Commander\n1 Maeve, Insidious Singer\n\nDeck\n1 Fire // Ice\n32 Island\n\nSideboard\n2 Duress\n1 Island\n", out)
}

func TestExportDeck_RoundTripsThroughTheImportParser(t *testing.T) {
	out := exportDeckWith(t, boardedDeck(), DeckExportMoxfield)

	lines, err := parseMoxfieldDeckList(strings.NewReader(out))

	require.NoError(t, err)
	counts := map[string]int{}
	for _, line := range lines {
		counts[line.Board] += line.Quantity
	}
	assert.Equal(t, map[string]int{deck.BoardMain: 34, deck.BoardSideboard: 3, deck.BoardConsidering: 1}, counts)
}

func TestExportSharedDeck_ExportsAsTheOwner(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.owners = map[string]string{"00000000-0000-0000-0000-000000000001": "22222222-2222-2222-2222-222222222222"}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var plain bytes.Buffer
	require.NoError(t, svc.ExportSharedDeck(context.Background(), "00000000-0000-0000-0000-000000000001", DeckExportOptions{Format: DeckExportPlain, WithTags: false}, &plain))
	assert.Contains(t, plain.String(), "1 Tamiyo, Inquisitive Student")
	assert.Contains(t, plain.String(), "5 Island")

	var tamiyo bytes.Buffer
	require.NoError(t, svc.ExportSharedDeck(context.Background(), "00000000-0000-0000-0000-000000000001", DeckExportOptions{Format: DeckExportTamiyo, WithTags: true}, &tamiyo))
	assert.Contains(t, tamiyo.String(), `"Ramp"`)
}

func TestExportSharedDeck_ListsTheCommanderApartFromAnIdenticalMainBoardCopy(t *testing.T) {
	decks := commanderWithAnIdenticalMainBoardCopy()
	decks.owners = map[string]string{fixtureDeckID: "22222222-2222-2222-2222-222222222222"}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportSharedDeck(context.Background(), fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield}, &buf))

	assert.Equal(t, "1 Mountain (WOE) 265\n1 Mountain (WOE) 265\n", buf.String())
}

func TestExportSharedDeck_Errors(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	assert.ErrorIs(t, svc.ExportSharedDeck(context.Background(), "00000000-0000-0000-0000-000000000001", DeckExportOptions{Format: DeckExportPlain, WithTags: false}, &bytes.Buffer{}), ErrDeckNotFound)
	assert.ErrorIs(t, svc.ExportSharedDeck(context.Background(), "00000000-0000-0000-0000-000000000001", DeckExportOptions{Format: "csv", WithTags: false}, &bytes.Buffer{}), ErrUnknownExportFormat)
}
