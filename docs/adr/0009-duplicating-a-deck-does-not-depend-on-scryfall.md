# Duplicating a deck does not depend on Scryfall

Duplicating a deck copies its content straight from the database, inside the deck module, in the same transaction that creates the new deck. It never calls Scryfall, and it never goes through the file import. ADR-0003 covers imports, which resolve decklist lines on Scryfall where the format carries no `scryfall_id`; every card of a deck being duplicated already has its printing and details stored, so nothing needs resolving.

The earlier implementation exported the deck as a Tamiyo file and re-imported it, which made duplication fail with a 502 when Scryfall was down and left an empty deck to clean up when the import failed. Do not route duplication back through the import: a purely local action must not fail on a third-party outage, and the new deck must exist entirely or not at all.
