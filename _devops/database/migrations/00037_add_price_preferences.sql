-- +goose Up
-- Whether the app shows card prices and collection, storage and deck values,
-- and in which currency: euros (Cardmarket) or US dollars (TCGplayer).
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS show_prices boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS price_currency text NOT NULL DEFAULT 'eur' CHECK (price_currency IN ('eur', 'usd'));

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP COLUMN IF EXISTS price_currency,
    DROP COLUMN IF EXISTS show_prices;
