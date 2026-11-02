package bulk

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/storage"
)

// cancelAfterFirstCard cancels the request after one copy is created and fails
// every later insert with the context error, like the database driver would.
type cancelAfterFirstCard struct {
	fakeCardService
	ctx    context.Context
	cancel context.CancelFunc
}

func newCancelAfterFirstCard() *cancelAfterFirstCard {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelAfterFirstCard{ctx: ctx, cancel: cancel}
}

func (c *cancelAfterFirstCard) CreateCard(ctx context.Context, userID string, in card.Card) (card.Card, error) {
	if err := ctx.Err(); err != nil {
		return card.Card{}, err
	}
	created, err := c.fakeCardService.CreateCard(ctx, userID, in)
	c.cancel()
	return created, err
}

func TestImports_StopWhenTheRequestIsCancelled(t *testing.T) {
	boxes := func() *fakeStorageService {
		return &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}}}
	}
	imports := map[string]func(cards *cancelAfterFirstCard) (Summary, error){
		"manabox": func(cards *cancelAfterFirstCard) (Summary, error) {
			csv := "Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity\n" +
				"Main Binder,binder,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,5\n"
			return NewService(cards, boxes(), &fakeDeckService{}, &fakeResolver{}).
				ImportManaBox(cards.ctx, testUserID, ptr(4), strings.NewReader(csv))
		},
		"moxfield collection": func(cards *cancelAfterFirstCard) (Summary, error) {
			csv := "Count,Name,Edition,Foil,Collector Number\n5,Sol Ring,sld,,1011\n"
			resolver := &fakeResolver{resolved: map[string]ResolvedCard{resolveKey("sld", "1011"): {ScryfallID: solRingID, Name: "Sol Ring"}}}
			return NewService(cards, boxes(), &fakeDeckService{}, resolver).
				ImportMoxfieldCollection(cards.ctx, testUserID, 4, strings.NewReader(csv))
		},
		"card list": func(cards *cancelAfterFirstCard) (Summary, error) {
			return NewService(cards, boxes(), &fakeDeckService{}, cardListResolver()).
				ImportCardList(cards.ctx, testUserID, ptr(4), strings.NewReader("5 Sol Ring (SLD) 1011\n"))
		},
		"tamiyo": func(cards *cancelAfterFirstCard) (Summary, error) {
			file := `{"tamiyo": 1, "kind": "collection", "cards": [
				{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "set_code": "sld", "collector_number": "1011", "quantity": 5}
			]}`
			return NewService(cards, boxes(), &fakeDeckService{}, tamiyoResolver()).
				ImportTamiyoCollection(cards.ctx, testUserID, ptr(4), strings.NewReader(file))
		},
	}

	for name, run := range imports {
		t.Run(name, func(t *testing.T) {
			cards := newCancelAfterFirstCard()

			summary, err := run(cards)

			require.ErrorIs(t, err, context.Canceled)
			assert.Len(t, cards.created, 1)
			assert.Equal(t, 1, summary.CardsCreated)
			assert.Zero(t, summary.CardsSkipped)
			assert.Empty(t, summary.Warnings)
		})
	}
}
