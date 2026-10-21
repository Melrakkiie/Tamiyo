# Deck art names a printing and a face

The deck art is stored as a Scryfall printing id plus a face (`background_face`: `front` or `back`), not as an image URL. Clients build the art (`art_crop`) and the artist credit from the printing, so storing the URL would lose the credit, tie the database to Scryfall's image hosting, and let any URL in. The API cannot check that a printing has a back face, since it keeps no Scryfall face data; clients fall back to the front when it doesn't.

A meld card is the exception: its back is the melded card, a printing with its own Scryfall id, so picking it stores that id with `front`, never the meld half with `back`. Do not "fix" this into `back`: the half's own Scryfall data has no back face to show.
