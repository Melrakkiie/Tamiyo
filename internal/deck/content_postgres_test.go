//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentDeckID = "00000000-0000-0000-0000-000000000001"

type printing struct {
	name, scryfallID, setCode, collectorNumber string
}

var (
	bolt   = printing{"Lightning Bolt", "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", "2xm", "129"}
	atraxa = printing{"Atraxa, Praetors' Voice", "d0d33d52-3d28-4635-b985-51e126289259", "2xm", "190"}
	tymna  = printing{"Tymna the Weaver", "4bd1b9c1-0d47-4a2b-8d4f-5f3a8c6c0c11", "c16", "48"}
	island = printing{"Island", "fc3f6a8f-0b5e-4b5f-9a1a-1d4b0e3c4c2f", "neo", "294"}
)

type copyOptions struct {
	foil, proxy bool
	board       string
}

type contentFixture struct {
	db      *sqlx.DB
	repo    *PostgresRepository
	service *Service
	userID  string
}

func newContentFixture(t *testing.T) contentFixture {
	t.Helper()
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	seedDecks(t, db, userID)
	repo := NewPostgresRepository(db)
	return contentFixture{db: db, repo: repo, service: NewService(repo), userID: userID}
}

func (f contentFixture) ownedCopy(t *testing.T, p printing, opts copyOptions) int {
	t.Helper()
	if opts.board == "" {
		opts.board = BoardMain
	}
	var id int
	err := f.db.Get(&id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, mana_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 1)
		RETURNING id
	`, f.userID, p.name, p.scryfallID, p.setCode, p.collectorNumber, opts.foil, opts.proxy)
	require.NoError(t, err)
	_, err = f.db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id, board) VALUES ($1, $2, $3)`, id, contentDeckID, opts.board)
	require.NoError(t, err)
	return id
}

func (f contentFixture) pendingCard(t *testing.T, p printing, quantity int, opts copyOptions) int {
	t.Helper()
	if opts.board == "" {
		opts.board = BoardMain
	}
	created, err := f.repo.CreatePendingCard(context.Background(), f.userID, PendingCard{
		DeckID: contentDeckID, Name: p.name, ScryfallID: p.scryfallID, SetCode: p.setCode, CollectorNumber: p.collectorNumber,
		Foil: opts.foil, Proxy: opts.proxy, Board: opts.board, Quantity: quantity, ManaValue: 1,
	})
	require.NoError(t, err)
	return created.ID
}

func (f contentFixture) setCommanders(t *testing.T, commanderID, commanderPendingID, partnerID, partnerPendingID *int) {
	t.Helper()
	_, err := f.db.Exec(`
		UPDATE tamiyo.deck SET commander_id = $2, commander_pending_id = $3, partner_id = $4, partner_pending_id = $5 WHERE id = $1
	`, contentDeckID, commanderID, commanderPendingID, partnerID, partnerPendingID)
	require.NoError(t, err)
}

func (f contentFixture) content(t *testing.T) DeckContent {
	t.Helper()
	content, err := readDeckContent(context.Background(), f.repo, f.userID, contentDeckID)
	require.NoError(t, err)
	return content
}

func TestDeckContent_MergesOwnedAndPendingCopiesOfOnePrinting(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, bolt, copyOptions{})
	second := f.ownedCopy(t, bolt, copyOptions{})
	pendingID := f.pendingCard(t, bolt, 3, copyOptions{})

	content := f.content(t)

	assert.Equal(t, contentDeckID, content.Deck.ID)
	require.Len(t, content.Entries, 1)
	entry := content.Entries[0]
	assert.Equal(t, bolt.name, entry.Name)
	assert.Equal(t, bolt.scryfallID, entry.ScryfallID)
	assert.Equal(t, bolt.setCode, entry.SetCode)
	assert.Equal(t, bolt.collectorNumber, entry.CollectorNumber)
	assert.Equal(t, BoardMain, entry.Board)
	assert.Equal(t, 5, entry.Quantity())
	assert.Equal(t, 2, entry.OwnedQuantity())
	assert.Equal(t, 3, entry.PendingQuantity())
	assert.ElementsMatch(t, []int{first, second}, entry.OwnedCardIDs)
	assert.Equal(t, []PendingPart{{ID: pendingID, Quantity: 3}}, entry.Pending)
	assert.Equal(t, SlotNone, entry.Slot)
	assert.False(t, entry.IsCommander())
}

