package deck

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxTagLength   = 40
	maxTagsPerCard = 20
)

var (
	ErrCardNotInDeck = errors.New("card is not in the deck")
	ErrTagNotFound   = errors.New("tag not found")
	ErrInvalidTag    = errors.New("a tag must have between 1 and 40 characters")
	ErrTooManyTags   = errors.New("a card can have at most 20 tags")
)

type CardTag struct {
	CardName string
	Tag      string
}

type TagRepository interface {
	FindCardTags(ctx context.Context, userID string, deckID string) ([]CardTag, error)
	ReplaceCardTags(ctx context.Context, userID string, deckID string, cardName string, tags []string) error
	RenameTag(ctx context.Context, userID string, deckID string, from string, to string) error
	DeleteTag(ctx context.Context, userID string, deckID string, tag string) error
}

type TaggedCard struct {
	Name string
	Tags []string
}

type DeckTags struct {
	Tags  []string
	Cards []TaggedCard
}

func CardNameKey(name string) string {
	key := strings.ToLower(strings.ReplaceAll(name, "//", "/"))
	return strings.Join(strings.Fields(key), " ")
}

func (t DeckTags) ByCardName() map[string][]string {
	byName := make(map[string][]string, len(t.Cards))
	for _, c := range t.Cards {
		byName[CardNameKey(c.Name)] = c.Tags
	}
	return byName
}

func cleanTag(raw string) (string, error) {
	tag := strings.Join(strings.Fields(raw), " ")
	if tag == "" || utf8.RuneCountInString(tag) > maxTagLength {
		return "", ErrInvalidTag
	}
	return tag, nil
}

func (s *Service) deckCardNames(ctx context.Context, userID string, deckID string) (map[string]string, error) {
	owned, err := s.repo.FindCardsByDeckID(ctx, userID, deckID, "name", false)
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.FindPendingCards(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(owned)+len(pending))
	for _, c := range owned {
		names[CardNameKey(c.Name)] = c.Name
	}
	for _, p := range pending {
		key := CardNameKey(p.Name)
		if _, ok := names[key]; !ok {
			names[key] = p.Name
		}
	}
	return names, nil
}

func (s *Service) GetCardTags(ctx context.Context, userID string, deckID string) (DeckTags, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return DeckTags{}, err
	}
	names, err := s.deckCardNames(ctx, userID, deckID)
	if err != nil {
		return DeckTags{}, err
	}
	rows, err := s.repo.FindCardTags(ctx, userID, deckID)
	if err != nil {
		return DeckTags{}, err
	}

	byName := make(map[string][]string)
	distinct := make(map[string]bool)
	for _, row := range rows {
		if _, inDeck := names[row.CardName]; !inDeck {
			continue
		}
		byName[row.CardName] = append(byName[row.CardName], row.Tag)
		distinct[row.Tag] = true
	}

	result := DeckTags{Tags: sortedTags(distinct), Cards: make([]TaggedCard, 0, len(byName))}
	for key, tags := range byName {
		sortTags(tags)
		result.Cards = append(result.Cards, TaggedCard{Name: names[key], Tags: tags})
	}
	sort.Slice(result.Cards, func(i, j int) bool {
		return strings.ToLower(result.Cards[i].Name) < strings.ToLower(result.Cards[j].Name)
	})
	return result, nil
}

func (s *Service) SetCardTags(ctx context.Context, userID string, deckID string, cardName string, rawTags []string) (TaggedCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return TaggedCard{}, err
	}
	names, err := s.deckCardNames(ctx, userID, deckID)
	if err != nil {
		return TaggedCard{}, err
	}
	key := CardNameKey(cardName)
	displayName, inDeck := names[key]
	if !inDeck {
		return TaggedCard{}, ErrCardNotInDeck
	}

	existing, err := s.existingTags(ctx, userID, deckID)
	if err != nil {
		return TaggedCard{}, err
	}

	tags := make([]string, 0, len(rawTags))
	seen := make(map[string]bool)
	for _, raw := range rawTags {
		tag, err := cleanTag(raw)
		if err != nil {
			return TaggedCard{}, err
		}
		if canonical, ok := existing[strings.ToLower(tag)]; ok {
			tag = canonical
		}
		if seen[strings.ToLower(tag)] {
			continue
		}
		seen[strings.ToLower(tag)] = true
		tags = append(tags, tag)
	}
	if len(tags) > maxTagsPerCard {
		return TaggedCard{}, ErrTooManyTags
	}

	if err := s.repo.ReplaceCardTags(ctx, userID, deckID, key, tags); err != nil {
		return TaggedCard{}, err
	}
	sorted := append([]string{}, tags...)
	sortTags(sorted)
	return TaggedCard{Name: displayName, Tags: sorted}, nil
}

func (s *Service) RenameTag(ctx context.Context, userID string, deckID string, from string, rawTo string) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	to, err := cleanTag(rawTo)
	if err != nil {
		return err
	}
	existing, err := s.existingTags(ctx, userID, deckID)
	if err != nil {
		return err
	}
	current, ok := existing[strings.ToLower(strings.Join(strings.Fields(from), " "))]
	if !ok {
		return ErrTagNotFound
	}
	if canonical, ok := existing[strings.ToLower(to)]; ok && canonical != current {
		to = canonical
	}
	if to == current {
		return nil
	}
	return s.repo.RenameTag(ctx, userID, deckID, current, to)
}

func (s *Service) DeleteTag(ctx context.Context, userID string, deckID string, tag string) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	existing, err := s.existingTags(ctx, userID, deckID)
	if err != nil {
		return err
	}
	current, ok := existing[strings.ToLower(strings.Join(strings.Fields(tag), " "))]
	if !ok {
		return ErrTagNotFound
	}
	return s.repo.DeleteTag(ctx, userID, deckID, current)
}

func (s *Service) existingTags(ctx context.Context, userID string, deckID string) (map[string]string, error) {
	rows, err := s.repo.FindCardTags(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]string)
	for _, row := range rows {
		if _, ok := existing[strings.ToLower(row.Tag)]; !ok {
			existing[strings.ToLower(row.Tag)] = row.Tag
		}
	}
	return existing, nil
}

func sortTags(tags []string) {
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
}

func sortedTags(set map[string]bool) []string {
	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	sortTags(tags)
	return tags
}
