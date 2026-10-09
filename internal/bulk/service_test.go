package bulk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

// --- fakes -------------------------------------------------------------

type fakeCardService struct {
	nextID    int
	created   []card.Card
	createErr error

	allCards  []card.Card
	getAllErr error

	missingDetails     []card.Card
	missingCountAfter  map[int]int
	lastMissingAfterID int
	lastMissingLimit   int
	setDetails         map[int]card.Details
}

func (f *fakeCardService) GetCardsMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]card.Card, error) {
	f.lastMissingAfterID = afterID
	f.lastMissingLimit = limit
	return f.missingDetails, nil
}

func (f *fakeCardService) CountCardsMissingDetails(ctx context.Context, userID string, afterID int) (int, error) {
	return f.missingCountAfter[afterID], nil
}

func (f *fakeCardService) SetCardDetails(ctx context.Context, userID string, id int, details card.Details) error {
	if f.setDetails == nil {
		f.setDetails = map[int]card.Details{}
	}
	f.setDetails[id] = details
	return nil
}

func (f *fakeCardService) CreateCard(ctx context.Context, userID string, c card.Card) (card.Card, error) {
	if f.createErr != nil {
		return card.Card{}, f.createErr
	}
	f.nextID++
	c.ID = f.nextID
	f.created = append(f.created, c)
	return c, nil
}

func (f *fakeCardService) GetAllCards(ctx context.Context, userID string, filter card.CardFilter) ([]card.Card, int, error) {
	if f.getAllErr != nil {
		return nil, 0, f.getAllErr
	}
	matching := f.allCards
	if filter.StorageID != nil {
		matching = nil
		for _, c := range f.allCards {
			if c.StorageID != nil && *c.StorageID == *filter.StorageID {
				matching = append(matching, c)
			}
		}
	}
	total := len(matching)
	limit := filter.Limit
	if limit <= 0 {
		limit = total
	}
	start := (filter.Page - 1) * limit
	if start < 0 || start >= total {
		return nil, total, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	return matching[start:end], total, nil
}

type fakeStorageService struct {
	storages   []storage.Storage
	nextID     int
	created    []storage.Storage
	getByIDErr error
	getAllErr  error
}

func (f *fakeStorageService) GetAllStorages(ctx context.Context, userID string, filter storage.Filter) ([]storage.Storage, int, error) {
	if f.getAllErr != nil {
		return nil, 0, f.getAllErr
	}
	return f.storages, len(f.storages), nil
}

func (f *fakeStorageService) GetStorage(ctx context.Context, userID string, id int) (storage.Storage, error) {
	if f.getByIDErr != nil {
		return storage.Storage{}, f.getByIDErr
	}
	for _, s := range f.storages {
		if s.ID == id {
			return s, nil
		}
	}
	return storage.Storage{}, storage.ErrNotFound
}

func (f *fakeStorageService) CreateStorage(ctx context.Context, userID string, s storage.Storage) (storage.Storage, error) {
	f.nextID++
	s.ID = f.nextID
	f.storages = append(f.storages, s)
	f.created = append(f.created, s)
	return s, nil
}

type fakeDeckService struct {
	tags       deck.DeckTags
	setTags    map[string][]string
	setTagsErr error

	decks       []deck.Deck
	nextID      int
	created     []deck.Deck
	linkedCards map[string][]int
	linkErr     error

	cardsByDeck     map[string][]deck.DeckCard
	getDeckErr      error
	getDeckCardsErr error
	createDeckErr   error

	pending        []deck.PendingCard
	removedPending []int
	promoted       map[int]int

	addedPending       []deck.PendingCard
	addPendingErr      error
	pendingCommanderID int
	cardCommanderID    int
	pendingQuantities  map[int]int
	linkedBoards       map[int]string

	owners      map[string]string
	ownedCopies map[string]int
	deleted     []string
}

func (f *fakeDeckService) DeleteDeck(ctx context.Context, userID string, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeDeckService) GetSharedDeck(ctx context.Context, id string) (string, deck.Deck, error) {
	for _, d := range f.decks {
		if d.ID == id && f.owners[id] != "" {
			return f.owners[id], d, nil
		}
	}
	return "", deck.Deck{}, deck.ErrNotFound
}

func (f *fakeDeckService) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	counts := make(map[string]int, len(nameKeys))
	for _, key := range nameKeys {
		if n := f.ownedCopies[key]; n > 0 {
			counts[key] = n
		}
	}
	return counts, nil
}

