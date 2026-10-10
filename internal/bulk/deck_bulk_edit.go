package bulk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type deckCopyMatch struct {
	card    deck.DeckCard
	matched bool
}

type pendingMatch struct {
	item deck.PendingCard
	kept int
}

func lineMatches(line moxfieldDeckLine, name string, setCode string, collectorNumber string, foil bool, board string) bool {
	if boardOf(board) != line.Board {
		return false
	}
	if line.SetCode != "" {
		return strings.EqualFold(setCode, line.SetCode) && collectorNumber == line.CollectorNumber && foil == line.Foil
	}
	if line.Foil && !foil {
		return false
	}
	wanted := deck.CardNameKey(line.CardName)
	return deck.CardNameKey(name) == wanted || deck.CardNameKey(frontFaceName(name)) == wanted
}

func bulkEditOrder(lines []moxfieldDeckLine) []moxfieldDeckLine {
	ordered := append([]moxfieldDeckLine(nil), lines...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].SetCode != "" && ordered[j].SetCode == ""
	})
	return ordered
}

func (s *Service) BulkEditDeck(ctx context.Context, userID string, deckID string, r io.Reader) (Summary, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return Summary{}, ErrDeckNotFound
		}
		return Summary{}, err
	}
	data, err := readImport(r)
	if err != nil {
		return Summary{}, err
	}
	lines, err := parseMoxfieldDeckList(bytes.NewReader(data))
	if err != nil {
		return Summary{}, err
	}

	cards, err := s.decks.GetDeckCards(ctx, userID, d.ID, "name", false)
	if err != nil {
		return Summary{}, fmt.Errorf("loading deck cards: %w", err)
	}
	pending, err := s.decks.GetPendingCards(ctx, userID, d.ID)
	if err != nil {
		return Summary{}, fmt.Errorf("loading pending cards: %w", err)
	}

	copies := make([]*deckCopyMatch, 0, len(cards))
	for _, c := range cards {
		copies = append(copies, &deckCopyMatch{card: c})
	}
	sort.SliceStable(copies, func(i, j int) bool {
		return d.CommanderID != nil && copies[i].card.ID == *d.CommanderID && copies[j].card.ID != *d.CommanderID
	})
	pendings := make([]*pendingMatch, 0, len(pending))
	for _, p := range pending {
		pendings = append(pendings, &pendingMatch{item: p})
	}

	var toAdd []moxfieldDeckLine
	for _, line := range bulkEditOrder(lines) {
		need := line.Quantity
		for _, c := range copies {
			if need == 0 {
				break
			}
			if !c.matched && lineMatches(line, c.card.Name, c.card.SetCode, c.card.CollectorNumber, c.card.Foil, c.card.Board) {
				c.matched = true
				need--
			}
		}
		for _, p := range pendings {
			if need == 0 {
				break
			}
			free := p.item.Quantity - p.kept
			if free > 0 && lineMatches(line, p.item.Name, p.item.SetCode, p.item.CollectorNumber, p.item.Foil, p.item.Board) {
				taken := min(free, need)
				p.kept += taken
				need -= taken
			}
		}
		if need > 0 {
			missing := line
			missing.Quantity = need
			toAdd = append(toAdd, missing)
		}
	}

	var resolved map[string]ResolvedCard
	if len(toAdd) > 0 {
		if resolved, err = s.resolveDeckLines(ctx, toAdd); err != nil {
			return Summary{}, err
		}
	}

	var summary Summary
	if err := s.removeUnlisted(ctx, userID, d, copies, pendings, &summary); err != nil {
		return summary, err
	}

	if len(toAdd) > 0 {
		placements, placed, err := s.placeResolvedLines(ctx, userID, toAdd, resolved, false, d.ID)
		if err != nil {
			return summary, err
		}
		summary.CardsSkipped += placed.CardsSkipped
		summary.Warnings = append(summary.Warnings, placed.Warnings...)
		s.fillDeck(ctx, userID, d.ID, placements, &summary)
	}

	s.replaceListedTags(ctx, userID, d.ID, lines, &summary)
	return summary, nil
}

func (s *Service) removeUnlisted(ctx context.Context, userID string, d deck.Deck, copies []*deckCopyMatch, pendings []*pendingMatch, summary *Summary) error {
	for _, c := range copies {
		if c.matched {
			continue
		}
		if d.CommanderID != nil && *d.CommanderID == c.card.ID {
			if err := s.decks.ClearCommander(ctx, userID, d.ID); err != nil {
				return fmt.Errorf("clearing the commander: %w", err)
			}
		}
		if err := s.decks.RemoveCardFromDeck(ctx, userID, d.ID, c.card.ID); err != nil {
			return fmt.Errorf("removing %q from the deck: %w", c.card.Name, err)
		}
		summary.CardsRemoved++
	}
	for _, p := range pendings {
		removed := p.item.Quantity - p.kept
		if removed == 0 {
			continue
		}
		if p.kept == 0 {
			if err := s.decks.RemovePendingCard(ctx, userID, d.ID, p.item.ID); err != nil {
				return fmt.Errorf("removing %q from the deck: %w", p.item.Name, err)
			}
		} else if _, err := s.decks.SetPendingQuantity(ctx, userID, d.ID, p.item.ID, p.kept); err != nil {
			return fmt.Errorf("updating %q in the deck: %w", p.item.Name, err)
		}
		summary.CardsRemoved += removed
	}
	return nil
}

func (s *Service) replaceListedTags(ctx context.Context, userID string, deckID string, lines []moxfieldDeckLine, summary *Summary) {
	tags := make(map[string][]string)
	names := make(map[string]string)
	var order []string
	for _, line := range lines {
		key := deck.CardNameKey(line.CardName)
		if _, seen := names[key]; !seen {
			names[key] = line.CardName
			order = append(order, key)
		}
		for _, tag := range line.Tags {
			if !containsFold(tags[key], tag) {
				tags[key] = append(tags[key], tag)
			}
		}
	}

	current, err := s.decks.GetCardTags(ctx, userID, deckID)
	if err != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not load the tags: %v", err))
		return
	}
	existing := current.ByCardName()
	for _, key := range order {
		wanted := tags[key]
		if sameTags(existing[key], wanted) {
			continue
		}
		if wanted == nil {
			wanted = []string{}
		}
		if _, err := s.decks.SetCardTags(ctx, userID, deckID, names[key], wanted); err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not tag %q: %v", names[key], err))
		}
	}
}

func sameTags(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, tag := range a {
		if !containsFold(b, tag) {
			return false
		}
	}
	return true
}

func (h *Handler) bulkEditDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.BulkEditDeck(ctx.Request.Context(), userID, deckID, file)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrInvalidFile, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway},
		)
		return
	}
	ctx.JSON(http.StatusOK, summary)
}
