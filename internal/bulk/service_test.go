package bulk

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

	movedIDs     []int
	movedStorage int
	moveErr      error
	movedFrom    int
	moveCalled   bool
}

func (f *fakeCardService) MoveStorageCards(ctx context.Context, userID string, fromStorageID, toStorageID int) (int, error) {
	f.moveCalled = true
	if f.moveErr != nil {
		return 0, f.moveErr
	}
	f.movedFrom = fromStorageID
	f.movedStorage = toStorageID
	return 7, nil
}

func (f *fakeCardService) MoveCardsToStorage(ctx context.Context, userID string, ids []int, storageID int) (int, error) {
	if f.moveErr != nil {
		return 0, f.moveErr
	}
	f.movedIDs = ids
	f.movedStorage = storageID
	return len(ids), nil
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
	tags    deck.DeckTags
	setTags map[string][]string

	decks       []deck.Deck
	nextID      int
	created     []deck.Deck
	linkedCards map[string][]int

	cardsByDeck   map[string][]deck.DeckCard
	getDeckErr    error
	createDeckErr error

	pending        []deck.PendingCard
	entries        map[string][]deck.DeckEntry
	getContentErr  error
	removedPending []int
	commits        []deck.PendingCommit

	addedPending       []deck.PendingCard
	pendingCommanderID int
	cardCommanderID    int
	pendingPartnerID   int
	cardPartnerID      int
	pendingQuantities  map[int]int
	linkedBoards       map[int]string

	owners      map[string]string
	ownedCopies map[string]int
	deleted     []string

	unlinkedCards    []int
	commanderCleared bool
	clearedSlot      deck.CommanderSlot

	collection *fakeCardService
	storages   *fakeStorageService

	placed       []placedPlan
	placeSummary *deck.PlacementSummary
	placeErr     error
}

type placedPlan struct {
	deckID string
	plan   deck.PlacementPlan
}

// PlaceCards records the plan and, unless told otherwise, reports every card
// as pending.
func (f *fakeDeckService) PlaceCards(ctx context.Context, userID string, deckID string, plan deck.PlacementPlan) (deck.PlacementSummary, error) {
	if f.placeErr != nil {
		return deck.PlacementSummary{}, f.placeErr
	}
	f.placed = append(f.placed, placedPlan{deckID: deckID, plan: plan})
	if f.placeSummary != nil {
		return *f.placeSummary, nil
	}
	var summary deck.PlacementSummary
	for _, line := range plan.Lines {
		summary.CardsPending += line.Quantity
	}
	return summary, nil
}

// placedPlan returns the one plan placed into the deck.
func (f *fakeDeckService) placedPlan(t *testing.T, deckID string) deck.PlacementPlan {
	t.Helper()
	require.Len(t, f.placed, 1)
	require.Equal(t, deckID, f.placed[0].deckID)
	return f.placed[0].plan
}

func (f *fakeDeckService) RemoveCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error {
	f.unlinkedCards = append(f.unlinkedCards, cardID)
	return nil
}

