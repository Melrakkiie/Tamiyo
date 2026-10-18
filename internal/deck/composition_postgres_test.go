//go:build integration

package deck

import (
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type collectionCard struct {
	name       string
	scryfallID string
	foil       bool
	proxy      bool
	storageID  *int
}

func (f contentFixture) collection(t *testing.T) []collectionCard {
	t.Helper()
	var rows []struct {
		Name       string `db:"name"`
		ScryfallID string `db:"scryfall_id"`
		Foil       bool   `db:"foil"`
		Proxy      bool   `db:"proxy"`
		StorageID  *int   `db:"storage_id"`
	}
	require.NoError(t, f.db.Select(&rows, `
		SELECT name, scryfall_id, foil, proxy, storage_id FROM tamiyo.cards WHERE user_id = $1 ORDER BY id
	`, f.userID))
	cards := make([]collectionCard, 0, len(rows))
	for _, r := range rows {
		cards = append(cards, collectionCard{r.Name, r.ScryfallID, r.Foil, r.Proxy, r.StorageID})
	}
	return cards
}

// deckState is everything a composition operation may change: the deck row
// (commander slots, updated date), its owned and pending cards, and the
// owner's collection.
type deckState struct {
	content    DeckContent
	collection []collectionCard
}

func (f contentFixture) state(t *testing.T) deckState {
	t.Helper()
	return deckState{content: f.content(t), collection: f.collection(t)}
}

// assertRolledBack runs a composition operation that must fail and checks
// that it left the deck and the collection exactly as they were.
func (f contentFixture) assertRolledBack(t *testing.T, operation func() error) {
	t.Helper()
	before := f.state(t)

	err := operation()

	require.Error(t, err)
	assert.Equal(t, before, f.state(t))
}

// failNthWrite makes the nth row written to table by op (INSERT, UPDATE or
// DELETE) raise an error, to fail a composition operation midway. The
// counter is a sequence, which a rollback doesn't reset.
func failNthWrite(t *testing.T, db *sqlx.DB, table string, op string, nth int) {
	t.Helper()
	statements := []string{
		`CREATE SEQUENCE tamiyo.injected_failure_count`,
		fmt.Sprintf(`CREATE FUNCTION tamiyo.injected_failure() RETURNS TRIGGER AS $$
		BEGIN
			IF nextval('tamiyo.injected_failure_count') = %d THEN
				RAISE EXCEPTION 'injected failure on %s %s #%d';
			END IF;
			RETURN COALESCE(NEW, OLD);
		END;
		$$ LANGUAGE plpgsql`, nth, op, table, nth),
		fmt.Sprintf(`CREATE TRIGGER injected_failure BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION tamiyo.injected_failure()`, op, table),
	}
	for _, statement := range statements {
		_, err := db.Exec(statement)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, err := db.Exec(fmt.Sprintf(`
			DROP TRIGGER IF EXISTS injected_failure ON %s;
			DROP FUNCTION IF EXISTS tamiyo.injected_failure();
			DROP SEQUENCE IF EXISTS tamiyo.injected_failure_count;
		`, table))
		require.NoError(t, err)
	})
}
