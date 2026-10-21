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
	FindDeckTags(ctx context.Context, userID string, deckID string) ([]string, error)
	CreateTag(ctx context.Context, userID string, deckID string, tag string) error
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

func cleanTag(raw string) (string, error) {
	tag := strings.Join(strings.Fields(raw), " ")
	if tag == "" || utf8.RuneCountInString(tag) > maxTagLength {
		return "", ErrInvalidTag
	}
	return tag, nil
}

func (s *Service) deckCardNames(ctx context.Context, userID string, deckID string) (map[string]string, error) {
	content, err := readDeckContent(ctx, s.repo, userID, deckID)
	if err != nil {
		return nil, err
	}
	return cardNames(content.Entries), nil
}

func cardNames(entries []DeckEntry) map[string]string {
	groups := GroupByCardName(entries)
	names := make(map[string]string, len(groups))
	for _, g := range groups {
		names[CardNameKey(g[0].Name)] = g[0].Name
	}
	return names
}

func (s *Service) GetCardTags(ctx context.Context, userID string, deckID string) (DeckTags, error) {
	names, err := s.deckCardNames(ctx, userID, deckID)
	if err != nil {
		return DeckTags{}, err
	}
	rows, err := s.repo.FindCardTags(ctx, userID, deckID)
	if err != nil {
		return DeckTags{}, err
	}

	byName := tagsByName(names, rows)
	distinct := make(map[string]bool)
	for _, tags := range byName {
		for _, tag := range tags {
			distinct[tag] = true
		}
	}
	declared, err := s.repo.FindDeckTags(ctx, userID, deckID)
	if err != nil {
		return DeckTags{}, err
	}
	for _, tag := range declared {
		distinct[tag] = true
	}

	result := DeckTags{Tags: sortedTags(distinct), Cards: make([]TaggedCard, 0, len(byName))}
	for key, tags := range byName {
		result.Cards = append(result.Cards, TaggedCard{Name: names[key], Tags: tags})
	}
	sort.Slice(result.Cards, func(i, j int) bool {
		return strings.ToLower(result.Cards[i].Name) < strings.ToLower(result.Cards[j].Name)
	})
	return result, nil
}

// tagsByName keeps only the tags of cards still in the deck: a tag row
// outlives the last copy of its card.
func tagsByName(names map[string]string, rows []CardTag) map[string][]string {
	byName := make(map[string][]string)
	for _, row := range rows {
		if _, inDeck := names[row.CardName]; inDeck {
			byName[row.CardName] = append(byName[row.CardName], row.Tag)
		}
	}
	for _, tags := range byName {
		sortTags(tags)
	}
	return byName
}

func (s *Service) SetCardTags(ctx context.Context, userID string, deckID string, cardName string, rawTags []string) (TaggedCard, error) {
	names, err := s.deckCardNames(ctx, userID, deckID)
	if err != nil {
		return TaggedCard{}, err
	}
	key := CardNameKey(cardName)
	displayName, inDeck := names[key]
	if !inDeck {
		return TaggedCard{}, ErrCardNotInDeck
	}

	existing, err := existingTags(ctx, s.repo, userID, deckID)
	if err != nil {
		return TaggedCard{}, err
	}
	tags, err := cleanCardTags(existing, rawTags)
	if err != nil {
		return TaggedCard{}, err
	}

	if err := s.repo.ReplaceCardTags(ctx, userID, deckID, key, tags); err != nil {
		return TaggedCard{}, err
	}
	sorted := append([]string{}, tags...)
	sortTags(sorted)
	return TaggedCard{Name: displayName, Tags: sorted}, nil
}

func (s *Service) CreateTag(ctx context.Context, userID string, deckID string, raw string) (string, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return "", err
	}
	tag, err := cleanTag(raw)
	if err != nil {
		return "", err
	}
	existing, err := existingTags(ctx, s.repo, userID, deckID)
	if err != nil {
		return "", err
	}
	if canonical, ok := existing[strings.ToLower(tag)]; ok {
		return canonical, nil
	}
	if err := s.repo.CreateTag(ctx, userID, deckID, tag); err != nil {
		return "", err
	}
	return tag, nil
}

func (s *Service) RenameTag(ctx context.Context, userID string, deckID string, from string, rawTo string) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	to, err := cleanTag(rawTo)
	if err != nil {
		return err
	}
	existing, err := existingTags(ctx, s.repo, userID, deckID)
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
	existing, err := existingTags(ctx, s.repo, userID, deckID)
	if err != nil {
		return err
	}
	current, ok := existing[strings.ToLower(strings.Join(strings.Fields(tag), " "))]
	if !ok {
		return ErrTagNotFound
	}
	return s.repo.DeleteTag(ctx, userID, deckID, current)
}

// cleanCardTags cleans and dedupes a card's tags, reusing the spelling of
// existing (keyed by lowercase tag).
func cleanCardTags(existing map[string]string, rawTags []string) ([]string, error) {
	tags := make([]string, 0, len(rawTags))
	seen := make(map[string]bool)
	for _, raw := range rawTags {
		tag, err := cleanTag(raw)
		if err != nil {
			return nil, err
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
		return nil, ErrTooManyTags
	}
	return tags, nil
}

type tagReader interface {
	FindCardTags(ctx context.Context, userID string, deckID string) ([]CardTag, error)
	FindDeckTags(ctx context.Context, userID string, deckID string) ([]string, error)
}

func existingTags(ctx context.Context, repo tagReader, userID string, deckID string) (map[string]string, error) {
	rows, err := repo.FindCardTags(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	declared, err := repo.FindDeckTags(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]string)
	for _, tag := range declared {
		existing[strings.ToLower(tag)] = tag
	}
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