func (f *fakeDeckService) ClearCommander(ctx context.Context, userID string, deckID string, slot deck.CommanderSlot) error {
	f.commanderCleared = true
	f.clearedSlot = slot
	return nil
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

// newTestService lets the fake deck service answer FindOwnedCopies from the
// collection and storages held by the fake card and storage services.
func newTestService(cards cardService, storages storageService, decks deckService, scryfall ScryfallResolver) *Service {
	if d, ok := decks.(*fakeDeckService); ok {
		d.collection, _ = cards.(*fakeCardService)
		d.storages, _ = storages.(*fakeStorageService)
	}
	return NewService(cards, storages, decks, scryfall)
}

func (f *fakeDeckService) FindOwnedCopies(ctx context.Context, userID string, targetDeckID string, wanted deck.CopiesWanted) ([]deck.OwnedCopy, error) {
	if f.collection == nil {
		return nil, nil
	}
	wantedPrintings := make(map[string]bool, len(wanted.ScryfallIDs))
	for _, id := range wanted.ScryfallIDs {
		wantedPrintings[strings.ToLower(id)] = true
	}
	wantedNames := make(map[string]bool, len(wanted.Names))
	for _, name := range wanted.Names {
		wantedNames[deck.CardNameKey(name)] = true
	}
	inDeck, inTarget := map[int]bool{}, map[int]bool{}
	decks, _, _ := f.GetAllDecks(ctx, userID, deck.Filter{})
	for _, d := range decks {
		for _, c := range f.cardsByDeck[d.ID] {
			inDeck[c.ID] = true
			if d.ID == targetDeckID {
				inTarget[c.ID] = true
			}
		}
	}
	var copies []deck.OwnedCopy
	for _, c := range f.collection.allCards {
		if !wantedPrintings[strings.ToLower(c.ScryfallID)] && !wantedNames[deck.CardNameKey(c.Name)] && !wantedNames[deck.CardNameKey(frontFaceName(c.Name))] {
			continue
		}
		copies = append(copies, deck.OwnedCopy{
			ID: c.ID, Name: c.Name, ScryfallID: c.ScryfallID, Foil: c.Foil, Proxy: c.Proxy,
			InDeck: inDeck[c.ID], InTargetDeck: inTarget[c.ID], LockedAgainstTarget: f.lockedAgainst(c.StorageID, targetDeckID),
		})
	}
	return copies, nil
}

func (f *fakeDeckService) lockedAgainst(storageID *int, deckID string) bool {
	if storageID == nil || f.storages == nil {
		return false
	}
	for _, st := range f.storages.storages {
		if st.ID == *storageID {
			return st.Locked && !slices.Contains(st.AllowedDeckIDs, deckID)
		}
	}
	return false
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
	if f.setTags == nil {
		f.setTags = map[string][]string{}
	}
	f.setTags[cardName] = tags
	return deck.TaggedCard{Name: cardName, Tags: tags}, nil
}

func (f *fakeDeckService) SetCardCommander(ctx context.Context, userID string, deckID string, slot deck.CommanderSlot, cardID int) error {
	if slot == deck.SlotPartner {
		f.cardPartnerID = cardID
		return nil
	}
	f.cardCommanderID = cardID
	return nil
}

func (f *fakeDeckService) AddPendingCard(ctx context.Context, userID string, deckID string, p deck.PendingCard) (deck.PendingCard, error) {
	p.ID = len(f.addedPending) + 100
	p.DeckID = deckID
	f.addedPending = append(f.addedPending, p)
	return p, nil
}

func (f *fakeDeckService) SetPendingCommander(ctx context.Context, userID string, deckID string, slot deck.CommanderSlot, pendingID int) error {
	if slot == deck.SlotPartner {
		f.pendingPartnerID = pendingID
		return nil
	}
	f.pendingCommanderID = pendingID
	return nil
}

func (f *fakeDeckService) CommitPendingCards(ctx context.Context, userID string, deckID string, req deck.PendingCommit) (deck.PendingCommitSummary, error) {
	f.commits = append(f.commits, req)
	for _, p := range f.pending {
		if req.PendingID != nil && p.ID == *req.PendingID {
			return deck.PendingCommitSummary{CardsCreated: p.Quantity}, nil
		}
	}
	return deck.PendingCommitSummary{}, deck.ErrPendingCardNotFound
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
	return f.cardsByDeck[id], nil
}

func (f *fakeDeckService) GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error) {
	d, err := f.GetDeck(ctx, userID, deckID)
	if err != nil {
		return deck.DeckContent{}, err
	}
	if f.getContentErr != nil {
		return deck.DeckContent{}, f.getContentErr
	}
	return deck.DeckContent{Deck: d, Entries: f.entries[deckID]}, nil
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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, resolver)

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	assert.Equal(t, 1.0, cards.created[0].ManaValue)
}

func TestImportManaBox_StoresTheEnglishScryfallName(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Anneau solaire,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByID("aaaaaaaa-0000-0000-0000-000000000000"): {ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000", Name: "Sol Ring", ManaValue: 1},
	}}
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	_, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
}