func (f *fakeDeckService) SetPendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (deck.PendingCard, error) {
	if f.pendingQuantities == nil {
		f.pendingQuantities = map[int]int{}
	}
	f.pendingQuantities[id] = quantity
	return deck.PendingCard{ID: id, DeckID: deckID, Quantity: quantity}, nil
}

func (f *fakeDeckService) GetCardTags(ctx context.Context, userID string, deckID string) (deck.DeckTags, error) {
	return f.tags, nil
}

func (f *fakeDeckService) SetCardTags(ctx context.Context, userID string, deckID string, cardName string, tags []string) (deck.TaggedCard, error) {
	if f.setTagsErr != nil {
		return deck.TaggedCard{}, f.setTagsErr
	}
	if f.setTags == nil {
		f.setTags = map[string][]string{}
	}
	f.setTags[cardName] = tags
	return deck.TaggedCard{Name: cardName, Tags: tags}, nil
}

func (f *fakeDeckService) SetCardCommander(ctx context.Context, userID string, deckID string, cardID int) error {
	f.cardCommanderID = cardID
	return nil
}

func (f *fakeDeckService) AddPendingCard(ctx context.Context, userID string, deckID string, p deck.PendingCard) (deck.PendingCard, error) {
	if f.addPendingErr != nil {
		return deck.PendingCard{}, f.addPendingErr
	}
	p.ID = len(f.addedPending) + 100
	p.DeckID = deckID
	f.addedPending = append(f.addedPending, p)
	return p, nil
}

func (f *fakeDeckService) SetPendingCommander(ctx context.Context, userID string, deckID string, pendingID int) error {
	f.pendingCommanderID = pendingID
	return nil
}

func (f *fakeDeckService) PromotePendingCommander(ctx context.Context, userID string, deckID string, pendingID, cardID int) error {
	if f.promoted == nil {
		f.promoted = map[int]int{}
	}
	f.promoted[pendingID] = cardID
	return nil
}

func (f *fakeDeckService) GetPendingCards(ctx context.Context, userID string, deckID string) ([]deck.PendingCard, error) {
	if f.getDeckErr != nil {
		return nil, f.getDeckErr
	}
	return f.pending, nil
}

func (f *fakeDeckService) RemovePendingCard(ctx context.Context, userID string, deckID string, id int) error {
	f.removedPending = append(f.removedPending, id)
	return nil
}

func (f *fakeDeckService) GetAllDecks(ctx context.Context, userID string, filter deck.Filter) ([]deck.Deck, int, error) {
	mine := make([]deck.Deck, 0, len(f.decks))
	for _, d := range f.decks {
		if owner := f.owners[d.ID]; owner == "" || owner == userID {
			mine = append(mine, d)
		}
	}
	return mine, len(mine), nil
}

func (f *fakeDeckService) GetDeck(ctx context.Context, userID string, id string) (deck.Deck, error) {
	if f.getDeckErr != nil {
		return deck.Deck{}, f.getDeckErr
	}
	if owner := f.owners[id]; owner != "" && owner != userID {
		return deck.Deck{}, deck.ErrNotFound
	}
	for _, d := range f.decks {
		if d.ID == id {
			return d, nil
		}
	}
	return deck.Deck{}, deck.ErrNotFound
}

func (f *fakeDeckService) GetDeckCards(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]deck.DeckCard, error) {
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.cardsByDeck[id], nil
}

func (f *fakeDeckService) CreateDeck(ctx context.Context, userID string, d deck.Deck) (deck.Deck, error) {
	if f.createDeckErr != nil {
		return deck.Deck{}, f.createDeckErr
	}
	f.nextID++
	d.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	f.decks = append(f.decks, d)
	f.created = append(f.created, d)
	return d, nil
}

func (f *fakeDeckService) PutCardInDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error {
	if f.linkErr != nil {
		return f.linkErr
	}
	if f.linkedCards == nil {
		f.linkedCards = make(map[string][]int)
	}
	if f.linkedBoards == nil {
		f.linkedBoards = make(map[int]string)
	}
	f.linkedBoards[cardID] = board
	f.linkedCards[deckID] = append(f.linkedCards[deckID], cardID)
	return nil
}

type fakeResolver struct {
	resolved map[string]ResolvedCard
	err      error
	setNames map[string]string
}

func (f *fakeResolver) SetNames(ctx context.Context) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.setNames, nil
}

