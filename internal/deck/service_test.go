package deck

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID = "11111111-1111-1111-1111-111111111111"
const otherUserID = "22222222-2222-2222-2222-222222222222"

type fakeRepository struct {
	decks        []Deck
	findAllTotal int
	findAllErr   error
	lastUserID   string
	lastFilter   Filter

	findByIDDeck Deck
	findByIDErr  error

	createdDeck Deck
	createErr   error

	updatedDeck Deck
	updateErr   error

	deleteErr error

	sharedOwnerID string
	sharedDeck    Deck
	sharedErr     error
	lastShareID   string

	getDeckCards    []DeckCard
	getDeckCardsErr error

	linkErr   error
	unlinkErr error

	pending          []PendingCard
	createdPending   PendingCard
	deletePendingErr error
	lastPendingID    int
}

func (f *fakeRepository) FindPendingCards(ctx context.Context, userID string, deckID int) ([]PendingCard, error) {
	f.lastUserID = userID
	return f.pending, nil
}

func (f *fakeRepository) CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error) {
	f.lastUserID = userID
	f.createdPending = p
	p.ID = 7
	return p, nil
}

func (f *fakeRepository) DeletePendingCard(ctx context.Context, userID string, deckID, id int) error {
	f.lastUserID = userID
	f.lastPendingID = id
	return f.deletePendingErr
}

func (f *fakeRepository) FindAll(ctx context.Context, userID string, filter Filter) ([]Deck, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.decks, f.findAllTotal, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, userID string, id int) (Deck, error) {
	f.lastUserID = userID
	if f.findByIDErr != nil {
		return Deck{}, f.findByIDErr
	}
	return f.findByIDDeck, nil
}

func (f *fakeRepository) Create(ctx context.Context, userID string, d Deck) (Deck, error) {
	f.lastUserID = userID
	if f.createErr != nil {
		return Deck{}, f.createErr
	}
	f.createdDeck = d
	d.ID = 1
	return d, nil
}

func (f *fakeRepository) Update(ctx context.Context, userID string, d Deck) (Deck, error) {
	f.lastUserID = userID
	if f.updateErr != nil {
		return Deck{}, f.updateErr
	}
	f.updatedDeck = d
	return d, nil
}

func (f *fakeRepository) Delete(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
	return f.deleteErr
}

func (f *fakeRepository) FindShared(ctx context.Context, shareID string) (string, Deck, error) {
	f.lastShareID = shareID
	if f.sharedErr != nil {
		return "", Deck{}, f.sharedErr
	}
	return f.sharedOwnerID, f.sharedDeck, nil
}

func (f *fakeRepository) FindCardsByDeckID(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]DeckCard, error) {
	f.lastUserID = userID
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.getDeckCards, nil
}

func (f *fakeRepository) LinkCardToDeck(ctx context.Context, userID string, deckID, cardID int) error {
	f.lastUserID = userID
	return f.linkErr
}

func (f *fakeRepository) UnlinkCardFromDeck(ctx context.Context, userID string, deckID, cardID int) error {
	f.lastUserID = userID
	return f.unlinkErr
}

func TestService_GetAllDecks_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, _, err := service.GetAllDecks(context.Background(), testUserID, Filter{})

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetAllDecks_PassesFilterToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, _, err := service.GetAllDecks(context.Background(), testUserID, Filter{Format: "commander", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, Filter{Format: "commander", Page: 1, Limit: 25}, repo.lastFilter)
}

func TestService_GetAllDecks_ReturnsDecksAndTotalFromRepository(t *testing.T) {
	expected := []Deck{
		{ID: 1, Name: "Otterly Playful", Format: "commander"},
		{ID: 2, Name: "Cutelings Everywhere", Format: "commander"},
	}
	repo := &fakeRepository{decks: expected, findAllTotal: 2}
	service := NewService(repo)

	result, total, err := service.GetAllDecks(context.Background(), testUserID, Filter{})

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, 2, total)
}

func TestService_GetAllDecks_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, total, err := service.GetAllDecks(context.Background(), testUserID, Filter{})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Zero(t, total)
}

func TestService_GetDeck_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.GetDeck(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetDeck_ReturnsNotFoundWhenDeckBelongsToAnotherUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetDeck(context.Background(), otherUserID, 1)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateDeck_PassesUserIDAndDeckToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	input := Deck{Name: "Otterly Playful", Format: "commander"}

	_, err := service.CreateDeck(context.Background(), testUserID, input)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
	assert.Equal(t, input, repo.createdDeck)
}

func TestService_CreateDeck_ReturnsDeckFromRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	result, err := service.CreateDeck(context.Background(), testUserID, Deck{Name: "Otterly Playful", Format: "commander"})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestService_CreateDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("insert failed")}
	service := NewService(repo)

	result, err := service.CreateDeck(context.Background(), testUserID, Deck{Name: "Otterly Playful"})

	assert.Error(t, err)
	assert.Equal(t, Deck{}, result)
}

