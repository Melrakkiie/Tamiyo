# Tamiyo

A Magic: The Gathering collection manager: the cards a user owns, where they are stored, and the decks built from them.

## Language

### Copies

**Finish**:
The surface of a copy: regular or foil, among those its printing comes in. A physical fact of the copy: changing it means taking another copy.
_Avoid_: Treatment, foiling

**Foil**:
The shiny finish, Scryfall's etched finish included. Shown as "foil" in the interface.
_Avoid_: Etched (as a finish of its own), holo, shiny

### Card images

**Printing**:
One edition of a card, identified by its Scryfall id.
_Avoid_: Version, edition (for the whole printing)

**Front face**:
The face of a printing shown by default. Shown as "recto" in the interface.
_Avoid_: Front side, recto (in code and docs)

**Back face**:
The second face of a double-faced or reversible printing; for a meld card, the melded card it forms, which is a printing of its own. Shown as "verso" in the interface.
_Avoid_: Back side, verso (in code and docs), reverse

**Flipped card**:
A card the interface currently shows on its back face.
_Avoid_: Turned card, reversed card

### Decks

**Deck art**:
The art of one face of one printing, chosen to represent a deck; the commander's front face when the owner chose none.
_Avoid_: Deck image, background, cover

### Deck statistics

**Total mana value**:
The sum of the mana values of the main board's nonland cards, each counted once per copy. Shown as "Coût de mana total" in the interface.
_Avoid_: Total mana cost, total CMC

**Combo**:
A set of cards that together produce a notable result (an infinite loop, a win), as listed by Commander Spellbook. A deck has a combo only when every one of its cards is in the main board, with any card the combo needs as commander set as the deck's commander.
_Avoid_: Synergy, interaction, almost-combo
