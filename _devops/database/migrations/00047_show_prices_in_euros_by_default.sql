-- +goose Up
-- The app shows card prices in euros by default. Existing accounts are
-- switched to it as well, since a saved false cannot be told apart from the
-- old default.
ALTER TABLE tamiyo.user_preferences ALTER COLUMN show_prices SET DEFAULT true;
UPDATE tamiyo.user_preferences SET show_prices = true, price_currency = 'eur';

-- +goose Down
ALTER TABLE tamiyo.user_preferences ALTER COLUMN show_prices SET DEFAULT false;
