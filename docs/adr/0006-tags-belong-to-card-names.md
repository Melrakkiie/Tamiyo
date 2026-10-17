# Deck tags belong to a normalized card name, not to a copy

Every copy and printing of a card in a deck, pending ones included, shares its tags, so a reprint or a foil swap never loses them. Names are normalized (lowercase, `//` and `/` treated alike, spaces collapsed) by the same rule in Go (`deck.CardNameKey`) and in SQL; change both together. Deck comparison and ownership (`GET /deck/:id/ownership`) also compare by normalized name, whatever the printing.
