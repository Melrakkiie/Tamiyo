package printing

import (
	"context"
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
	ScryfallID   string
	TypeLine     string
	LegalFormats []string
}

type Repository interface {
	IDsToRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]string, error)
	Upsert(ctx context.Context, printings []Printing) error
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
	formats := make([]string, 0, len(card.Legalities))
	for format, status := range card.Legalities {
		if status == "legal" || status == "restricted" {
			formats = append(formats, format)
		}
	}
	sort.Strings(formats)
	return Printing{ScryfallID: card.ID, TypeLine: card.TypeLine, LegalFormats: formats}
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
	if err != nil {
		return 0, err
	}

	found := make(map[string]Printing, len(cards))
	for _, card := range cards {
		found[strings.ToLower(card.ID)] = FromScryfall(card)
	}
	printings := make([]Printing, 0, len(ids))
	for _, id := range ids {
		p, ok := found[strings.ToLower(id)]
		if !ok {
			p = Printing{ScryfallID: id, LegalFormats: []string{}}
		}
		p.ScryfallID = id
		printings = append(printings, p)
	}
	if err := r.repo.Upsert(ctx, printings); err != nil {
		return 0, err
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
