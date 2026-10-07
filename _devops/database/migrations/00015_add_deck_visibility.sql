-- +goose Up
-- Who may see a deck once decks can be shared: only its owner (private),
-- anyone with its link (unlisted), or anyone, listed when browsing (public).
ALTER TABLE tamiyo.deck
    ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'unlisted'
        CHECK (visibility IN ('public', 'unlisted', 'private'));

-- +goose Down
ALTER TABLE tamiyo.deck DROP COLUMN IF EXISTS visibility;
