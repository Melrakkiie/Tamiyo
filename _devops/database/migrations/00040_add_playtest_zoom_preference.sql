-- +goose Up
-- The card size the front's deck playtest opens with, as a percentage of the
-- normal size, so each player finds the zoom they last picked.
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS playtest_zoom integer NOT NULL DEFAULT 100 CHECK (playtest_zoom BETWEEN 50 AND 200);

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP COLUMN IF EXISTS playtest_zoom;
