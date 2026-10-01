package bulk

import (
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

const testUserID = "11111111-1111-1111-1111-111111111111"

// --- fakes -------------------------------------------------------------

type fakeCardService struct {
	nextID    int
	created   []card.Card
	createErr error

	allCards  []card.Card
	getAllErr error
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
	total := len(f.allCards)
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
	return f.allCards[start:end], total, nil
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
	decks       []deck.Deck
	nextID      int
	created     []deck.Deck
	linkedCards map[int][]int
	linkErr     error

	cardsByDeck     map[int][]deck.DeckCard
	getDeckErr      error
	getDeckCardsErr error
	createDeckErr   error
}

func (f *fakeDeckService) GetAllDecks(ctx context.Context, userID string, filter deck.Filter) ([]deck.Deck, int, error) {
	return f.decks, len(f.decks), nil
}

func (f *fakeDeckService) GetDeck(ctx context.Context, userID string, id int) (deck.Deck, error) {
	if f.getDeckErr != nil {
		return deck.Deck{}, f.getDeckErr
	}
	for _, d := range f.decks {
		if d.ID == id {
			return d, nil
		}
	}
	return deck.Deck{}, deck.ErrNotFound
}

func (f *fakeDeckService) GetDeckCards(ctx context.Context, userID string, id int) ([]deck.DeckCard, error) {
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
	d.ID = f.nextID
	f.decks = append(f.decks, d)
	f.created = append(f.created, d)
	return d, nil
}

func (f *fakeDeckService) PutCardInDeck(ctx context.Context, userID string, deckID, cardID int) error {
	if f.linkErr != nil {
		return f.linkErr
	}
	if f.linkedCards == nil {
		f.linkedCards = make(map[int][]int)
	}
	f.linkedCards[deckID] = append(f.linkedCards[deckID], cardID)
	return nil
}

type fakeResolver struct {
	resolved map[string]string
	err      error
}

func (f *fakeResolver) Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]string)
	for _, id := range identifiers {
		key := resolveKey(id.SetCode, id.CollectorNumber)
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

	summary, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 1, summary.StoragesCreated)
	assert.Equal(t, 0, summary.DecksCreated)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
	require.NotNil(t, cards.created[0].StorageID)
	assert.Equal(t, storages.created[0].ID, *cards.created[0].StorageID)
}

func TestImportManaBox_ReusesExistingStorageByName(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 42, Name: "Main Binder", Type: "binder"}}}
	decks := &fakeDeckService{}
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

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

	summary, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.DecksCreated)
	require.Len(t, decks.created, 1)
	assert.Equal(t, "commander", decks.created[0].Format)
	assert.Equal(t, []int{cards.created[0].ID}, decks.linkedCards[decks.created[0].ID])
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

	summary, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

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

	_, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

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

	summary, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 2, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 2)
}

func TestImportManaBox_InvalidFileReturnsErrInvalidFile(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.ImportManaBox(context.Background(), testUserID, strings.NewReader("not,a,valid,header\n"))

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
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("sld", "1011"): "cccccccc-0000-0000-0000-000000000000"}}
	svc := NewService(cards, storages, &fakeDeckService{}, resolver)

	summary, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 0, summary.CardsSkipped)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "cccccccc-0000-0000-0000-000000000000", cards.created[0].ScryfallID)
	assert.Equal(t, 7, *cards.created[0].StorageID)
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

// --- ImportMoxfieldDeck ---------------------------------------------------

func TestImportMoxfieldDeck_FirstLineBecomesCommander(t *testing.T) {
	decklist := "1 Atraxa, Praetors' Voice (CMR) 1\n1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{resolved: map[string]string{
		resolveKey("CMR", "1"):    "11111111-0000-0000-0000-000000000000",
		resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000",
	}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "My Deck", Format: "commander", CommanderFromFirstLine: true,
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 1, summary.DecksCreated)
	require.Len(t, decks.created, 1)
	require.NotNil(t, decks.created[0].CommanderID)
	assert.Equal(t, cards.created[0].ID, *decks.created[0].CommanderID)
	assert.Equal(t, "Atraxa, Praetors' Voice", cards.created[0].Name)
	assert.ElementsMatch(t, []int{cards.created[0].ID, cards.created[1].ID}, decks.linkedCards[decks.created[0].ID])
}

func TestImportMoxfieldDeck_WithoutCommanderFlag(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern", CommanderFromFirstLine: false,
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Nil(t, decks.created[0].CommanderID)
}

func TestImportMoxfieldDeck_AssignsStorageWhenProvided(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 9}}}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, storages, &fakeDeckService{}, resolver)

	storageID := 9
	_, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern", StorageID: &storageID,
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	require.NotNil(t, cards.created[0].StorageID)
	assert.Equal(t, 9, *cards.created[0].StorageID)
}

func TestImportMoxfieldDeck_UnresolvedCommanderIsSkippedButDeckStillCreated(t *testing.T) {
	decklist := "1 Mystery Commander (XXX) 999\n1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "My Deck", Format: "commander", CommanderFromFirstLine: true,
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Nil(t, decks.created[0].CommanderID)
}

func TestImportMoxfieldDeck_ReturnsErrorWhenTargetStorageDoesNotExist(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	storages := &fakeStorageService{} // no storages registered
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, storages, &fakeDeckService{}, resolver)

	missingStorageID := 999
	_, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern", StorageID: &missingStorageID,
	}, strings.NewReader(decklist))

	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
}

func TestImportMoxfieldDeck_ReturnsErrorWhenScryfallResolveFails(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	resolver := &fakeResolver{err: errors.New("scryfall is down")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	_, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern",
	}, strings.NewReader(decklist))

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

func TestImportMoxfieldDeck_SkipsLineAndWarnsWhenCardCreationFails(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{createErr: errors.New("db is down")}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern",
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 1, summary.CardsSkipped)
	assert.NotEmpty(t, summary.Warnings)
}

func TestImportMoxfieldDeck_ReturnsErrorWhenDeckCreationFails(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{createDeckErr: errors.New("db is down")}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	_, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern",
	}, strings.NewReader(decklist))

	require.Error(t, err)
}

func TestImportMoxfieldDeck_WarnsWhenLinkingCommanderToDeckFails(t *testing.T) {
	decklist := "1 Atraxa, Praetors' Voice (CMR) 1\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{linkErr: errors.New("link failed")}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("CMR", "1"): "11111111-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "My Deck", Format: "commander", CommanderFromFirstLine: true,
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.NotEmpty(t, summary.Warnings)
}

func TestImportMoxfieldDeck_WarnsWhenLinkingRegularCardToDeckFails(t *testing.T) {
	decklist := "1 Sol Ring (SLD) 1011\n"
	cards := &fakeCardService{}
	decks := &fakeDeckService{linkErr: errors.New("link failed")}
	resolver := &fakeResolver{resolved: map[string]string{resolveKey("SLD", "1011"): "22222222-0000-0000-0000-000000000000"}}
	svc := NewService(cards, &fakeStorageService{}, decks, resolver)

	summary, err := svc.ImportMoxfieldDeck(context.Background(), testUserID, MoxfieldDeckImportRequest{
		Name: "Modern Pile", Format: "modern",
	}, strings.NewReader(decklist))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.NotEmpty(t, summary.Warnings)
}