func TestService_UpdateDeck_AppliesPartialChangesOnExistingDeck(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "modern"}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	newName := "Renamed"
	req := updateDeckRequest{Name: &newName}

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	assert.Equal(t, "Renamed", result.Name)
	assert.Equal(t, "modern", result.Format)
}

func TestService_UpdateDeck_AppliesFormatAndCommanderIDWhenProvided(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "modern"}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	newFormat := "commander"
	newCommanderID := 42
	req := updateDeckRequest{Format: &newFormat, CommanderID: &newCommanderID}

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	assert.Equal(t, "commander", result.Format)
	require.NotNil(t, result.CommanderID)
	assert.Equal(t, 42, *result.CommanderID)
}

func TestService_UpdateDeck_ClearsCommanderIDWhenRequested(t *testing.T) {
	existingCommanderID := 42
	existing := Deck{ID: 1, Name: "Red Deck", Format: "commander", CommanderID: &existingCommanderID}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	req := updateDeckRequest{ClearCommanderID: true}

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	assert.Nil(t, result.CommanderID)
}

func TestService_UpdateDeck_SetsBackgroundScryfallID(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "commander"}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	background := "436d6a84-4cea-4ca7-94aa-9d08280652af"
	req := updateDeckRequest{BackgroundScryfallID: &background}

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	require.NotNil(t, result.BackgroundScryfallID)
	assert.Equal(t, "436d6a84-4cea-4ca7-94aa-9d08280652af", *result.BackgroundScryfallID)
	assert.Equal(t, "Red Deck", result.Name)
}

func TestService_UpdateDeck_ClearsBackgroundWhenRequested(t *testing.T) {
	background := "436d6a84-4cea-4ca7-94aa-9d08280652af"
	existing := Deck{ID: 1, Name: "Red Deck", Format: "commander", BackgroundScryfallID: &background}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{ClearBackground: true})

	require.NoError(t, err)
	assert.Nil(t, result.BackgroundScryfallID)
}

func TestService_UpdateDeck_KeepsBackgroundWhenNotMentioned(t *testing.T) {
	background := "436d6a84-4cea-4ca7-94aa-9d08280652af"
	existing := Deck{ID: 1, Name: "Red Deck", Format: "commander", BackgroundScryfallID: &background}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	newName := "Renamed"
	result, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{Name: &newName})

	require.NoError(t, err)
	require.NotNil(t, result.BackgroundScryfallID)
	assert.Equal(t, background, *result.BackgroundScryfallID)
}

func TestService_UpdateDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	newName := "Doesn't matter"
	req := updateDeckRequest{Name: &newName}

	_, err := service.UpdateDeck(context.Background(), otherUserID, 999, req)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_UpdateDeck_PropagatesRepositoryUpdateError(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "modern"}
	repo := &fakeRepository{findByIDDeck: existing, updateErr: errors.New("update failed")}
	service := NewService(repo)

	newName := "New Name"
	req := updateDeckRequest{Name: &newName}

	_, err := service.UpdateDeck(context.Background(), testUserID, 1, req)

	assert.Error(t, err)
}

func TestService_DeleteDeck_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_DeleteDeck_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), testUserID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{deleteErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), testUserID, 1)

	assert.Error(t, err)
}

func TestService_GetDeckCards_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}}
	service := NewService(repo)

	_, err := service.GetDeckCards(context.Background(), testUserID, 1, "updated", true)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetDeckCards_ReturnsCardsFromRepository(t *testing.T) {
	expected := []DeckCard{
		{ID: 1, Name: "Black Lotus", SetCode: "lea"},
		{ID: 2, Name: "Lightning Bolt", SetCode: "2xm"},
	}
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, getDeckCards: expected}
	service := NewService(repo)

	result, err := service.GetDeckCards(context.Background(), testUserID, 1, "updated", true)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetDeckCards_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetDeckCards(context.Background(), otherUserID, 999, "updated", true)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_GetDeckCards_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, getDeckCardsErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetDeckCards(context.Background(), testUserID, 1, "updated", true)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_PutCardInDeck_ChecksDeckOwnershipBeforeLinking(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), testUserID, 1, 4)

	assert.NoError(t, err)
}

func TestService_PutCardInDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), otherUserID, 1, 4)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_PutCardInDeck_PropagatesCardNotFoundError(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, linkErr: ErrCardNotFound}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), testUserID, 1, 9999)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestService_PutCardInDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, linkErr: errors.New("insert failed")}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), testUserID, 1, 4)

	assert.Error(t, err)
}

func TestService_RemoveCardFromDeck_ChecksDeckOwnershipBeforeUnlinking(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), testUserID, 1, 4)

	assert.NoError(t, err)
}

func TestService_RemoveCardFromDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), otherUserID, 1, 4)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_RemoveCardFromDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, unlinkErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), testUserID, 1, 4)

	assert.Error(t, err)
}