func (f *fakeResolver) Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]ResolvedCard, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]ResolvedCard)
	for _, id := range identifiers {
		key := identifierKey(id)
		if v, ok := f.resolved[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

// --- ImportManaBox -------------------------------------------------------

func TestImportManaBox_CreatesStorageAndCards(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,2
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 1, summary.StoragesCreated)
	assert.Equal(t, 0, summary.DecksCreated)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
	require.NotNil(t, cards.created[0].StorageID)
	assert.Equal(t, storages.created[0].ID, *cards.created[0].StorageID)
}

func TestImportManaBox_IntoAStoragePutsEveryCardThereAndCreatesNothingElse(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,2
Atraxa Deck,deck,Counterspell,CLB,bbbbbbbb-0000-0000-0000-000000000000,1,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}}}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, ptr(4), strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsCreated)
	assert.Equal(t, 0, summary.StoragesCreated)
	assert.Equal(t, 0, summary.DecksCreated)
	assert.Empty(t, storages.created)
	assert.Empty(t, decks.created)
	assert.Empty(t, decks.linkedCards)
	require.Len(t, cards.created, 3)
	for _, c := range cards.created {
		require.NotNil(t, c.StorageID)
		assert.Equal(t, 4, *c.StorageID)
	}
}

func TestImportManaBox_IntoAnUnknownStorageFails(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.ImportManaBox(context.Background(), testUserID, ptr(4), strings.NewReader(csv))

	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	assert.Empty(t, cards.created)
}

func TestImportManaBox_ResolvesManaValueFromScryfallByID(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByID("aaaaaaaa-0000-0000-0000-000000000000"): {ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", ManaValue: 1},
	}}
	svc := NewService(cards, storages, decks, resolver)

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	assert.Equal(t, 1.0, cards.created[0].ManaValue)
}

func TestImportManaBox_DefaultsManaValueToZeroWhenScryfallUnavailable(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{err: errors.New("scryfall unreachable")}
	svc := NewService(cards, storages, decks, resolver)

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err, "a scryfall outage must not break a manabox import, which never depended on it before")
	require.Len(t, cards.created, 1)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Equal(t, 0.0, cards.created[0].ManaValue)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], "mana value")
}

func TestImportManaBox_ReusesExistingStorageByName(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 42, Name: "Main Binder", Type: "binder"}}}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.StoragesCreated)
	assert.Equal(t, 42, *cards.created[0].StorageID)
}

func TestImportManaBox_CreatesDeckAndLinksCards(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Atraxa Deck,deck,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.DecksCreated)
	require.Len(t, decks.created, 1)
	assert.Equal(t, "commander", decks.created[0].Format)
	assert.Equal(t, []int{cards.created[0].ID}, decks.linkedCards[decks.created[0].ID])
	require.Len(t, storages.created, 1)
	assert.Equal(t, "deckbox", storages.created[0].Type)
}

func TestImportManaBox_ReusesSameDeckAcrossMultipleRows(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Atraxa Deck,deck,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
Atraxa Deck,deck,Lightning Bolt,CMM,bbbbbbbb-0000-0000-0000-000000000000,456,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.DecksCreated)
	require.Len(t, decks.created, 1)
	assert.ElementsMatch(t, []int{cards.created[0].ID, cards.created[1].ID}, decks.linkedCards[decks.created[0].ID])
}

func TestImportManaBox_ReturnsErrorWhenDeckCreationFails(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Atraxa Deck,deck,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{createDeckErr: errors.New("db is down")}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.Error(t, err)
}

func TestImportManaBox_SkipsCardOnCreateErrorButContinues(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
Main Binder,binder,Lightning Bolt,CMM,bbbbbbbb-0000-0000-0000-000000000000,456,,1
`
	cards := &fakeCardService{createErr: errors.New("boom")}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 2, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 2)
}

func TestImportManaBox_InvalidFileReturnsErrInvalidFile(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader("not,a,valid,header\n"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
}

// --- ImportMoxfieldCollection --------------------------------------------

func TestImportMoxfieldCollection_ResolvesAndCreatesCards(t *testing.T) {
	csv := `Count,Name,Edition,Foil,Collector Number
2,Sol Ring,sld,foil,1011
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Binder", Type: "binder"}}}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{resolveKey("sld", "1011"): {ScryfallID: "cccccccc-0000-0000-0000-000000000000", ManaValue: 1}}}
	svc := NewService(cards, storages, &fakeDeckService{}, resolver)

	summary, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 0, summary.CardsSkipped)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "cccccccc-0000-0000-0000-000000000000", cards.created[0].ScryfallID)
	assert.Equal(t, 7, *cards.created[0].StorageID)
	assert.Equal(t, 1.0, cards.created[0].ManaValue)
}

