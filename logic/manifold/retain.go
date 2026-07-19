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
intensity without a book leaves Candidate evaluation empty. Readiness is a cheap
two-sided touch peek so the authoritative population is copied exactly once, in
advance, rather than once here and again per stepped symbol.
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
		if !touchReady(source, candidate.symbol) {
			continue
		}

		ready = append(ready, candidate)
	}

	return ready
}
