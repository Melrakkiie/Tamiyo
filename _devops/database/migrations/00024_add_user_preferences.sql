-- +goose Up
-- Display preferences of each user, kept apart from tamiyo.users so the
-- account itself only changes when its identity does. A user without a row
-- has the default preferences.
CREATE TABLE IF NOT EXISTS tamiyo.user_preferences
(
    user_id                  uuid PRIMARY KEY REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    show_collection_in_decks boolean NOT NULL DEFAULT true
);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.user_preferences;