func TestImportMoxfieldCollection_SkipsUnresolvedCards(t *testing.T) {
	csv := `Count,Name,Edition,Foil,Collector Number
1,Mystery Card,xxx,,999
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7}}}
	svc := NewService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	summary, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 1)
}

func TestImportMoxfieldCollection_UnknownStorageReturnsErrTargetStorageNotFound(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 99, strings.NewReader("Count,Name,Edition,Foil,Collector Number\n"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
}

func TestImportMoxfieldCollection_ScryfallFailureReturnsErrScryfallUnavailable(t *testing.T) {
	csv := `Count,Name,Edition,Foil,Collector Number
1,Sol Ring,sld,,1011
`
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7}}}
	resolver := &fakeResolver{err: errors.New("network down")}
	svc := NewService(&fakeCardService{}, storages, &fakeDeckService{}, resolver)

	_, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

// --- ImportIntoDeck --------------------------------------------------------

const (
	atraxaID  = "11111111-0000-0000-0000-000000000000"
	solRingID = "22222222-0000-0000-0000-000000000000"
)

func deckImportResolver() *fakeResolver {
	return &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKey("CMR", "1"):    {ScryfallID: atraxaID, ManaValue: 4, Colors: "WUBG", CardType: "Creature", ColorIdentity: "WUBG"},
		resolveKey("SLD", "1011"): {ScryfallID: solRingID, ManaValue: 1, CardType: "Artifact"},
	}}
}

const targetDeckID = "00000000-0000-0000-0000-000000000077"

func withTargetDeck(decks *fakeDeckService) *fakeDeckService {
	for _, d := range decks.decks {
		if d.ID == targetDeckID {
			return decks
		}
	}
	decks.decks = append(decks.decks, deck.Deck{ID: targetDeckID, Name: "Target"})
	return decks
}

func importDeck(t *testing.T, cards *fakeCardService, decks *fakeDeckService, decklist string, commanderFromFirstLine bool) Summary {
	t.Helper()
	svc := NewService(cards, &fakeStorageService{}, withTargetDeck(decks), deckImportResolver())
	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, commanderFromFirstLine, strings.NewReader(decklist))
	require.NoError(t, err)
	return summary
}

func TestImportIntoDeck_NeverCreatesCardsOrDecks(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n2 Sol Ring (SLD) 1011\n", true)

	assert.Empty(t, cards.created)
	assert.Empty(t, decks.created)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 0, summary.DecksCreated)
}

func TestImportIntoDeck_PutsOwnedCopiesInTheDeck(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID},
		{ID: 6, Name: "Sol Ring", ScryfallID: solRingID},
		{ID: 7, Name: "Sol Ring", ScryfallID: solRingID},
	}}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n2 Sol Ring (SLD) 1011\n", true)

	assert.Equal(t, 3, summary.CardsLinked)
	assert.Equal(t, 0, summary.CardsPending)
	assert.Equal(t, 5, decks.cardCommanderID)
	assert.ElementsMatch(t, []int{5, 6, 7}, decks.linkedCards[targetDeckID])
	assert.Empty(t, decks.addedPending)
}

func TestImportIntoDeck_AddsMissingCopiesAsPending(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "3 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 2, summary.CardsPending)
	assert.Equal(t, []int{6}, decks.linkedCards[targetDeckID])
	require.Len(t, decks.addedPending, 1)
	pending := decks.addedPending[0]
	assert.Equal(t, targetDeckID, pending.DeckID)
	assert.Equal(t, "Sol Ring", pending.Name)
	assert.Equal(t, solRingID, pending.ScryfallID)
	assert.Equal(t, "SLD", pending.SetCode)
	assert.Equal(t, "1011", pending.CollectorNumber)
	assert.Equal(t, 2, pending.Quantity)
	assert.Equal(t, 1.0, pending.ManaValue)
	require.NotNil(t, pending.CardType)
	assert.Equal(t, "Artifact", *pending.CardType)
}

func TestImportIntoDeck_OnlyUsesTheExactPrinting(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: "33333333-0000-0000-0000-000000000000"}}}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 0, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Empty(t, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_PrefersCopiesInNoDeckThenTheSameFinish(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 1, Name: "Sol Ring", ScryfallID: solRingID, Foil: false},
		{ID: 2, Name: "Sol Ring", ScryfallID: solRingID, Foil: true},
		{ID: 3, Name: "Sol Ring", ScryfallID: solRingID, Foil: true},
	}}
	decks := &fakeDeckService{
		decks:       []deck.Deck{{ID: "00000000-0000-0000-0000-000000000099", Name: "Other"}},
		cardsByDeck: map[string][]deck.DeckCard{"00000000-0000-0000-0000-000000000099": {{ID: 2}}},
	}

	importDeck(t, cards, decks, "2 Sol Ring (SLD) 1011 *F*\n", false)

	assert.Equal(t, []int{3, 1}, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_ReusesACopyAlreadyInAnotherDeck(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 2, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{
		decks:       []deck.Deck{{ID: "00000000-0000-0000-0000-000000000099", Name: "Other"}},
		cardsByDeck: map[string][]deck.DeckCard{"00000000-0000-0000-0000-000000000099": {{ID: 2}}},
	}

	summary := importDeck(t, cards, decks, "1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, []int{2}, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_UsesEachOwnedCopyOnce(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Sol Ring (SLD) 1011\n1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
}

func TestImportIntoDeck_MakesAPendingCommanderWhenItIsNotOwned(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n1 Sol Ring (SLD) 1011\n", true)

	assert.Equal(t, 2, summary.CardsPending)
	assert.Zero(t, decks.cardCommanderID)
	require.Len(t, decks.addedPending, 2)
	assert.Equal(t, "Atraxa, Praetors' Voice", decks.addedPending[0].Name)
	assert.Equal(t, decks.addedPending[0].ID, decks.pendingCommanderID)
}

func TestImportIntoDeck_WithoutCommanderFlag(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID}}}
	decks := &fakeDeckService{}

	importDeck(t, cards, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n", false)

	assert.Zero(t, decks.cardCommanderID)
	assert.Zero(t, decks.pendingCommanderID)
}

func TestImportIntoDeck_UnresolvedLinesAreSkippedButTheRestIsImported(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "1 Mystery Commander (XXX) 999\n1 Sol Ring (SLD) 1011\n", true)

	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Zero(t, decks.cardCommanderID)
	assert.Zero(t, decks.pendingCommanderID)
	assert.NotEmpty(t, summary.Warnings)
}

func plainListResolver() *fakeResolver {
	return &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByName("Sol Ring"):    {ScryfallID: solRingID, Name: "Sol Ring", SetCode: "cmm", CollectorNumber: "464", ManaValue: 1, CardType: "Artifact"},
		resolveKeyByName("Fire // Ice"): {ScryfallID: "44444444-0000-0000-0000-000000000000", Name: "Fire // Ice", SetCode: "mh2", CollectorNumber: "290", ManaValue: 4},
	}}
}

func importPlainList(t *testing.T, cards *fakeCardService, decks *fakeDeckService, decklist string) (Summary, *fakeResolver) {
	t.Helper()
	resolver := plainListResolver()
	svc := NewService(cards, &fakeStorageService{}, withTargetDeck(decks), resolver)
	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, false, strings.NewReader(decklist))
	require.NoError(t, err)
	return summary, resolver
}

func TestImportIntoDeck_PlainListUsesAnyOwnedPrinting(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 6, Name: "Sol Ring", ScryfallID: "33333333-0000-0000-0000-000000000000"},
		{ID: 7, Name: "Sol Ring", ScryfallID: solRingID},
	}}
	decks := &fakeDeckService{}

	summary, _ := importPlainList(t, cards, decks, "3 Sol Ring\n")

	assert.Equal(t, 2, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.ElementsMatch(t, []int{6, 7}, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_PlainListAddsTheScryfallPrintingAsPending(t *testing.T) {
	decks := &fakeDeckService{}

	summary, _ := importPlainList(t, &fakeCardService{}, decks, "1 Sol Ring\n")

	assert.Equal(t, 1, summary.CardsPending)
	require.Len(t, decks.addedPending, 1)
	pending := decks.addedPending[0]
	assert.Equal(t, "Sol Ring", pending.Name)
	assert.Equal(t, solRingID, pending.ScryfallID)
	assert.Equal(t, "cmm", pending.SetCode)
	assert.Equal(t, "464", pending.CollectorNumber)
	assert.Equal(t, 1.0, pending.ManaValue)
}

func TestImportIntoDeck_PlainListMatchesSplitCardsWithEitherSeparator(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 8, Name: "Fire // Ice", ScryfallID: "55555555-0000-0000-0000-000000000000"}}}
	decks := &fakeDeckService{}

	summary, _ := importPlainList(t, cards, decks, "2 Fire / Ice\n")

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Equal(t, "Fire // Ice", decks.addedPending[0].Name)
}

func TestImportIntoDeck_PlainListWarnsAboutUnknownNames(t *testing.T) {
	summary, _ := importPlainList(t, &fakeCardService{}, &fakeDeckService{}, "1 Not A Real Card\n1 Sol Ring\n")

	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Equal(t, 1, summary.CardsPending)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], "Not A Real Card")
}

func importIntoDeck(t *testing.T, cards *fakeCardService, decks *fakeDeckService, decklist string, commanderFromFirstLine bool) Summary {
	t.Helper()
	svc := NewService(cards, &fakeStorageService{}, decks, deckImportResolver())
	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, commanderFromFirstLine, strings.NewReader(decklist))
	require.NoError(t, err)
	return summary
}

func TestImportIntoDeck_AddsCardsToTheExistingDeck(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{decks: []deck.Deck{{ID: targetDeckID, Name: "Target"}}}

	summary := importIntoDeck(t, cards, decks, "2 Sol Ring (SLD) 1011\n", false)

	assert.Empty(t, decks.created)
	assert.Equal(t, 0, summary.DecksCreated)
	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Equal(t, []int{6}, decks.linkedCards[targetDeckID])
	assert.Equal(t, targetDeckID, decks.addedPending[0].DeckID)
}

func TestImportIntoDeck_DoesNotReuseCopiesAlreadyInTheDeck(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 6, Name: "Sol Ring", ScryfallID: solRingID},
		{ID: 7, Name: "Sol Ring", ScryfallID: solRingID},
	}}
	decks := &fakeDeckService{
		decks:       []deck.Deck{{ID: targetDeckID, Name: "Target"}},
		cardsByDeck: map[string][]deck.DeckCard{targetDeckID: {{ID: 6}}},
	}

	summary := importIntoDeck(t, cards, decks, "2 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Equal(t, []int{7}, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_SetsTheCommanderOnlyWhenTheDeckHasNone(t *testing.T) {
	owned := []card.Card{{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID}}

	decks := &fakeDeckService{decks: []deck.Deck{{ID: targetDeckID, Name: "Target"}}}
	importIntoDeck(t, &fakeCardService{allCards: owned}, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n", true)
	assert.Equal(t, 5, decks.cardCommanderID)

	commanderID := 9
	decks = &fakeDeckService{decks: []deck.Deck{{ID: targetDeckID, Name: "Target", CommanderID: &commanderID}}}
	importIntoDeck(t, &fakeCardService{allCards: owned}, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n", true)
	assert.Zero(t, decks.cardCommanderID)
	assert.Equal(t, []int{5}, decks.linkedCards[targetDeckID])
}

func TestImportIntoDeck_ReturnsDeckNotFound(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, deckImportResolver())

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, false, strings.NewReader("1 Sol Ring\n"))

	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestImportIntoDeck_ReturnsErrorWhenScryfallResolveFails(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("scryfall is down")}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, withTargetDeck(&fakeDeckService{}), resolver)

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, false, strings.NewReader("1 Sol Ring (SLD) 1011\n"))

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

func TestImportIntoDeck_ReturnsErrorWhenTheCollectionCannotBeLoaded(t *testing.T) {
	svc := NewService(&fakeCardService{getAllErr: errors.New("db is down")}, &fakeStorageService{}, withTargetDeck(&fakeDeckService{}), deckImportResolver())

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, false, strings.NewReader("1 Sol Ring (SLD) 1011\n"))

	require.Error(t, err)
}

func TestImportIntoDeck_WarnsWhenPuttingACopyInTheDeckFails(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{linkErr: errors.New("link failed")}

	summary := importDeck(t, cards, decks, "1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 0, summary.CardsLinked)
	assert.NotEmpty(t, summary.Warnings)
}

func TestImportIntoDeck_WarnsWhenAddingAPendingCardFails(t *testing.T) {
	decks := &fakeDeckService{addPendingErr: errors.New("insert failed")}

	summary := importDeck(t, &fakeCardService{}, decks, "1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 0, summary.CardsPending)
	assert.Equal(t, 1, summary.CardsSkipped)
	assert.NotEmpty(t, summary.Warnings)
}

func TestImportManaBox_StoresColorsAndTypeFromScryfall(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Lightning Helix,RAV,aaaaaaaa-0000-0000-0000-000000000000,213,,1
`
	cards := &fakeCardService{}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByID("aaaaaaaa-0000-0000-0000-000000000000"): {
			ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", ManaValue: 2, Colors: "WR", CardType: "Instant", ColorIdentity: "WR",
		},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	require.NotNil(t, cards.created[0].Colors)
	assert.Equal(t, "WR", *cards.created[0].Colors)
	require.NotNil(t, cards.created[0].CardType)
	assert.Equal(t, "Instant", *cards.created[0].CardType)
	require.NotNil(t, cards.created[0].ColorIdentity)
	assert.Equal(t, "WR", *cards.created[0].ColorIdentity)
}