type entrySummary struct {
	name     string
	board    string
	foil     bool
	proxy    bool
	slot     CommanderSlot
	owned    int
	pending  int
	quantity int
}

func summarize(entries []DeckEntry) []entrySummary {
	summaries := make([]entrySummary, 0, len(entries))
	for _, e := range entries {
		summaries = append(summaries, entrySummary{e.Name, e.Board, e.Foil, e.Proxy, e.Slot, e.OwnedQuantity(), e.PendingQuantity(), e.Quantity()})
	}
	return summaries
}

func TestDeckContent_KeepsProxiesApartFromRealCopies(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{proxy: true})
	f.pendingCard(t, bolt, 2, copyOptions{proxy: true})

	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, owned: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, proxy: true, owned: 1, pending: 2, quantity: 3},
	}, summarize(f.content(t).Entries))
}

func TestDeckContent_KeepsFinishesAndBoardsApart(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	f.pendingCard(t, bolt, 1, copyOptions{foil: true})
	f.ownedCopy(t, bolt, copyOptions{board: BoardSideboard})
	f.pendingCard(t, bolt, 2, copyOptions{board: BoardConsidering})

	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, owned: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, foil: true, owned: 1, pending: 1, quantity: 2},
		{name: bolt.name, board: BoardSideboard, owned: 1, quantity: 1},
		{name: bolt.name, board: BoardConsidering, pending: 2, quantity: 2},
	}, summarize(f.content(t).Entries))
}

func TestDeckContent_KeepsAnOwnedCommanderApartFromAnIdenticalMainBoardCopy(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	other := f.ownedCopy(t, atraxa, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)

	entries := f.content(t).Entries

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, owned: 1, quantity: 1},
	}, summarize(entries))
	assert.Equal(t, []int{commander}, entries[0].OwnedCardIDs)
	assert.Equal(t, []int{other}, entries[1].OwnedCardIDs)
}

func TestDeckContent_SplitsAPendingCommanderOfThreeIntoTheCommanderAndTwoRegularCopies(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, atraxa, 3, copyOptions{})
	f.setCommanders(t, nil, &pendingID, nil, nil)

	entries := f.content(t).Entries

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, pending: 2, quantity: 2},
	}, summarize(entries))
	assert.Equal(t, []PendingPart{{ID: pendingID, Quantity: 1}}, entries[0].Pending)
	assert.Equal(t, []PendingPart{{ID: pendingID, Quantity: 2}}, entries[1].Pending)
}

func TestDeckContent_MergesACommandersExtraCopiesWithAnotherPendingCardOfThePrinting(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 2, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	other := f.pendingCard(t, atraxa, 3, copyOptions{})

	entries := f.content(t).Entries

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, pending: 4, quantity: 4},
	}, summarize(entries))
	assert.ElementsMatch(t, []PendingPart{{ID: commander, Quantity: 1}, {ID: other, Quantity: 3}}, entries[1].Pending)
}

func TestDeckContent_ListsTwoCommandersOwnedAndPending(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.ownedCopy(t, tymna, copyOptions{})
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, &partner, nil)

	entries := f.content(t).Entries

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, owned: 1, quantity: 1},
	}, summarize(entries))
	assert.Equal(t, []int{commander}, entries[0].PendingIDs())
	assert.Empty(t, entries[0].OwnedCardIDs)
	assert.Equal(t, []int{partner}, entries[1].OwnedCardIDs)
	assert.Empty(t, entries[1].Pending)
}

