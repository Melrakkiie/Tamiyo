package bulk

import "Melrakkiie/Tamiyo/internal/card"

// copiesOf expands one import line into quantity identical copies of the plan.
func copiesOf(base card.Card, quantity int) []card.ImportCopy {
	copies := make([]card.ImportCopy, quantity)
	for i := range copies {
		copies[i] = card.ImportCopy{Card: base}
	}
	return copies
}
