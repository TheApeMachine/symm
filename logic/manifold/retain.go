package manifold

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
)

/*
intensityCandidate is one Hawkes-ready symbol queued for field advance.
*/
type intensityCandidate struct {
	symbol    string
	outcome   excitation.Outcome
	intensity float64
}

/*
bookReady keeps only symbols whose L3 book can ground the field. Hawkes
intensity without a book leaves Candidate evaluation empty.
*/
func bookReady(
	candidates []intensityCandidate,
	source BookSource,
) []intensityCandidate {
	if source == nil || len(candidates) == 0 {
		return candidates
	}

	ready := make([]intensityCandidate, 0, len(candidates))

	for _, candidate := range candidates {
		if _, _, ok := ordersForSymbol(source, candidate.symbol); !ok {
			continue
		}

		ready = append(ready, candidate)
	}

	return ready
}
