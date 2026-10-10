-- +goose Up
-- Folders a user sorts their decks into. A folder can sit inside another
-- folder of the same user, to any depth. Deleting a folder is handled by the
-- application, which first moves its decks and subfolders to its parent;
-- the cascade only matters when the whole account goes away.
CREATE TABLE IF NOT EXISTS tamiyo.deck_folders
(
    id        SERIAL PRIMARY KEY,
    user_id   uuid                     NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    parent_id integer                  REFERENCES tamiyo.deck_folders(id) ON DELETE CASCADE,
    name      text                     NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 100),
    collapsed boolean                  NOT NULL DEFAULT false,
    added     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX IF NOT EXISTS deck_folders_user_idx ON tamiyo.deck_folders (user_id);
CREATE INDEX IF NOT EXISTS deck_folders_parent_idx ON tamiyo.deck_folders (parent_id);

CREATE TRIGGER update_deck_folders_modtime
    BEFORE UPDATE ON tamiyo.deck_folders
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_deck_folder_parent_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.deck_folders
            WHERE id = NEW.parent_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'folder % does not belong to user %', NEW.parent_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
-- +goose StatementEnd

CREATE TRIGGER check_deck_folder_parent_ownership
    BEFORE INSERT OR UPDATE ON tamiyo.deck_folders
    FOR EACH ROW EXECUTE FUNCTION check_deck_folder_parent_ownership();

-- The folder a deck sits in (NULL at the root), and whether its owner
-- marked it as a favorite.
ALTER TABLE tamiyo.deck ADD COLUMN IF NOT EXISTS folder_id integer REFERENCES tamiyo.deck_folders(id) ON DELETE SET NULL;
ALTER TABLE tamiyo.deck ADD COLUMN IF NOT EXISTS favorite boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_deck_folder_id ON tamiyo.deck (folder_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_deck_folder_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.folder_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.deck_folders
            WHERE id = NEW.folder_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'folder % does not belong to user %', NEW.folder_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
-- +goose StatementEnd

CREATE TRIGGER check_deck_folder_ownership
    BEFORE INSERT OR UPDATE ON tamiyo.deck
    FOR EACH ROW EXECUTE FUNCTION check_deck_folder_ownership();

-- Filing a deck away or marking it as a favorite doesn't change the deck
-- itself, so it leaves its updated timestamp alone.
DROP TRIGGER IF EXISTS update_deck_modtime ON tamiyo.deck;
CREATE TRIGGER update_deck_modtime
    BEFORE UPDATE ON tamiyo.deck
    FOR EACH ROW
    WHEN (OLD.folder_id IS NOT DISTINCT FROM NEW.folder_id AND OLD.favorite = NEW.favorite)
    EXECUTE FUNCTION update_modified_column();

-- +goose Down
DROP TRIGGER IF EXISTS update_deck_modtime ON tamiyo.deck;
CREATE TRIGGER update_deck_modtime
    BEFORE UPDATE ON tamiyo.deck
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

DROP TRIGGER IF EXISTS check_deck_folder_ownership ON tamiyo.deck;
DROP FUNCTION IF EXISTS check_deck_folder_ownership();
ALTER TABLE tamiyo.deck DROP COLUMN IF EXISTS favorite;
ALTER TABLE tamiyo.deck DROP COLUMN IF EXISTS folder_id;
DROP TABLE IF EXISTS tamiyo.deck_folders;
DROP FUNCTION IF EXISTS check_deck_folder_parent_ownership();
