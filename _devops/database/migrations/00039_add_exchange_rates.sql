-- +goose Up
-- Exchange rates of the European Central Bank, as units of a currency per
-- euro, refreshed in the background. Values convert a card's dollar price to
-- euros when Scryfall has no euro price for it.
CREATE TABLE IF NOT EXISTS tamiyo.exchange_rates (
    currency text PRIMARY KEY,
    per_eur numeric(12, 6) NOT NULL CHECK (per_eur > 0),
    rate_date date NOT NULL,
    refreshed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.exchange_rates;