func TestImportManaBox_LeavesColorsAndTypeUnknownWhenScryfallMissesTheCard(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	assert.Nil(t, cards.created[0].Colors)
	assert.Nil(t, cards.created[0].CardType)
}

func TestRefreshCardDetails_FillsMissingDetailsFromScryfall(t *testing.T) {
	cards := &fakeCardService{missingDetails: []card.Card{
		{ID: 1, Name: "Sol Ring", ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000"},
		{ID: 2, Name: "Ghost Card", ScryfallID: "bbbbbbbb-0000-0000-0000-000000000000"},
	}}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByID("aaaaaaaa-0000-0000-0000-000000000000"): {
			ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", ManaValue: 1, Colors: "", CardType: "Artifact", ColorIdentity: "",
		},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	summary, err := svc.RefreshCardDetails(context.Background(), testUserID, 0)

	require.NoError(t, err)
	assert.Equal(t, DetailsRefreshSummary{Updated: 1, NotFound: 1}, summary)
	assert.Equal(t, card.Details{Colors: "", CardType: "Artifact", ManaValue: 1}, cards.setDetails[1])
	_, touched := cards.setDetails[2]
	assert.False(t, touched)
	assert.Equal(t, 0, cards.lastMissingAfterID)
	assert.Equal(t, RefreshDetailsChunkSize, cards.lastMissingLimit)
}

func TestRefreshCardDetails_ReturnsCursorWhenCardsRemain(t *testing.T) {
	cards := &fakeCardService{
		missingDetails: []card.Card{
			{ID: 11, ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000"},
			{ID: 14, ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000"},
		},
		missingCountAfter: map[int]int{14: 30},
	}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByID("aaaaaaaa-0000-0000-0000-000000000000"): {ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", CardType: "Artifact"},
	}}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	summary, err := svc.RefreshCardDetails(context.Background(), testUserID, 10)

	require.NoError(t, err)
	assert.Equal(t, 10, cards.lastMissingAfterID)
	assert.Equal(t, 2, summary.Updated)
	assert.Equal(t, 30, summary.Remaining)
	require.NotNil(t, summary.NextAfterID)
	assert.Equal(t, 14, *summary.NextAfterID)
}

func TestRefreshCardDetails_DoesNothingWhenNoCardIsMissingDetails(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("must not be called")}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, resolver)

	summary, err := svc.RefreshCardDetails(context.Background(), testUserID, 0)

	require.NoError(t, err)
	assert.Equal(t, DetailsRefreshSummary{}, summary)
}

