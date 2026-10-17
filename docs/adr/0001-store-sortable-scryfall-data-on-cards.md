# Store sortable Scryfall data on each card

`mana_value`, `colors`, `card_type` and `color_identity` are copied onto each card at creation or import, instead of being fetched from Scryfall on read. The collection, storage and deck lists sort and paginate in SQL (`ORDER BY ... LIMIT/OFFSET`); a value that isn't in the database would force loading the whole filtered result to sort it in Go, and would tie every list request to Scryfall's availability. Cards that miss these values are caught up by `POST /cards/refresh-details`.
