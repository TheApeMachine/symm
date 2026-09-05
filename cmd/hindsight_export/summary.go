package main

import (
	"encoding/json"
	"io"
	"sort"
)

/*
summary is the aggregate account of a run: how far each planning round got,
and how the council was distributed when it got there.

It exists because the per-round stream answers "what happened on this tick"
but not "where is the funnel dying", which is the first question anyone asks
of a run that opened no positions.
*/
type summary struct {
	Run    string `json:"run"`
	Rounds int    `json:"rounds"`

	// Statuses counts rounds by the gate they stopped at, so the funnel's
	// narrowest point is visible without reading a single round.
	Statuses map[string]int `json:"statusCounts"`
	Actions  map[string]int `json:"actionCounts"`
	Symbols  int            `json:"symbols"`

	// Searched counts rounds that reached the causal search at all.
	Searched     int            `json:"roundsSearched"`
	Recommended  map[string]int `json:"searchRecommended,omitempty"`
	Identified   map[string]int `json:"searchIdentification,omitempty"`
	DominantMove map[string]int `json:"consensusDominantMove,omitempty"`

	// MeanMoveMass is the run-average probability the council assigned each
	// move. A run whose average is indistinguishable from its prior is a run
	// whose advisors never said anything.
	MeanMoveMass map[string]float64 `json:"meanMoveMass,omitempty"`

	// ConfidenceQuantiles reports the shape of council confidence across the
	// run, not just its mean, so a flat distribution is distinguishable from
	// a bimodal one.
	ConfidenceQuantiles map[string]float64 `json:"confidenceQuantiles,omitempty"`
}

/* newSummary starts an empty aggregate for one run. */
func newSummary(runID string) *summary {
	return &summary{
		Run:          runID,
		Statuses:     make(map[string]int),
		Actions:      make(map[string]int),
		Recommended:  make(map[string]int),
		Identified:   make(map[string]int),
		DominantMove: make(map[string]int),
		MeanMoveMass: make(map[string]float64),
	}
}

/* observe folds one round into the aggregate. */
func (aggregate *summary) observe(record round, symbols map[string]bool, confidences *[]float64) {
	aggregate.Rounds++
	aggregate.Statuses[record.PredictiveStatus]++
	aggregate.Actions[record.Action]++
	symbols[record.Symbol] = true
	*confidences = append(*confidences, record.Confidence)

	for move, mass := range record.Probabilities {
		aggregate.MeanMoveMass[move] += mass
	}

}

/* finish converts running totals into the reported averages and quantiles. */
func (aggregate *summary) finish(symbols map[string]bool, confidences []float64) {
	aggregate.Symbols = len(symbols)

	if aggregate.Rounds > 0 {
		for move := range aggregate.MeanMoveMass {
			aggregate.MeanMoveMass[move] /= float64(aggregate.Rounds)
		}
	}

	if len(confidences) == 0 {
		return
	}

	sort.Float64s(confidences)
	aggregate.ConfidenceQuantiles = map[string]float64{
		"p05":    quantile(confidences, 0.05),
		"p25":    quantile(confidences, 0.25),
		"median": quantile(confidences, 0.50),
		"p75":    quantile(confidences, 0.75),
		"p95":    quantile(confidences, 0.95),
		"max":    confidences[len(confidences)-1],
	}
}

/* quantile reads one quantile from an already sorted sample. */
func quantile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	index := int(fraction * float64(len(sorted)-1))

	return sorted[index]
}

/* write emits the aggregate as one indented JSON object. */
func (aggregate *summary) write(target io.Writer) error {
	encoder := json.NewEncoder(target)
	encoder.SetIndent("", "  ")

	return encoder.Encode(aggregate)
}
