-- +goose Up
-- An address is the same account whatever its case: store it trimmed and lowercased and make the
-- database refuse two accounts for one lowercased address. Deploy with the code that normalises emails.
-- +goose StatementBegin
DO $$
DECLARE
    collisions integer;
BEGIN
    SELECT count(*) INTO collisions FROM (
        SELECT 1 FROM tamiyo.users GROUP BY lower(btrim(email)) HAVING count(*) > 1
    ) AS duplicated;
    IF collisions > 0 THEN
        RAISE EXCEPTION 'cannot lowercase users.email: % address(es) collide case-insensitively, merge or delete those accounts first', collisions;
    END IF;
END
$$;
-- +goose StatementEnd

UPDATE tamiyo.users SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

ALTER TABLE tamiyo.users DROP CONSTRAINT users_email_key;
CREATE UNIQUE INDEX users_email_lower_key ON tamiyo.users (lower(email));

-- +goose Down
-- Lowercased emails stay lowercased, which the previous code accepts.
DROP INDEX tamiyo.users_email_lower_key;
ALTER TABLE tamiyo.users ADD CONSTRAINT users_email_key UNIQUE (email);