func TestImportManaBox_DefaultsManaValueToZeroWhenScryfallUnavailable(t *testing.T) {
	csv := `Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity
Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,1
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{}
	decks := &fakeDeckService{}
	resolver := &fakeResolver{err: errors.New("scryfall unreachable")}
	svc := newTestService(cards, storages, decks, resolver)

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err, "a scryfall outage must not break a manabox import, which never depended on it before")
	require.Len(t, cards.created, 1)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Equal(t, 0.0, cards.created[0].ManaValue)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

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
	svc := newTestService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.ImportManaBox(context.Background(), testUserID, nil, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 2, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 2)
}

func TestImportManaBox_InvalidFileReturnsErrInvalidFile(t *testing.T) {
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

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
	svc := newTestService(cards, storages, &fakeDeckService{}, resolver)

	summary, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 2, summary.CardsCreated)
	assert.Equal(t, 0, summary.CardsSkipped)
	require.Len(t, cards.created, 2)
	assert.Equal(t, "cccccccc-0000-0000-0000-000000000000", cards.created[0].ScryfallID)
	assert.Equal(t, 7, *cards.created[0].StorageID)
	assert.Equal(t, 1.0, cards.created[0].ManaValue)
}

func TestImportMoxfieldCollection_StoresTheEnglishScryfallName(t *testing.T) {
	csv := `Count,Name,Edition,Foil,Collector Number
1,Anneau solaire,sld,,1011
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7, Name: "Binder", Type: "binder"}}}
	resolver := &fakeResolver{resolved: map[string]ResolvedCard{resolveKey("sld", "1011"): {ScryfallID: solRingID, Name: "Sol Ring", ManaValue: 1}}}
	svc := newTestService(cards, storages, &fakeDeckService{}, resolver)

	_, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, cards.created, 1)
	assert.Equal(t, "Sol Ring", cards.created[0].Name)
}

func TestImportMoxfieldCollection_SkipsUnresolvedCards(t *testing.T) {
	csv := `Count,Name,Edition,Foil,Collector Number
1,Mystery Card,xxx,,999
`
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 7}}}
	svc := newTestService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	summary, err := svc.ImportMoxfieldCollection(context.Background(), testUserID, 7, strings.NewReader(csv))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 1)
}

func TestImportMoxfieldCollection_UnknownStorageReturnsErrTargetStorageNotFound(t *testing.T) {
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

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
	svc := newTestService(&fakeCardService{}, storages, &fakeDeckService{}, resolver)

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
		resolveKey("CMR", "1"):    {ScryfallID: atraxaID, Name: "Atraxa, Praetors' Voice", ManaValue: 4, Colors: "WUBG", CardType: "Creature", ColorIdentity: "WUBG"},
		resolveKey("SLD", "1011"): {ScryfallID: solRingID, Name: "Sol Ring", ManaValue: 1, CardType: "Artifact"},
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
	svc := newTestService(cards, &fakeStorageService{}, withTargetDeck(decks), deckImportResolver())
	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{CommanderFromFirstLine: commanderFromFirstLine}, strings.NewReader(decklist))
	require.NoError(t, err)
	return summary
}

func TestImportIntoDeck_PlacesTheResolvedLinesAsOnePlan(t *testing.T) {
	decks := &fakeDeckService{placeSummary: &deck.PlacementSummary{CardsLinked: 2, CardsPending: 1}}

	summary := importDeck(t, &fakeCardService{}, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n2 Sol Ring (SLD) 1011 *F*\n", true)

	assert.Equal(t, deck.PlacementPlan{Lines: []deck.PlacementLine{
		{
			Printing:  deck.Printing{Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID, SetCode: "CMR", CollectorNumber: "1", ManaValue: 4, Colors: ptrString("WUBG"), CardType: ptrString("Creature"), ColorIdentity: ptrString("WUBG")},
			Board:     deck.BoardMain,
			Quantity:  1,
			Commander: true,
		},
		{
			Printing: deck.Printing{Name: "Sol Ring", ScryfallID: solRingID, SetCode: "SLD", CollectorNumber: "1011", ManaValue: 1, Colors: ptrString(""), CardType: ptrString("Artifact"), ColorIdentity: ptrString("")},
			Foil:     true,
			Board:    deck.BoardMain,
			Quantity: 2,
		},
	}}, decks.placedPlan(t, targetDeckID))
	assert.Equal(t, Summary{CardsLinked: 2, CardsPending: 1}, summary)
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

// placedLine is what a test checks of a placement line.
type placedLine struct {
	name        string
	setCode     string
	quantity    int
	board       string
	commander   bool
	anyPrinting bool
	foil        bool
	proxy       deck.ProxyMatch
}

func placedLines(plan deck.PlacementPlan) []placedLine {
	lines := make([]placedLine, 0, len(plan.Lines))
	for _, l := range plan.Lines {
		lines = append(lines, placedLine{l.Printing.Name, l.Printing.SetCode, l.Quantity, l.Board, l.Commander, l.AnyPrinting, l.Foil, l.Proxy})
	}
	return lines
}

func TestImportIntoDeck_UsesLockedStoragesOnlyWhenAsked(t *testing.T) {
	for _, include := range []bool{false, true} {
		decks := withTargetDeck(&fakeDeckService{})
		svc := newTestService(&fakeCardService{}, &fakeStorageService{}, decks, deckImportResolver())

		_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{IncludeLockedStorages: include}, strings.NewReader("1 Sol Ring (SLD) 1011\n"))

		require.NoError(t, err)
		assert.Equal(t, include, decks.placedPlan(t, targetDeckID).IncludeLockedStorages)
	}
}

func TestImportIntoDeck_AddsMoxfieldTags(t *testing.T) {
	decks := &fakeDeckService{}

	importDeck(t, &fakeCardService{}, decks, "1 Sol Ring (SLD) 1011 #ramp #mana\nSideboard\n2 Sol Ring (SLD) 1011 #artifact #Ramp\n1 Atraxa, Praetors' Voice (CMR) 1\n", false)

	assert.Equal(t, []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"ramp", "mana", "artifact"}}}, decks.placedPlan(t, targetDeckID).Tags)
}

