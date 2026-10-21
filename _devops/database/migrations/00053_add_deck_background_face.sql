-- +goose Up
-- The deck's art names a face of a printing: its front, or the back of a
-- double-faced or reversible card. A meld card's back is another printing (the
-- melded card), stored as background_scryfall_id with the front face. Without a
-- chosen printing the face stays front.
ALTER TABLE tamiyo.deck
    ADD COLUMN background_face text NOT NULL DEFAULT 'front',
    ADD CONSTRAINT deck_background_face CHECK (background_face IN ('front', 'back')),
    ADD CONSTRAINT deck_background_face_needs_printing CHECK (
        background_face = 'front' OR background_scryfall_id IS NOT NULL
    );

-- +goose Down
ALTER TABLE tamiyo.deck
    DROP CONSTRAINT deck_background_face_needs_printing,
    DROP CONSTRAINT deck_background_face,
    DROP COLUMN background_face;
