# Switching a deck card to foil changes the copy, not an attribute

A card's finish is a physical fact of that copy, so `POST /deck/:id/switch-foil` swaps the card for another copy of the same printing in the other finish: first one in no deck, then one used by another deck, and otherwise a pending card. A pending card that finds a copy becomes owned again. The commander stays commander.
