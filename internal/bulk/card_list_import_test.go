package bulk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/storage"
)

func cardListResolver() *fakeResolver {
	resolver := plainListResolver()
	for key, value := range deckImportResolver().resolved {
		resolver.resolved[key] = value
	}
	return resolver
}

func TestImportCardList_CreatesOneCardPerCopyInTheStorage(t *testing.T) {
	cards := &fakeCardService{}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}}}
	svc := NewService(cards, storages, &fakeDeckService{}, cardListResolver())

	summary, err := svc.ImportCardList(context.Background(), testUserID, ptr(4), strings.NewReader("Deck\n2 Sol Ring (SLD) 1011 *F*\n1 Fire / Ice\n"))

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsCreated)
	assert.Equal(t, 0, summary.CardsSkipped)
	require.Len(t, cards.created, 3)

	solRing := cards.created[0]
	assert.Equal(t, "Sol Ring", solRing.Name)
	assert.Equal(t, solRingID, solRing.ScryfallID)
	assert.Equal(t, "SLD", solRing.SetCode)
	assert.Equal(t, "1011", solRing.CollectorNumber)
	assert.True(t, solRing.Foil)
	assert.Equal(t, 1.0, solRing.ManaValue)
	require.NotNil(t, solRing.CardType)
	assert.Equal(t, "Artifact", *solRing.CardType)
	require.NotNil(t, solRing.StorageID)
	assert.Equal(t, 4, *solRing.StorageID)

	fireIce := cards.created[2]
	assert.Equal(t, "Fire // Ice", fireIce.Name)
	assert.Equal(t, "mh2", fireIce.SetCode)
	assert.Equal(t, "290", fireIce.CollectorNumber)
	assert.False(t, fireIce.Foil)
}

func TestImportCardList_WithoutStorage(t *testing.T) {
	cards := &fakeCardService{}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, cardListResolver())

	summary, err := svc.ImportCardList(context.Background(), testUserID, nil, strings.NewReader("1 Sol Ring\n"))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Nil(t, cards.created[0].StorageID)
	assert.Equal(t, "cmm", cards.created[0].SetCode)
}

func TestImportCardList_SkipsUnknownCards(t *testing.T) {
	cards := &fakeCardService{}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, cardListResolver())

	summary, err := svc.ImportCardList(context.Background(), testUserID, nil, strings.NewReader("3 Not A Real Card\n1 Sol Ring\n"))

	require.NoError(t, err)
	assert.Equal(t, 1, summary.CardsCreated)
	assert.Equal(t, 3, summary.CardsSkipped)
	require.Len(t, summary.Warnings, 1)
	assert.Contains(t, summary.Warnings[0], "Not A Real Card")
}

func TestImportCardList_WarnsWhenACardCannotBeCreated(t *testing.T) {
	cards := &fakeCardService{createErr: errors.New("insert failed")}
	svc := NewService(cards, &fakeStorageService{}, &fakeDeckService{}, cardListResolver())

	summary, err := svc.ImportCardList(context.Background(), testUserID, nil, strings.NewReader("2 Sol Ring\n"))

	require.NoError(t, err)
	assert.Equal(t, 0, summary.CardsCreated)
	assert.Equal(t, 2, summary.CardsSkipped)
	assert.Len(t, summary.Warnings, 2)
}

func TestImportCardList_Errors(t *testing.T) {
	cases := map[string]struct {
		storages  *fakeStorageService
		resolver  *fakeResolver
		storageID *int
		list      string
		want      error
	}{
		"unreadable list":  {&fakeStorageService{}, cardListResolver(), nil, "not a card line\n", ErrInvalidFile},
		"unknown storage":  {&fakeStorageService{}, cardListResolver(), ptr(9), "1 Sol Ring\n", ErrTargetStorageNotFound},
		"scryfall is down": {&fakeStorageService{}, &fakeResolver{err: errors.New("down")}, nil, "1 Sol Ring\n", ErrScryfallUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cards := &fakeCardService{}
			svc := NewService(cards, tc.storages, &fakeDeckService{}, tc.resolver)

			_, err := svc.ImportCardList(context.Background(), testUserID, tc.storageID, strings.NewReader(tc.list))

			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, cards.created)
		})
	}
}