func TestDeckContent_ListsAPendingPartnerAfterAnOwnedCommander(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, tymna, copyOptions{})
	partner := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)

	assert.Equal(t, []entrySummary{
		{name: tymna.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, slot: SlotPartner, pending: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
}

func TestDeckContent_CountsAPendingCardOfMoreThanAThousandCopies(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, island, copyOptions{})
	f.pendingCard(t, island, 1500, copyOptions{})

	assert.Equal(t, []entrySummary{
		{name: island.name, board: BoardMain, owned: 1, pending: 1500, quantity: 1501},
	}, summarize(f.content(t).Entries))
}

func TestDeckContent_CarriesThePricePerCopyInTheOwnersCurrency(t *testing.T) {
	f := newContentFixture(t)
	_, err := f.db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, eur, eur_foil) VALUES ($1, 'Instant', 0.50, 2.00)`, bolt.scryfallID)
	require.NoError(t, err)
	f.ownedCopy(t, bolt, copyOptions{})
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, bolt, 1, copyOptions{foil: true})

	entries := f.content(t).Entries

	require.Len(t, entries, 2)
	require.NotNil(t, entries[0].Price)
	assert.InDelta(t, 0.5, *entries[0].Price, 0.001)
	require.NotNil(t, entries[1].Price)
	assert.InDelta(t, 2.0, *entries[1].Price, 0.001)
}

func TestDeckContent_IsNotReadableByAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	f.pendingCard(t, bolt, 1, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")

	_, err := readDeckContent(context.Background(), f.repo, bob, contentDeckID)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeckContent_OrdersCommandersFirstThenBoardNameSetCollectorNumberFinishAndProxy(t *testing.T) {
	f := newContentFixture(t)
	boltA25 := printing{bolt.name, "e3285e6b-3e79-4d7c-bf96-d920f973b122", "a25", "141"}
	boltOther := printing{bolt.name, "a4c2a5f8-55b3-4b43-a7b2-5e3a6c0f8a11", "2xm", "117"}
	f.pendingCard(t, island, 1, copyOptions{board: BoardConsidering})
	f.ownedCopy(t, island, copyOptions{board: BoardSideboard})
	f.ownedCopy(t, bolt, copyOptions{foil: true, proxy: true})
	f.pendingCard(t, bolt, 1, copyOptions{foil: true})
	f.ownedCopy(t, bolt, copyOptions{proxy: true})
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, boltA25, copyOptions{})
	f.pendingCard(t, boltOther, 1, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	partner := f.ownedCopy(t, atraxa, copyOptions{})
	commander := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, &partner, nil)

	var order []string
	for _, e := range f.content(t).Entries {
		order = append(order, e.Board+" "+e.Name+" "+e.SetCode+" "+e.CollectorNumber+finishLabel(e)+commanderLabel(e))
	}

	assert.Equal(t, []string{
		"main Tymna the Weaver c16 48 (commander)",
		"main Atraxa, Praetors' Voice 2xm 190 (partner)",
		"main Island neo 294",
		"main Lightning Bolt 2xm 117",
		"main Lightning Bolt 2xm 129",
		"main Lightning Bolt 2xm 129 proxy",
		"main Lightning Bolt 2xm 129 foil",
		"main Lightning Bolt 2xm 129 foil proxy",
		"main Lightning Bolt a25 141",
		"sideboard Island neo 294",
		"considering Island neo 294",
	}, order)
}

func finishLabel(e DeckEntry) string {
	label := ""
	if e.Foil {
		label += " foil"
	}
	if e.Proxy {
		label += " proxy"
	}
	return label
}

func commanderLabel(e DeckEntry) string {
	switch e.Slot {
	case SlotCommander:
		return " (commander)"
	case SlotPartner:
		return " (partner)"
	}
	return ""
}
