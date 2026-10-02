package deckinsights

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

func ptr(i int) *int { return &i }

// --- fakes -------------------------------------------------------------

type fakeDeckService struct {
	decks           map[int]deck.Deck
	cardsByDeck     map[int][]deck.DeckCard
	getDeckErr      error
	getDeckCardsErr error
}

func (f *fakeDeckService) GetDeck(ctx context.Context, userID string, id int) (deck.Deck, error) {
	if f.getDeckErr != nil {
		return deck.Deck{}, f.getDeckErr
	}
	d, ok := f.decks[id]
	if !ok {
		return deck.Deck{}, deck.ErrNotFound
	}
	return d, nil
}

func (f *fakeDeckService) GetDeckCards(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]deck.DeckCard, error) {
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.cardsByDeck[id], nil
}

type fakeScryfallFetcher struct {
	cards       map[string]scryfall.Card // keyed by Scryfall ID
	err         error
	lastRequest []scryfall.Identifier
}

func (f *fakeScryfallFetcher) Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error) {
	f.lastRequest = identifiers
	if f.err != nil {
		return nil, f.err
	}
	var out []scryfall.Card
	for _, id := range identifiers {
		if c, ok := f.cards[id.ID]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// --- GetDeckLegality ------------------------------------------------------

func TestGetDeckLegality_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeDeckService{decks: map[int]deck.Deck{}}, &fakeScryfallFetcher{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, 999)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckLegality_AllLegalCardsReportsLegalTrue(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Sol Ring", ScryfallID: "aaaa"}},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
	assert.Equal(t, "commander", report.Format)
}

func TestGetDeckLegality_BannedCardIsReportedAndMarksDeckIllegal(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Channel", ScryfallID: "bbbb"}},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"bbbb": {ID: "bbbb", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"commander": "banned"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "banned")
}

func TestGetDeckLegality_UnrecognizedFormatReturnsErrUnknownFormat(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "not-a-real-format"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Sol Ring", ScryfallID: "aaaa"}},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownFormat)
}

func TestGetDeckLegality_CardNotFoundOnScryfallIsReportedAndMarksDeckIllegal(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Ghost Card", ScryfallID: "cccc"}},
		},
	}
	svc := NewService(decks, &fakeScryfallFetcher{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "not found on scryfall")
}

func TestGetDeckLegality_CommanderSingletonViolationIsReported(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Sol Ring", ScryfallID: "aaaa"},
				{ID: 11, Name: "Sol Ring", ScryfallID: "aaaa"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "singleton violation")
}

func TestGetDeckLegality_DuplicateBasicLandsAreExemptFromSingleton(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Mountain", ScryfallID: "mtn"},
				{ID: 11, Name: "Mountain", ScryfallID: "mtn"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn": {ID: "mtn", Name: "Mountain", TypeLine: "Basic Land — Mountain", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
}

func TestGetDeckLegality_CardOutsideCommanderColorIdentityIsReported(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander", CommanderID: ptr(10)}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Golos, Tireless Pilgrim", ScryfallID: "golos"},
				{ID: 11, Name: "Lightning Bolt", ScryfallID: "bolt"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"golos": {ID: "golos", Name: "Golos, Tireless Pilgrim", TypeLine: "Legendary Creature — Dog", ColorIdentity: []string{}, Legalities: map[string]string{"commander": "legal"}},
		"bolt":  {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "outside commander's color identity")
}

func TestGetDeckLegality_CardWithinCommanderColorIdentityIsFine(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander", CommanderID: ptr(10)}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Krenko, Mob Boss", ScryfallID: "krenko"},
				{ID: 11, Name: "Lightning Bolt", ScryfallID: "bolt"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"krenko": {ID: "krenko", Name: "Krenko, Mob Boss", TypeLine: "Legendary Creature — Goblin", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
		"bolt":   {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.True(t, report.Legal)
}

func TestGetDeckLegality_NonCommanderFormatSkipsConstructionRules(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "modern"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Mountain", ScryfallID: "mtn"},
				{ID: 11, Name: "Mountain", ScryfallID: "mtn"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn": {ID: "mtn", Name: "Mountain", TypeLine: "Basic Land — Mountain", Legalities: map[string]string{"modern": "legal"}},
	}}
	svc := NewService(decks, fetcher)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.True(t, report.Legal) // duplicate Mountains are fine outside commander
}

func TestGetDeckLegality_PropagatesScryfallError(t *testing.T) {
	decks := &fakeDeckService{
		decks:       map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{1: {{ID: 10, Name: "Sol Ring", ScryfallID: "aaaa"}}},
	}
	fetcher := &fakeScryfallFetcher{err: errors.New("network down")}
	svc := NewService(decks, fetcher)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, 1)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

// --- GetDeckStats ----------------------------------------------------------

func TestGetDeckStats_SeparatesLandsFromNonlands(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Name: "Pile", Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Mountain", ScryfallID: "mtn"},
				{ID: 11, Name: "Sol Ring", ScryfallID: "ring"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn":  {ID: "mtn", TypeLine: "Basic Land — Mountain", CMC: 0},
		"ring": {ID: "ring", TypeLine: "Artifact", CMC: 1, Colors: []string{}},
	}}
	svc := NewService(decks, fetcher)

	stats, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, 2, stats.CardCount)
	assert.Equal(t, 1, stats.LandCount)
	assert.Equal(t, 1, stats.NonlandCount)
	assert.Equal(t, 1.0, stats.AverageManaValue)
	assert.Equal(t, 1, stats.ColorBreakdown["C"])
}

func TestGetDeckStats_BuildsManaCurveSortedByManaValue(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "A", ScryfallID: "a"},
				{ID: 11, Name: "B", ScryfallID: "b"},
				{ID: 12, Name: "C", ScryfallID: "c"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"a": {ID: "a", TypeLine: "Creature", CMC: 2, Colors: []string{"R"}},
		"b": {ID: "b", TypeLine: "Creature", CMC: 2, Colors: []string{"R"}},
		"c": {ID: "c", TypeLine: "Instant", CMC: 1, Colors: []string{"U"}},
	}}
	svc := NewService(decks, fetcher)

	stats, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	require.Len(t, stats.ManaCurve, 2)
	assert.Equal(t, ManaCurveBucket{ManaValue: 1, Count: 1}, stats.ManaCurve[0])
	assert.Equal(t, ManaCurveBucket{ManaValue: 2, Count: 2}, stats.ManaCurve[1])
}

