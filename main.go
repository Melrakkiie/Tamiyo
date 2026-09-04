package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type card struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Scryfall_id      string `json:"scryfall_id"`
	Set_code         string `json:"set_code"`
	Collector_number int    `json:"collector_number"`
	Foil             bool   `json:"foil"`
	Binder_name      string `json:"binder_name"`
	Binder_type      string `json:"binder_type"`
	Added            string `json:"added"`
}

var cards = []card{
	{
		ID:               "1",
		Name:             "Black Lotus",
		Scryfall_id:      "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd",
		Set_code:         "lea",
		Collector_number: 232,
		Foil:             false,
		Binder_name:      "Vintage Collection",
		Binder_type:      "binder",
		Added:            "2024-01-15T10:30:00Z",
	},
	{
		ID:               "2",
		Name:             "Lightning Bolt",
		Scryfall_id:      "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d",
		Set_code:         "2xm",
		Collector_number: 129,
		Foil:             true,
		Binder_name:      "Red Deck Wins",
		Binder_type:      "deckbox",
		Added:            "2024-02-20T14:45:00Z",
	},
	{
		ID:               "3",
		Name:             "Sol Ring",
		Scryfall_id:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		Set_code:         "cmr",
		Collector_number: 322,
		Foil:             false,
		Binder_name:      "Commander Staples",
		Binder_type:      "binder",
		Added:            "2024-03-05T09:15:00Z",
	},
	{
		ID:               "4",
		Name:             "Tarmogoyf",
		Scryfall_id:      "3a1b2c3d-4e5f-6789-0abc-def123456789",
		Set_code:         "mm3",
		Collector_number: 156,
		Foil:             true,
		Binder_name:      "Modern Staples",
		Binder_type:      "box",
		Added:            "2024-04-10T16:20:00Z",
	},
}

// getCards responds with the list of all cards as JSON.
func getCards(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, cards)
}

func main() {
	router := gin.Default()
	router.GET("/cards", getCards)

	router.Run(":8080")
}