func TestImportIntoDeck_ReportsTagsTheDeckCouldNotApply(t *testing.T) {
	decks := &fakeDeckService{placeSummary: &deck.PlacementSummary{CardsPending: 1, SkippedTags: []deck.SkippedTags{{CardName: "Sol Ring", Err: deck.ErrInvalidTag}}}}

	summary := importDeck(t, &fakeCardService{}, decks, "1 Sol Ring (SLD) 1011 #ramp\n", false)

	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], `could not tag "Sol Ring"`)
	assert.Equal(t, 1, summary.CardsPending)
}

func TestImportIntoDeck_StoresAndTagsTheEnglishScryfallName(t *testing.T) {
	decks := &fakeDeckService{}

	importDeck(t, &fakeCardService{}, decks, "2 Anneau solaire (SLD) 1011 #ramp\n", false)

	plan := decks.placedPlan(t, targetDeckID)
	assert.Equal(t, []placedLine{{name: "Sol Ring", setCode: "SLD", quantity: 2, board: deck.BoardMain}}, placedLines(plan))
	assert.Equal(t, []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"ramp"}}}, plan.Tags)
}

func TestImportIntoDeck_MarksNoCommanderWithoutTheFlag(t *testing.T) {
	decks := &fakeDeckService{}

	importDeck(t, &fakeCardService{}, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n", false)

	assert.False(t, decks.placedPlan(t, targetDeckID).Lines[0].Commander)
}

func TestImportIntoDeck_UnresolvedLinesAreSkippedButTheRestIsImported(t *testing.T) {
	decks := &fakeDeckService{}

	summary := importDeck(t, &fakeCardService{}, decks, "1 Mystery Commander (XXX) 999\n1 Sol Ring (SLD) 1011\n", true)

	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Equal(t, 1, summary.CardsPending)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], `commander "Mystery Commander"`)
	assert.Equal(t, []placedLine{{name: "Sol Ring", setCode: "SLD", quantity: 1, board: deck.BoardMain}}, placedLines(decks.placedPlan(t, targetDeckID)))
}

func plainListResolver() *fakeResolver {
	return &fakeResolver{resolved: map[string]ResolvedCard{
		resolveKeyByName("Sol Ring"):    {ScryfallID: solRingID, Name: "Sol Ring", SetCode: "cmm", CollectorNumber: "464", ManaValue: 1, CardType: "Artifact"},
		resolveKeyByName("Fire // Ice"): {ScryfallID: "44444444-0000-0000-0000-000000000000", Name: "Fire // Ice", SetCode: "mh2", CollectorNumber: "290", ManaValue: 4},
	}}
}

func importPlainList(t *testing.T, decks *fakeDeckService, decklist string) Summary {
	t.Helper()
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, withTargetDeck(decks), plainListResolver())
	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader(decklist))
	require.NoError(t, err)
	return summary
}