func TestRefreshCardDetails_ReturnsScryfallUnavailable(t *testing.T) {
	cards := &fakeCardService{missingDetails: []card.Card{{ID: 1, ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000"}}}
	resolver := &fakeResolver{err: errors.New("timeout")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	_, err := svc.RefreshCardDetails(context.Background(), testUserID, 0)

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

func TestCommitPendingCards_CreatesEachCopyAndPutsItInTheDeck(t *testing.T) {
	colors := "R"
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{
		{ID: 1, Name: "Lightning Bolt", ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", SetCode: "2xm", CollectorNumber: "129", Quantity: 2, ManaValue: 1, Colors: &colors},
		{ID: 2, Name: "Sol Ring", ScryfallID: "bbbbbbbb-0000-0000-0000-000000000000", SetCode: "c21", CollectorNumber: "263", Quantity: 1, Foil: true},
	}}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Binder"}}}
	svc := NewService(cards, storages, decks, &fakeResolver{})
	storageID := 4

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", &storageID, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsCreated)
	require.Len(t, cards.created, 3)
	assert.Equal(t, "Lightning Bolt", cards.created[0].Name)
	require.NotNil(t, cards.created[0].Colors)
	assert.Equal(t, "R", *cards.created[0].Colors)
	require.NotNil(t, cards.created[0].StorageID)
	assert.Equal(t, 4, *cards.created[0].StorageID)
	assert.True(t, cards.created[2].Foil)
	assert.Len(t, decks.linkedCards["00000000-0000-0000-0000-000000000009"], 3)
	assert.Equal(t, []int{1, 2}, decks.removedPending)
}

func TestCommitPendingCards_RejectsAnUnknownStorage(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 1, Name: "Sol Ring", Quantity: 1}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	storageID := 42

	_, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", &storageID, nil, nil)

	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	assert.Empty(t, cards.created)
	assert.Empty(t, decks.removedPending)
}

func TestCommitPendingCards_ReturnsDeckNotFound(t *testing.T) {
	decks := &fakeDeckService{getDeckErr: deck.ErrNotFound}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, nil, nil)

	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestCommitPendingCards_KeepsTheItemWhenCreationFails(t *testing.T) {
	cards := &fakeCardService{createErr: errors.New("database down")}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 1, Name: "Sol Ring", Quantity: 1}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, nil, nil)

	assert.Error(t, err)
	assert.Empty(t, decks.removedPending)
}

func TestCommitPendingCards_CanCommitASingleCard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{
		{ID: 1, Name: "Lightning Bolt", Quantity: 1},
		{ID: 2, Name: "Sol Ring", Quantity: 2},
	}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	pendingID := 2

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, &pendingID, nil)

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
	assert.Equal(t, []int{2}, decks.removedPending)
}

