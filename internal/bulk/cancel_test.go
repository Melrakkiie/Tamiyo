package bulk

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/storage"
)

// Imports written with the grouped operation are rolled back when the request is cancelled:
// an error, nothing created, no summary.
func TestGroupedImports_RollBackWhenTheRequestIsCancelled(t *testing.T) {
	boxes := func() *fakeStorageService {
		return &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}}}
	}
	imports := map[string]func(ctx context.Context, cards *fakeCardService) (Summary, error){
		"moxfield collection": func(ctx context.Context, cards *fakeCardService) (Summary, error) {
			csv := "Count,Name,Edition,Foil,Collector Number\n5,Sol Ring,sld,,1011\n"
			resolver := &fakeResolver{resolved: map[string]ResolvedCard{resolveKey("sld", "1011"): {ScryfallID: solRingID, Name: "Sol Ring"}}}
			return NewService(cards, boxes(), &fakeDeckService{}, resolver).
				ImportMoxfieldCollection(ctx, testUserID, 4, strings.NewReader(csv))
		},
		"tamiyo": func(ctx context.Context, cards *fakeCardService) (Summary, error) {
			file := `{"tamiyo": 1, "kind": "collection", "cards": [
				{"name": "Sol Ring", "scryfall_id": "` + solRingSLD + `", "set_code": "sld", "collector_number": "1011", "quantity": 5, "storage": "Neuf"}
			]}`
			return NewService(cards, &fakeStorageService{}, &fakeDeckService{}, tamiyoResolver()).
				ImportTamiyoCollection(ctx, testUserID, nil, strings.NewReader(file))
		},
		"manabox": func(ctx context.Context, cards *fakeCardService) (Summary, error) {
			csv := "Binder Name,Binder Type,Name,Set code,Scryfall ID,Collector number,Foil,Quantity\n" +
				"Atraxa Deck,deck,Sol Ring,CMM,aaaaaaaa-0000-0000-0000-000000000000,123,,5\n"
			return NewService(cards, &fakeStorageService{}, &fakeDeckService{}, &fakeResolver{}).
				ImportManaBox(ctx, testUserID, nil, strings.NewReader(csv))
		},
		"card list": func(ctx context.Context, cards *fakeCardService) (Summary, error) {
			return NewService(cards, boxes(), &fakeDeckService{}, cardListResolver()).
				ImportCardList(ctx, testUserID, ptr(4), strings.NewReader("5 Sol Ring (SLD) 1011\n"))
		},
	}

	for name, run := range imports {
		t.Run(name, func(t *testing.T) {
			cards := &fakeCardService{}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			summary, err := run(ctx, cards)

			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, cards.created)
			assert.Equal(t, Summary{}, summary)
		})
	}
}