func TestImportIntoDeck_PlainListAcceptsAnyPrintingAndAddsTheScryfallOneAsPending(t *testing.T) {
	decks := &fakeDeckService{}

	importPlainList(t, decks, "3 Sol Ring\n2 Fire / Ice\n")

	plan := decks.placedPlan(t, targetDeckID)
	assert.Equal(t, []placedLine{
		{name: "Sol Ring", setCode: "cmm", quantity: 3, board: deck.BoardMain, anyPrinting: true},
		{name: "Fire // Ice", setCode: "mh2", quantity: 2, board: deck.BoardMain, anyPrinting: true},
	}, placedLines(plan))
	assert.Equal(t, solRingID, plan.Lines[0].Printing.ScryfallID)
	assert.Equal(t, "464", plan.Lines[0].Printing.CollectorNumber)
}

func TestImportIntoDeck_PlainListWarnsAboutUnknownNames(t *testing.T) {
	summary := importPlainList(t, &fakeDeckService{}, "1 Not A Real Card\n1 Sol Ring\n")

	assert.Equal(t, 1, summary.CardsSkipped)
	assert.Equal(t, 1, summary.CardsPending)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], "Not A Real Card")
}

func TestImportIntoDeck_ReturnsDeckNotFound(t *testing.T) {
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, deckImportResolver())

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader("1 Sol Ring\n"))

	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestImportIntoDeck_ReturnsErrorWhenScryfallResolveFails(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("scryfall is down")}
	decks := withTargetDeck(&fakeDeckService{})
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, decks, resolver)

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader("1 Sol Ring (SLD) 1011\n"))

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
	assert.Empty(t, decks.placed)
}

func TestImportIntoDeck_ReturnsThePlacementErrorWithoutAPartialSummary(t *testing.T) {
	placeErr := errors.New("insert failed")
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, withTargetDeck(&fakeDeckService{placeErr: placeErr}), deckImportResolver())

	summary, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader("1 Mystery (XXX) 1\n1 Sol Ring (SLD) 1011\n"))

	assert.ErrorIs(t, err, placeErr)
	assert.Equal(t, Summary{}, summary)
}

func TestImportIntoDeck_ReturnsDeckNotFoundWhenTheDeckGoesAwayWhilePlacing(t *testing.T) {
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, withTargetDeck(&fakeDeckService{placeErr: deck.ErrNotFound}), deckImportResolver())

	_, err := svc.ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, strings.NewReader("1 Sol Ring (SLD) 1011\n"))

	assert.ErrorIs(t, err, ErrDeckNotFound)
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
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

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
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{})

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
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

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
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

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
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{}, resolver)

	summary, err := svc.RefreshCardDetails(context.Background(), testUserID, 0)

	require.NoError(t, err)
	assert.Equal(t, DetailsRefreshSummary{}, summary)
}

func TestRefreshCardDetails_ReturnsScryfallUnavailable(t *testing.T) {
	cards := &fakeCardService{missingDetails: []card.Card{{ID: 1, ScryfallID: "aaaaaaaa-0000-0000-0000-000000000000"}}}
	resolver := &fakeResolver{err: errors.New("timeout")}
	svc := newTestService(cards, &fakeStorageService{}, &fakeDeckService{}, resolver)

	_, err := svc.RefreshCardDetails(context.Background(), testUserID, 0)

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

func TestImportIntoDeck_PutsEachSectionInItsBoardAndTakesTheCommanderFromTheMainBoard(t *testing.T) {
	decks := &fakeDeckService{}

	importDeck(t, &fakeCardService{}, decks, "Sideboard\n1 Sol Ring (SLD) 1011\n\nMaybeboard:\n1 Sol Ring (SLD) 1011\nDeck\n1 Atraxa, Praetors' Voice (CMR) 1\n", true)

	assert.Equal(t, []placedLine{
		{name: "Sol Ring", setCode: "SLD", quantity: 1, board: deck.BoardSideboard},
		{name: "Sol Ring", setCode: "SLD", quantity: 1, board: deck.BoardConsidering},
		{name: "Atraxa, Praetors' Voice", setCode: "CMR", quantity: 1, board: deck.BoardMain, commander: true},
	}, placedLines(decks.placedPlan(t, targetDeckID)))
}
