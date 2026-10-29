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

**Board**:
One of the three parts of a deck: the main board, the sideboard and the cards being considered. Each card of a deck is on exactly one board.
_Avoid_: Zone, section

**Deck entry**:
The copies of one printing, with the same finish and proxy status, on one board of a deck, shown as a single pile.
_Avoid_: Pile, stack, line

**Main board**:
The cards of a deck outside the sideboard and the cards being considered, commander and partner included.
_Avoid_: Main deck, mainboard, maindeck

**Card count**:
The number of copies in a deck's main board, owned or pending, proxies and commanders included.
_Avoid_: Deck size, total cards

### Deck statistics

**Total mana value**:
The sum of the mana values of the main board's nonland cards, each counted once per copy. Shown as "Coût de mana total" in the interface.
_Avoid_: Total mana cost, total CMC

**Combo**:
A set of cards that together produce a notable result (an infinite loop, a win), as listed by Commander Spellbook. A deck has a combo only when every one of its cards is in the main board, with any card the combo needs as commander set as the deck's commander.
_Avoid_: Synergy, interaction, almost-combo

**Game changer**:
A card on Wizards of the Coast's Commander list of cards that strongly raise a deck's power, as flagged by Scryfall. Counted in a commander deck's statistics, once per distinct card in the main board. Shown as "Game changer" in the interface.
_Avoid_: Staple, power card, banned-list card

### Deck history

**Deck history**:
The day-by-day record of the cards added to and removed from each board of a deck, counted by card name: changing a card's printing, finish or proxy status, or committing a pending card, changes nothing. Starts when the deck is created; seen by whoever can see the deck. Shown as "Historique" in the interface.
_Avoid_: Changelog, log, revisions

**Addition**:
A card name whose number of copies on a board grew over a day, by that many copies; a move between boards is a removal from one and an addition to the other. Shown as "Ajout" in the interface.
_Avoid_: Entry (a deck entry is a pile), in

**Removal**:
A card name whose number of copies on a board shrank over a day, by that many copies. Shown as "Retrait" in the interface.
_Avoid_: Exit, out, cut

### Languages

**Interface language**:
The language a user reads Tamiyo in, chosen by the user; it also sets how numbers and dates are written and how names are sorted.
_Avoid_: Locale, app language, site language

**Card text language**:
The language a card's printed name and rules text are shown in: the interface language unless the user picks the other one for that card.
_Avoid_: Card language, translation