func TestCommitPendingCards_CanCommitPartOfAPendingCard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 2, Name: "Island", Quantity: 34}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	pendingID, quantity := 2, 10

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, &pendingID, &quantity)

	require.NoError(t, err)
	assert.Equal(t, 10, summary.CardsCreated)
	assert.Len(t, decks.linkedCards["00000000-0000-0000-0000-000000000009"], 10)
	assert.Equal(t, map[int]int{2: 24}, decks.pendingQuantities)
	assert.Empty(t, decks.removedPending)
}

func TestCommitPendingCards_ClampsTheQuantityToThePendingCard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 2, Name: "Island", Quantity: 3}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	pendingID, quantity := 2, 10

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, &pendingID, &quantity)

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsCreated)
	assert.Equal(t, []int{2}, decks.removedPending)
	assert.Empty(t, decks.pendingQuantities)
}

func TestCommitPendingCards_ReturnsNotFoundForAnUnknownPendingCard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 1, Name: "Lightning Bolt", Quantity: 1}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	pendingID := 42

	_, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, &pendingID, nil)

	assert.ErrorIs(t, err, deck.ErrPendingCardNotFound)
	assert.Empty(t, cards.created)
}

func TestCommitPendingCards_OffersEachItemAsCommanderOnce(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 3, Name: "Atraxa", Quantity: 2}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, nil, nil)

	require.NoError(t, err)
	require.Len(t, cards.created, 2)
	assert.Equal(t, map[int]int{3: cards.created[0].ID}, decks.promoted)
}

