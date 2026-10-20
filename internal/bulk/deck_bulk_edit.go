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

type ownedMatch struct {
	entry   deck.DeckEntry
	cardID  int
	matched bool
}

type pendingMatch struct {
	entry deck.DeckEntry
	part  deck.PendingPart
	kept  int
}

func lineMatches(line moxfieldDeckLine, e deck.DeckEntry) bool {
	if deck.BoardOrMain(e.Board) != line.Board {
		return false
	}
	if line.SetCode != "" {
		return strings.EqualFold(e.SetCode, line.SetCode) && e.CollectorNumber == line.CollectorNumber && e.Foil == line.Foil
	}
	if line.Foil && !e.Foil {
		return false
	}
	wanted := deck.CardNameKey(line.CardName)
	return deck.CardNameKey(e.Name) == wanted || deck.CardNameKey(deck.FrontFaceName(e.Name)) == wanted
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

	content, err := s.decks.GetDeckContent(ctx, userID, d.ID)
	if err != nil {
		return Summary{}, fmt.Errorf("loading deck content: %w", err)
	}
	var copies []*ownedMatch
	var pendings []*pendingMatch
	for _, e := range content.Entries {
		for _, id := range e.OwnedCardIDs {
			copies = append(copies, &ownedMatch{entry: e, cardID: id})
		}
		for _, part := range e.Pending {
			pendings = append(pendings, &pendingMatch{entry: e, part: part})
		}
	}

	var toAdd []moxfieldDeckLine
	for _, line := range bulkEditOrder(lines) {
		need := line.Quantity
		for _, c := range copies {
			if need == 0 {
				break
			}
			if !c.matched && lineMatches(line, c.entry) {
				c.matched = true
				need--
			}
		}
		for _, p := range pendings {
			if need == 0 {
				break
			}
			free := p.part.Quantity - p.kept
			if free > 0 && lineMatches(line, p.entry) {
				taken := min(free, need)
				p.kept += taken
				need -= taken
			}
		}
		if need > 0 {
			missing := line
			missing.Quantity = need
			missing.Commander = false
			toAdd = append(toAdd, missing)
		}
	}

	var resolved map[string]ResolvedCard
	if len(toAdd) > 0 {
		if resolved, err = s.resolveDeckLines(ctx, toAdd); err != nil {
			return Summary{}, err
		}
	}

	tags, err := s.changedTags(ctx, userID, d.ID, lines)
	if err != nil {
		return Summary{}, err
	}
	plan := deck.PlacementPlan{ReplaceTags: tags}
	var summary Summary
	summary.CardsRemoved = planRemovals(&plan, content.Entries, copies, pendings)

	if len(toAdd) > 0 {
		var unresolved Summary
		plan.Lines, unresolved = placementLines(toAdd, resolved)
		summary.CardsSkipped += unresolved.CardsSkipped
		summary.Warnings = append(summary.Warnings, unresolved.Warnings...)
	}
	if len(plan.Lines)+len(plan.RemoveCards)+len(plan.PendingRemovals)+len(plan.ReplaceTags) == 0 {
		return summary, nil
	}
	if err := s.placeLines(ctx, userID, d.ID, plan, &summary); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// planRemovals lists in the plan the copies and pending quantities the list
// leaves out, and returns how many cards that removes.
func planRemovals(plan *deck.PlacementPlan, entries []deck.DeckEntry, copies []*ownedMatch, pendings []*pendingMatch) int {
	removed := 0
	for _, c := range copies {
		if !c.matched {
			plan.RemoveCards = append(plan.RemoveCards, c.cardID)
			removed++
		}
	}
	kept := make(map[int]int)
	for _, p := range pendings {
		kept[p.part.ID] += p.kept
	}
	for _, p := range deck.PendingTotals(entries) {
		if gone := p.Quantity - kept[p.ID]; gone != 0 {
			plan.PendingRemovals = append(plan.PendingRemovals, deck.PendingRemoval{ID: p.ID, Count: gone})
			removed += gone
		}
	}
	return removed
}

// changedTags returns the listed cards whose tags differ from the deck's.
func (s *Service) changedTags(ctx context.Context, userID string, deckID string, lines []moxfieldDeckLine) ([]deck.TaggedCard, error) {
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
		return nil, fmt.Errorf("loading the tags: %w", err)
	}
	existing := current.ByCardName()
	var changed []deck.TaggedCard
	for _, key := range order {
		wanted := tags[key]
		if sameTags(existing[key], wanted) {
			continue
		}
		if wanted == nil {
			wanted = []string{}
		}
		changed = append(changed, deck.TaggedCard{Name: names[key], Tags: wanted})
	}
	return changed, nil
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

	file, err := openUploadedFile(ctx)
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
