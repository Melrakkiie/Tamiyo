package printing

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

const (
	refreshBatchSize = 750
	staleAfter       = 24 * time.Hour
	checkInterval    = 2 * time.Minute
)

type Printing struct {
	ScryfallID string
	Name       string
	ManaValue  float64
	ManaCost   string
	TypeLine   string
	OracleText string
	Rarity     string
	Keywords   []string
	// LegalFormats holds the formats where the card is legal or restricted,
	// the meaning search by format relies on.
	LegalFormats      []string
	BannedFormats     []string
	RestrictedFormats []string
	Colors            []string
	ColorIdentity     []string
	Powers            []string
	Toughnesses       []string
	Prices            scryfall.Prices
	GameChanger       bool
}

type Repository interface {
	IDsToRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]string, error)
	Upsert(ctx context.Context, printings []Printing) error
	// MarkRefreshed stamps printings as refreshed without changing their
	// data, so they stop coming back first.
	MarkRefreshed(ctx context.Context, ids []string) error
}

type Fetcher interface {
	Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error)
}

type Refresher struct {
	repo    Repository
	fetcher Fetcher
	logger  *zap.Logger
	now     func() time.Time
}

func NewRefresher(repo Repository, fetcher Fetcher, logger *zap.Logger) *Refresher {
	return &Refresher{repo: repo, fetcher: fetcher, logger: logger, now: time.Now}
}

func FromScryfall(card scryfall.Card) Printing {
	return Printing{
		ScryfallID: card.ID, Name: card.Name, ManaValue: card.CMC, ManaCost: card.ManaCost, TypeLine: card.TypeLine,
		OracleText: card.OracleText, Rarity: card.Rarity, Keywords: card.Keywords,
		LegalFormats:      formatsWithStatus(card.Legalities, "legal", "restricted"),
		BannedFormats:     formatsWithStatus(card.Legalities, "banned"),
		RestrictedFormats: formatsWithStatus(card.Legalities, "restricted"),
		Colors:            nonNil(card.Colors),
		ColorIdentity:     nonNil(card.ColorIdentity),
		Powers:            card.Powers, Toughnesses: card.Toughnesses, Prices: card.Prices,
		GameChanger: card.GameChanger,
	}
}

// formatsWithStatus lists, sorted, the formats whose Scryfall legality is one
// of statuses.
func formatsWithStatus(legalities map[string]string, statuses ...string) []string {
	formats := []string{}
	for format, status := range legalities {
		if slices.Contains(statuses, status) {
			formats = append(formats, format)
		}
	}
	sort.Strings(formats)
	return formats
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *Refresher) RefreshBatch(ctx context.Context) (int, error) {
	ids, err := r.repo.IDsToRefresh(ctx, r.now().Add(-staleAfter), refreshBatchSize)
	if err != nil || len(ids) == 0 {
		return 0, err
	}

	identifiers := make([]scryfall.Identifier, len(ids))
	for i, id := range ids {
		identifiers[i] = scryfall.Identifier{ID: id}
	}
	cards, err := r.fetcher.Fetch(ctx, identifiers)
	rejected := map[string]bool{}
	var rejectedErr *scryfall.RejectedError
	if errors.As(err, &rejectedErr) {
		for _, identifier := range rejectedErr.Identifiers {
			rejected[strings.ToLower(identifier.ID)] = true
		}
	} else if err != nil {
		return 0, err
	}

	found := make(map[string]Printing, len(cards))
	for _, card := range cards {
		found[strings.ToLower(card.ID)] = FromScryfall(card)
	}
	printings := make([]Printing, 0, len(ids))
	var rejectedIDs []string
	for _, id := range ids {
		// A rejected printing keeps its data: a request Scryfall refuses
		// whole would otherwise blank every printing.
		if rejected[strings.ToLower(id)] {
			rejectedIDs = append(rejectedIDs, id)
			continue
		}
		p, ok := found[strings.ToLower(id)]
		if !ok {
			p = Printing{
				ScryfallID: id, LegalFormats: []string{}, BannedFormats: []string{}, RestrictedFormats: []string{},
				Colors: []string{}, ColorIdentity: []string{}, Powers: []string{}, Toughnesses: []string{}, Keywords: []string{},
			}
		}
		p.ScryfallID = id
		printings = append(printings, p)
	}
	if err := r.repo.Upsert(ctx, printings); err != nil {
		return 0, err
	}
	if len(rejectedIDs) > 0 {
		r.logger.Warn("scryfall rejected printings", zap.Strings("scryfall_ids", rejectedIDs))
		if err := r.repo.MarkRefreshed(ctx, rejectedIDs); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (r *Refresher) refreshAll(ctx context.Context) {
	total := 0
	for ctx.Err() == nil {
		count, err := r.RefreshBatch(ctx)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.Warn("refreshing printings failed", zap.Error(err))
			}
			return
		}
		if count == 0 {
			break
		}
		total += count
	}
	if total > 0 {
		r.logger.Info("printings refreshed", zap.Int("count", total))
	}
}

func (r *Refresher) Run(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		r.refreshAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
