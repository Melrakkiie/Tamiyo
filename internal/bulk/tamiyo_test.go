package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const (
	solRingSLD = "aaaaaaaa-0000-0000-0000-000000000001"
	islandNEO  = "aaaaaaaa-0000-0000-0000-000000000002"
	duressM19  = "aaaaaaaa-0000-0000-0000-000000000003"
	tamiyoNEO  = "aaaaaaaa-0000-0000-0000-000000000004"
)

func tamiyoResolver() *fakeResolver {
	cards := []ResolvedCard{
		{ScryfallID: solRingSLD, Name: "Sol Ring", SetCode: "sld", CollectorNumber: "1011", ManaValue: 1, CardType: "Artifact"},
		{ScryfallID: islandNEO, Name: "Island", SetCode: "neo", CollectorNumber: "294", CardType: "Land", ColorIdentity: "U"},
		{ScryfallID: duressM19, Name: "Duress", SetCode: "m19", CollectorNumber: "94", ManaValue: 1, Colors: "B", CardType: "Sorcery", ColorIdentity: "B"},
		{ScryfallID: tamiyoNEO, Name: "Tamiyo, Inquisitive Student", SetCode: "neo", CollectorNumber: "75", ManaValue: 1, Colors: "U", CardType: "Creature", ColorIdentity: "U"},
	}
	resolved := make(map[string]ResolvedCard)
	for _, c := range cards {
		resolved[resolveKeyByID(c.ScryfallID)] = c
		resolved[resolveKey(c.SetCode, c.CollectorNumber)] = c
	}
	return &fakeResolver{resolved: resolved}
}

func collectionFixture() (*fakeCardService, *fakeStorageService) {
	binder := 3
	box := 4
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 1, Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", StorageID: &binder},
		{ID: 2, Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", StorageID: &binder},
		{ID: 3, Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", StorageID: &binder, Foil: true},
		{ID: 4, Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", StorageID: &box, Proxy: true},
		{ID: 5, Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94"},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{
		{ID: 3, Name: "Classeur bleu", Type: "binder"},
		{ID: 4, Name: "Boîte Kess", Type: "deckbox"},
	}}
	return cards, storages
}

func decodeTamiyo(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	return decoded
}

func TestExportTamiyoCollection_GroupsCopiesWithTheirStorage(t *testing.T) {
	cards, storages := collectionFixture()
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportTamiyoCollection(context.Background(), testUserID, nil, &buf))

	file, err := parseTamiyoFile(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, TamiyoKindCollection, file.Kind)
	assert.Equal(t, []tamiyoStorage{{Name: "Boîte Kess", Type: "deckbox"}, {Name: "Classeur bleu", Type: "binder"}}, file.Storages)
	kess, blue := "Boîte Kess", "Classeur bleu"
	assert.Equal(t, []tamiyoCollectionCard{
		{Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Proxy: true, Quantity: 1, Storage: &kess},
		{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Quantity: 2, Storage: &blue},
		{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Foil: true, Quantity: 1, Storage: &blue},
		{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Quantity: 1},
	}, file.collection)
	assert.EqualValues(t, 1, decodeTamiyo(t, buf.Bytes())["tamiyo"])
	assert.Contains(t, decodeTamiyo(t, buf.Bytes()), "exported_at")
}

func TestExportTamiyoCollection_OneStorage(t *testing.T) {
	cards, storages := collectionFixture()
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportTamiyoCollection(context.Background(), testUserID, ptr(3), &buf))

	file, err := parseTamiyoFile(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, TamiyoKindStorage, file.Kind)
	assert.Equal(t, []tamiyoStorage{{Name: "Classeur bleu", Type: "binder"}}, file.Storages)
	require.Len(t, file.collection, 2)
	assert.ErrorIs(t, svc.ExportTamiyoCollection(context.Background(), testUserID, ptr(99), &bytes.Buffer{}), ErrTargetStorageNotFound)
}

