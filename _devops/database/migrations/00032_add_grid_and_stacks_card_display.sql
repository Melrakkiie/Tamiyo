-- +goose Up
-- Two more ways to show card lists: "grid", card images in rows that
-- overlap so only the top of each row shows, and "stacks", columns of
-- cards where only each card's title shows.
ALTER TABLE tamiyo.user_preferences DROP CONSTRAINT IF EXISTS user_preferences_card_display_check;
ALTER TABLE tamiyo.user_preferences ADD CONSTRAINT user_preferences_card_display_check
    CHECK (card_display IN ('text', 'images', 'grid', 'stacks'));
ALTER TABLE tamiyo.deck_view_settings DROP CONSTRAINT IF EXISTS deck_view_settings_card_display_check;
ALTER TABLE tamiyo.deck_view_settings ADD CONSTRAINT deck_view_settings_card_display_check
    CHECK (card_display IN ('text', 'images', 'grid', 'stacks'));

-- +goose Down
UPDATE tamiyo.user_preferences SET card_display = 'images' WHERE card_display IN ('grid', 'stacks');
UPDATE tamiyo.deck_view_settings SET card_display = 'images' WHERE card_display IN ('grid', 'stacks');
ALTER TABLE tamiyo.user_preferences DROP CONSTRAINT IF EXISTS user_preferences_card_display_check;
ALTER TABLE tamiyo.user_preferences ADD CONSTRAINT user_preferences_card_display_check
    CHECK (card_display IN ('text', 'images'));
ALTER TABLE tamiyo.deck_view_settings DROP CONSTRAINT IF EXISTS deck_view_settings_card_display_check;
ALTER TABLE tamiyo.deck_view_settings ADD CONSTRAINT deck_view_settings_card_display_check
    CHECK (card_display IN ('text', 'images'));