func TestCommitPendingCards_CommitAllSkipsConsideringCardsAndKeepsTheBoard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{
		{ID: 1, Name: "Duress", Quantity: 1, Board: deck.BoardSideboard},
		{ID: 2, Name: "Opt", Quantity: 1, Board: deck.BoardConsidering},
	}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	require.Len(t, cards.created, 1)
	assert.Equal(t, "Duress", cards.created[0].Name)
	assert.Equal(t, []int{1}, decks.removedPending)
	linked := decks.linkedCards["00000000-0000-0000-0000-000000000009"]
	require.Len(t, linked, 1)
	assert.Equal(t, deck.BoardSideboard, decks.linkedBoards[linked[0]])
}

func TestCommitPendingCards_CanCommitASingleConsideringCard(t *testing.T) {
	cards := &fakeCardService{}
	decks := &fakeDeckService{pending: []deck.PendingCard{{ID: 2, Name: "Opt", Quantity: 1, Board: deck.BoardConsidering}}}
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	pendingID := 2

	summary, err := svc.CommitPendingCards(context.Background(), testUserID, "00000000-0000-0000-0000-000000000009", nil, &pendingID, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	linked := decks.linkedCards["00000000-0000-0000-0000-000000000009"]
	require.Len(t, linked, 1)
	assert.Equal(t, deck.BoardConsidering, decks.linkedBoards[linked[0]])
}

func TestImportIntoDeck_PutsEachSectionInItsBoard(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 6, Name: "Sol Ring", ScryfallID: solRingID}}}
	decks := &fakeDeckService{}

	summary := importDeck(t, cards, decks, "Sideboard\n1 Sol Ring (SLD) 1011\n\nMaybeboard:\n1 Atraxa, Praetors' Voice (CMR) 1\n", true)

	assert.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, 1, summary.CardsPending)
	assert.Equal(t, deck.BoardSideboard, decks.linkedBoards[6])
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, deck.BoardConsidering, decks.addedPending[0].Board)
	assert.Zero(t, decks.cardCommanderID)
	assert.Zero(t, decks.pendingCommanderID)
}

func TestImportIntoDeck_TakesTheCommanderFromTheMainBoard(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID}}}
	decks := &fakeDeckService{}

	importDeck(t, cards, decks, "Sideboard\n1 Sol Ring (SLD) 1011\nDeck\n1 Atraxa, Praetors' Voice (CMR) 1\n", true)

	assert.Equal(t, 5, decks.cardCommanderID)
	assert.Equal(t, deck.BoardMain, decks.linkedBoards[5])
}