func TestTamiyoCollection_RoundTrip(t *testing.T) {
	cards, storages := collectionFixture()
	var buf bytes.Buffer
	require.NoError(t, NewService(cards, storages, &fakeDeckService{}, &fakeResolver{}).ExportTamiyoCollection(context.Background(), testUserID, nil, &buf))

	imported := &fakeCardService{}
	targetStorages := &fakeStorageService{storages: []storage.Storage{{ID: 1, Name: "Classeur bleu", Type: "binder"}}, nextID: 1}
	summary, err := NewService(imported, targetStorages, &fakeDeckService{}, tamiyoResolver()).ImportTamiyoCollection(context.Background(), testUserID, nil, &buf)

	require.NoError(t, err)
	assert.Equal(t, 5, summary.CardsCreated)
	assert.Equal(t, 1, summary.StoragesCreated)
	require.Len(t, targetStorages.storages, 2)
	assert.Equal(t, storage.Storage{ID: 2, Name: "Boîte Kess", Type: "deckbox"}, targetStorages.storages[1])

	byName := map[string][]card.Card{}
	for _, c := range imported.created {
		byName[c.Name] = append(byName[c.Name], c)
	}
	require.Len(t, byName["Sol Ring"], 3)
	assert.Equal(t, 1, *byName["Sol Ring"][0].StorageID)
	assert.True(t, byName["Sol Ring"][2].Foil)
	assert.Equal(t, 1.0, byName["Sol Ring"][0].ManaValue)
	require.Len(t, byName["Island"], 1)
	assert.True(t, byName["Island"][0].Proxy)
	assert.Equal(t, 2, *byName["Island"][0].StorageID)
	assert.Equal(t, "U", *byName["Island"][0].ColorIdentity)
	require.Len(t, byName["Duress"], 1)
	assert.Nil(t, byName["Duress"][0].StorageID)
}

func TestImportTamiyoCollection_IntoOneStorage(t *testing.T) {
	cards, storages := collectionFixture()
	var buf bytes.Buffer
	require.NoError(t, NewService(cards, storages, &fakeDeckService{}, &fakeResolver{}).ExportTamiyoCollection(context.Background(), testUserID, nil, &buf))

	imported := &fakeCardService{}
	target := &fakeStorageService{storages: []storage.Storage{{ID: 9, Name: "Vrac", Type: "box"}}}
	summary, err := NewService(imported, target, &fakeDeckService{}, tamiyoResolver()).ImportTamiyoCollection(context.Background(), testUserID, ptr(9), &buf)

	require.NoError(t, err)
	assert.Equal(t, 5, summary.CardsCreated)
	assert.Zero(t, summary.StoragesCreated)
	for _, c := range imported.created {
		assert.Equal(t, 9, *c.StorageID)
	}
}

func TestImportTamiyoCollection_SkipsUnknownCards(t *testing.T) {
	file := `{"tamiyo": 1, "kind": "collection", "cards": [
		{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "set_code": "sld", "collector_number": "1011", "quantity": 2},
		{"name": "Mystery", "scryfall_id": "bbbbbbbb-0000-0000-0000-000000000000", "set_code": "zzz", "collector_number": "1", "quantity": 3}
	]}`
	imported := &fakeCardService{}

	summary, err := NewService(imported, &fakeStorageService{}, &fakeDeckService{}, tamiyoResolver()).ImportTamiyoCollection(context.Background(), testUserID, nil, strings.NewReader(file))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 3, summary.CardsSkipped)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], "Mystery")
}

