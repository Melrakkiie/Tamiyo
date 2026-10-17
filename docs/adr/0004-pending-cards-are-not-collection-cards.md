# A wanted card is a pending card, not a card of the collection

The collection only holds cards the user physically owns. A deck can be built before owning its cards through `deck_pending_cards` (one row per printing, finish and board, whose quantity grows), commander included (`deck.commander_pending_id`); committing pending cards creates them in the collection and puts them in the deck at once. The reverse holds too: deleting a card from the collection doesn't shrink its decks, a trigger turns it back into a pending card in each of them.