func TestService_AddPendingCard_SetsTheDeck(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 3}}
	service := NewService(repo)

	created, err := service.AddPendingCard(context.Background(), testUserID, 3, PendingCard{Name: "Sol Ring", Quantity: 1})

	require.NoError(t, err)
	assert.Equal(t, 7, created.ID)
	assert.Equal(t, 3, repo.createdPending.DeckID)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_PendingCards_RequireTheDeckToBelongToTheUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetPendingCards(context.Background(), testUserID, 3)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = service.AddPendingCard(context.Background(), testUserID, 3, PendingCard{Name: "Sol Ring"})
	assert.ErrorIs(t, err, ErrNotFound)

	err = service.RemovePendingCard(context.Background(), testUserID, 3, 1)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Zero(t, repo.lastPendingID)
}

func TestService_RemovePendingCard_PropagatesNotFound(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 3}, deletePendingErr: ErrPendingCardNotFound}
	service := NewService(repo)

	err := service.RemovePendingCard(context.Background(), testUserID, 3, 9)

	assert.ErrorIs(t, err, ErrPendingCardNotFound)
	assert.Equal(t, 9, repo.lastPendingID)
}

func TestService_UpdateDeck_SetsAPendingCommander(t *testing.T) {
	commanderID := 4
	repo := &fakeRepository{
		findByIDDeck: Deck{ID: 1, Name: "Deck", Format: "commander", CommanderID: &commanderID},
		pending:      []PendingCard{{ID: 7, DeckID: 1, Name: "Atraxa"}},
	}
	service := NewService(repo)

	pendingID := 7
	result, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{CommanderPendingID: &pendingID})

	require.NoError(t, err)
	require.NotNil(t, result.CommanderPendingID)
	assert.Equal(t, 7, *result.CommanderPendingID)
	assert.Nil(t, result.CommanderID)
}

func TestService_UpdateDeck_RejectsAPendingCommanderFromAnotherDeck(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, pending: []PendingCard{{ID: 7, DeckID: 1}}}
	service := NewService(repo)

	pendingID := 8
	_, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{CommanderPendingID: &pendingID})

	assert.ErrorIs(t, err, ErrCommanderNotFound)
}

func TestService_UpdateDeck_RealCommanderReplacesThePendingOne(t *testing.T) {
	pendingID := 7
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1, CommanderPendingID: &pendingID}}
	service := NewService(repo)

	commanderID := 4
	result, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{CommanderID: &commanderID})

	require.NoError(t, err)
	assert.Nil(t, result.CommanderPendingID)
	require.NotNil(t, result.CommanderID)
	assert.Equal(t, 4, *result.CommanderID)
}

func TestService_UpdateDeck_ClearCommanderClearsBoth(t *testing.T) {
	pendingID := 7
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1, CommanderPendingID: &pendingID}}
	service := NewService(repo)

	result, err := service.UpdateDeck(context.Background(), testUserID, 1, updateDeckRequest{ClearCommanderID: true})

	require.NoError(t, err)
	assert.Nil(t, result.CommanderPendingID)
	assert.Nil(t, result.CommanderID)
}

func TestService_PromotePendingCommander(t *testing.T) {
	pendingID := 7
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1, CommanderPendingID: &pendingID}}
	service := NewService(repo)

	require.NoError(t, service.PromotePendingCommander(context.Background(), testUserID, 1, 7, 42))

	require.NotNil(t, repo.updatedDeck.CommanderID)
	assert.Equal(t, 42, *repo.updatedDeck.CommanderID)
	assert.Nil(t, repo.updatedDeck.CommanderPendingID)
}

func TestService_PromotePendingCommander_IgnoresOtherPendingCards(t *testing.T) {
	pendingID := 7
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1, CommanderPendingID: &pendingID}}
	service := NewService(repo)

	require.NoError(t, service.PromotePendingCommander(context.Background(), testUserID, 1, 8, 42))

	assert.Zero(t, repo.updatedDeck.ID)
}

func TestService_GetSharedDeck_ReturnsOwnerAndDeck(t *testing.T) {
	repo := &fakeRepository{sharedOwnerID: otherUserID, sharedDeck: Deck{ID: 4, Name: "Shared", ShareID: "abc"}}
	service := NewService(repo)

	ownerID, d, err := service.GetSharedDeck(context.Background(), "abc")

	require.NoError(t, err)
	assert.Equal(t, "abc", repo.lastShareID)
	assert.Equal(t, otherUserID, ownerID)
	assert.Equal(t, 4, d.ID)
}

func TestService_GetSharedDeck_PropagatesNotFound(t *testing.T) {
	repo := &fakeRepository{sharedErr: ErrNotFound}
	service := NewService(repo)

	_, _, err := service.GetSharedDeck(context.Background(), "abc")

	assert.ErrorIs(t, err, ErrNotFound)
}