func TestImportTamiyoCollection_RejectsInvalidFiles(t *testing.T) {
	cases := map[string]string{
		"not json":       "4 Lightning Bolt",
		"no version":     `{"kind": "collection", "cards": []}`,
		"newer version":  `{"tamiyo": 2, "kind": "collection", "cards": []}`,
		"unknown kind":   `{"tamiyo": 1, "kind": "binder", "cards": []}`,
		"no scryfall id": `{"tamiyo": 1, "kind": "collection", "cards": [{"name": "Sol Ring", "quantity": 1}]}`,
		"zero quantity":  `{"tamiyo": 1, "kind": "collection", "cards": [{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "quantity": 0}]}`,
		"huge quantity":  `{"tamiyo": 1, "kind": "collection", "cards": [{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "quantity": 1001}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			imported := &fakeCardService{}
			_, err := NewService(imported, &fakeStorageService{}, &fakeDeckService{}, tamiyoResolver()).ImportTamiyoCollection(context.Background(), testUserID, nil, strings.NewReader(content))
			assert.ErrorIs(t, err, ErrInvalidFile)
			assert.Empty(t, imported.created)
		})
	}
}

func TestImportTamiyoCollection_RefusesADeckFile(t *testing.T) {
	_, err := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, tamiyoResolver()).ImportTamiyoCollection(context.Background(), testUserID, nil, strings.NewReader(`{"tamiyo": 1, "kind": "deck", "cards": []}`))

	assert.ErrorIs(t, err, ErrTamiyoDeckFile)
}

func tamiyoDeckFixture() *fakeDeckService {
	deckID := "00000000-0000-0000-0000-000000000001"
	return &fakeDeckService{
		decks: []deck.Deck{{ID: deckID, Name: "Tamiyo tempo", Format: "commander", CommanderPendingID: ptr(7)}},
		cardsByDeck: map[string][]deck.DeckCard{deckID: {
			{ID: 1, Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Board: deck.BoardMain},
			{ID: 2, Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Board: deck.BoardMain},
			{ID: 3, Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Board: deck.BoardMain},
			{ID: 4, Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Board: deck.BoardSideboard},
		}},
		pending: []deck.PendingCard{
			{ID: 7, Name: "Tamiyo, Inquisitive Student", ScryfallID: tamiyoNEO, SetCode: "neo", CollectorNumber: "75", Quantity: 1, Board: deck.BoardMain},
			{ID: 8, Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Quantity: 3, Board: deck.BoardMain},
			{ID: 9, Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Quantity: 1, Foil: true, Board: deck.BoardConsidering},
		},
		entries: map[string][]deck.DeckEntry{deckID: {
			{Name: "Tamiyo, Inquisitive Student", ScryfallID: tamiyoNEO, SetCode: "neo", CollectorNumber: "75", Board: deck.BoardMain, Slot: deck.SlotCommander, Pending: []deck.PendingPart{{ID: 7, Quantity: 1}}},
			{Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Board: deck.BoardMain, OwnedCardIDs: []int{2, 3}, Pending: []deck.PendingPart{{ID: 8, Quantity: 3}}},
			{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Board: deck.BoardMain, OwnedCardIDs: []int{1}},
			{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Board: deck.BoardSideboard, OwnedCardIDs: []int{4}},
			{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Foil: true, Board: deck.BoardConsidering, Pending: []deck.PendingPart{{ID: 9, Quantity: 1}}},
		}},
		tags: deck.DeckTags{Tags: []string{"Pioche", "Ramp"}, Cards: []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"Ramp"}}}},
	}
}

func TestExportTamiyoDeck_WritesBoardsCommanderAndTags(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportTamiyoDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", true, &buf))

	file, err := parseTamiyoFile(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, TamiyoKindDeck, file.Kind)
	assert.Equal(t, &tamiyoDeckInfo{Name: "Tamiyo tempo", Format: "commander"}, file.Deck)
	assert.Equal(t, []tamiyoDeckCard{
		{Name: "Tamiyo, Inquisitive Student", ScryfallID: tamiyoNEO, SetCode: "neo", CollectorNumber: "75", Quantity: 1, Board: deck.BoardMain, Commander: true},
		{Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Quantity: 5, Board: deck.BoardMain},
		{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Quantity: 1, Board: deck.BoardMain},
		{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Quantity: 1, Board: deck.BoardSideboard},
		{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Foil: true, Quantity: 1, Board: deck.BoardConsidering},
	}, file.deckCards)
	assert.Equal(t, []tamiyoCardTags{{Name: "Sol Ring", Tags: []string{"Ramp"}}}, file.Tags)
}

func proxyDeckFixture() *fakeDeckService {
	return &fakeDeckService{
		decks: []deck.Deck{{ID: fixtureDeckID, Name: "Proxies", Format: "commander"}},
		entries: map[string][]deck.DeckEntry{fixtureDeckID: {
			{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Board: deck.BoardMain, OwnedCardIDs: []int{1}},
			{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Proxy: true, Board: deck.BoardMain, OwnedCardIDs: []int{2}, Pending: []deck.PendingPart{{ID: 8, Quantity: 1}}},
			{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Proxy: true, Board: deck.BoardSideboard, Pending: []deck.PendingPart{{ID: 9, Quantity: 2}}},
		}},
	}
}

func TestExportTamiyoDeck_WritesEachLinesProxyState(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, proxyDeckFixture(), &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportTamiyoDeck(context.Background(), testUserID, fixtureDeckID, false, &buf))

	file, err := parseTamiyoFile(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, []tamiyoDeckCard{
		{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Quantity: 1, Board: deck.BoardMain},
		{Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Proxy: true, Quantity: 2, Board: deck.BoardMain},
		{Name: "Duress", ScryfallID: duressM19, SetCode: "m19", CollectorNumber: "94", Proxy: true, Quantity: 2, Board: deck.BoardSideboard},
	}, file.deckCards)
	assert.Contains(t, buf.String(), `"proxy": false`)
}

func TestExportTamiyoDeck_WithoutTags(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportTamiyoDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", false, &buf))

	assert.NotContains(t, decodeTamiyo(t, buf.Bytes()), "tags")
}

func TestExportTamiyoDeck_ReadsTheViewersOwnDeck(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	require.NoError(t, svc.ExportTamiyoDeck(context.Background(), testUserID, fixtureDeckID, true, &bytes.Buffer{}))

	assert.Equal(t, []deckRead{{viewerID: testUserID, deckID: fixtureDeckID, reach: deck.ReachOwn}}, decks.reads)
}

func TestExportTamiyoDeck_UnknownDeckIsNotFound(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.readErr = deck.ErrNotFound
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	assert.ErrorIs(t, svc.ExportTamiyoDeck(context.Background(), testUserID, fixtureDeckID, false, &bytes.Buffer{}), deck.ErrNotFound)
}

func TestImportIntoDeck_ReadsATamiyoDeckFile(t *testing.T) {
	var exported bytes.Buffer
	require.NoError(t, NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{}).ExportTamiyoDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", true, &exported))
	decks := withTargetDeck(&fakeDeckService{placeSummary: &deck.PlacementSummary{CardsLinked: 2, CardsPending: 7}})
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, &exported)

	require.NoError(t, err)
	assert.Equal(t, Summary{CardsLinked: 2, CardsPending: 7}, summary)
	plan := decks.placedPlan(t, targetDeckID)
	assert.Equal(t, []placedLine{
		{name: "Tamiyo, Inquisitive Student", setCode: "neo", quantity: 1, board: deck.BoardMain, commander: true, proxy: deck.RealOnly},
		{name: "Island", setCode: "neo", quantity: 5, board: deck.BoardMain, proxy: deck.RealOnly},
		{name: "Sol Ring", setCode: "sld", quantity: 1, board: deck.BoardMain, proxy: deck.RealOnly},
		{name: "Duress", setCode: "m19", quantity: 1, board: deck.BoardSideboard, proxy: deck.RealOnly},
		{name: "Duress", setCode: "m19", quantity: 1, board: deck.BoardConsidering, foil: true, proxy: deck.RealOnly},
	}, placedLines(plan))
	assert.Equal(t, []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"Ramp"}}}, plan.Tags)
}

func TestImportIntoDeck_RefusesATamiyoCollectionFile(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, withTargetDeck(&fakeDeckService{}), tamiyoResolver())

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader(`{"tamiyo": 1, "kind": "storage", "cards": []}`))
	assert.ErrorIs(t, err, ErrTamiyoCollectionFile)

	_, err = svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader(`{"tamiyo": 1, "kind": "deck", "cards": [{"name": "Sol Ring", "scryfall_id": "`+solRingSLD+`", "quantity": 1, "board": "graveyard"}]}`))
	assert.ErrorIs(t, err, ErrInvalidFile)
}

func TestTamiyoDeck_RoundTripKeepsEachLinesProxyState(t *testing.T) {
	var exported bytes.Buffer
	require.NoError(t, NewService(&fakeCardService{}, &fakeStorageService{}, proxyDeckFixture(), &fakeResolver{}).ExportTamiyoDeck(context.Background(), testUserID, fixtureDeckID, false, &exported))
	decks := withTargetDeck(&fakeDeckService{})

	_, err := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver()).ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, &exported)

	require.NoError(t, err)
	assert.Equal(t, []placedLine{
		{name: "Sol Ring", setCode: "sld", quantity: 1, board: deck.BoardMain, proxy: deck.RealOnly},
		{name: "Sol Ring", setCode: "sld", quantity: 2, board: deck.BoardMain, proxy: deck.ProxyOnly},
		{name: "Duress", setCode: "m19", quantity: 2, board: deck.BoardSideboard, proxy: deck.ProxyOnly},
	}, placedLines(decks.placedPlan(t, targetDeckID)))
}

func TestImportIntoDeck_AnOlderTamiyoFileWithoutProxyImportsRealCards(t *testing.T) {
	file := `{"tamiyo": 1, "kind": "deck", "cards": [{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "set_code": "sld", "collector_number": "1011", "quantity": 2, "board": "main"}]}`
	decks := withTargetDeck(&fakeDeckService{})

	_, err := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver()).ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader(file))

	require.NoError(t, err)
	assert.Equal(t, deck.RealOnly, decks.placedPlan(t, targetDeckID).Lines[0].Proxy)
}
