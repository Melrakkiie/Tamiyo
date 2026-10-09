package deck

import (
	"context"
	"time"
)

const (
	ColorModeExact   = "exact"
	ColorModeInclude = "include"
	ColorModeWithin  = "within"
)

type PublicFilter struct {
	Name      string
	Format    string
	Commander string
	Card      string
	Owner     string

	Colors     []string
	Colorless  bool
	ColorMode  string
	ColorCount *int

	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

type PublicDeck struct {
	ID                   string
	Name                 string
	Format               string
	BackgroundScryfallID *string
	CommanderScryfallID  *string
	CommanderName        *string
	ColorIdentity        string
	CardCount            int
	OwnerID              string
	OwnerDisplayName     *string
	OwnerAvatarID        *string
	Added                time.Time
	Updated              time.Time
}

type PublicRepository interface {
	FindPublic(ctx context.Context, filter PublicFilter) ([]PublicDeck, int, error)
}

func (s *Service) BrowsePublicDecks(ctx context.Context, filter PublicFilter) ([]PublicDeck, int, error) {
	return s.repo.FindPublic(ctx, filter)
}
