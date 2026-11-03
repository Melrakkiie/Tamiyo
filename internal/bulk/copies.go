package bulk

import "Melrakkiie/Tamiyo/internal/card"

// existingOrPlanned finds where a named storage or deck lives: its id when it already exists (cache),
// else its index in the plan, calling add to put it there the first time the name is met.
func existingOrPlanned[ID any](cache map[string]ID, planned map[string]int, name string, add func() int) (id *ID, planIdx *int) {
	if existing, ok := cache[name]; ok {
		return &existing, nil
	}
	idx, ok := planned[name]
	if !ok {
		idx = add()
		planned[name] = idx
	}
	return nil, &idx
}

// copiesOf expands one import line into quantity identical copies of the plan.
func copiesOf(base card.Card, quantity int) []card.ImportCopy {
	copies := make([]card.ImportCopy, quantity)
	for i := range copies {
		copies[i] = card.ImportCopy{Card: base}
	}
	return copies
}
