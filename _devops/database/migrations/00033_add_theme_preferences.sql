-- +goose Up
-- Whether the app shows its day or its night colors: always day, always
-- night, or by the time of day ("auto"), night running from night_starts_at
-- to night_ends_at (hours, 0 to 23, in the user's own time zone).
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS theme text NOT NULL DEFAULT 'auto' CHECK (theme IN ('day', 'night', 'auto')),
    ADD COLUMN IF NOT EXISTS night_starts_at smallint NOT NULL DEFAULT 22 CHECK (night_starts_at BETWEEN 0 AND 23),
    ADD COLUMN IF NOT EXISTS night_ends_at smallint NOT NULL DEFAULT 6 CHECK (night_ends_at BETWEEN 0 AND 23),
    ADD CONSTRAINT user_preferences_night_hours_check CHECK (night_starts_at <> night_ends_at);

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_night_hours_check,
    DROP COLUMN IF EXISTS night_ends_at,
    DROP COLUMN IF EXISTS night_starts_at,
    DROP COLUMN IF EXISTS theme;