func TestGetDeckStats_MulticolorCardCountsUnderEachColor(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Atraxa", ScryfallID: "atraxa"}},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"atraxa": {ID: "atraxa", TypeLine: "Legendary Creature — Angel", CMC: 4, Colors: []string{"W", "U", "B", "G"}},
	}}
	svc := NewService(decks, fetcher)

	stats, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, stats.ColorBreakdown["W"])
	assert.Equal(t, 1, stats.ColorBreakdown["U"])
	assert.Equal(t, 1, stats.ColorBreakdown["B"])
	assert.Equal(t, 1, stats.ColorBreakdown["G"])
}

func TestGetDeckStats_ArtifactCreatureCountsAsCreature(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Construct", ScryfallID: "construct"}},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"construct": {ID: "construct", TypeLine: "Artifact Creature — Construct", CMC: 3, Colors: []string{}},
	}}
	svc := NewService(decks, fetcher)

	stats, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, stats.TypeBreakdown["Creature"])
	assert.Zero(t, stats.TypeBreakdown["Artifact"])
}

func TestGetDeckStats_CardNotFoundOnScryfallCountsAsUnknownType(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {{ID: 10, Name: "Ghost Card", ScryfallID: "ghost"}},
		},
	}
	svc := NewService(decks, &fakeScryfallFetcher{})

	stats, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, stats.CardCount)
	assert.Equal(t, 1, stats.TypeBreakdown["Unknown"])
	assert.Zero(t, stats.LandCount)
	assert.Zero(t, stats.NonlandCount)
}

func TestGetDeckStats_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeDeckService{decks: map[int]deck.Deck{}}, &fakeScryfallFetcher{})

	_, err := svc.GetDeckStats(context.Background(), testUserID, 999)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckStats_DistinctScryfallIDsAreOnlyFetchedOnce(t *testing.T) {
	decks := &fakeDeckService{
		decks: map[int]deck.Deck{1: {ID: 1, Format: "commander"}},
		cardsByDeck: map[int][]deck.DeckCard{
			1: {
				{ID: 10, Name: "Mountain", ScryfallID: "mtn"},
				{ID: 11, Name: "Mountain", ScryfallID: "mtn"},
				{ID: 12, Name: "Mountain", ScryfallID: "mtn"},
			},
		},
	}
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn": {ID: "mtn", TypeLine: "Basic Land — Mountain"},
	}}
	svc := NewService(decks, fetcher)

	_, err := svc.GetDeckStats(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Len(t, fetcher.lastRequest, 1)
}
