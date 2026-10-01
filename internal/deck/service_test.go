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

	getDeckCards    []DeckCard
	getDeckCardsErr error

	linkErr   error
	unlinkErr error
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

func (f *fakeRepository) FindCardsByDeckID(ctx context.Context, userID string, id int) ([]DeckCard, error) {
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

	_, err := service.GetDeckCards(context.Background(), testUserID, 1)

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

	result, err := service.GetDeckCards(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetDeckCards_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetDeckCards(context.Background(), otherUserID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_GetDeckCards_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findByIDDeck: Deck{ID: 1}, getDeckCardsErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetDeckCards(context.Background(), testUserID, 1)

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
