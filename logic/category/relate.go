package category

import (
	"math"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
evidenceClock is the temporal envelope of a category's supporting measurements.
Horizon is the max producer horizon among those rows so staleness is relative
to the estimator's own scale, not a wall-clock constant.
*/
type evidenceClock struct {
	from    time.Time
	through time.Time
	horizon time.Duration
	mass    float64
	ok      bool
}

/*
contradictMass sums indexed live mass for affinity contradictions from→to.
*/
func contradictMass(
	index *evidenceIndex,
	symbol string,
	from, to types.CategoryType,
) (mass float64, evidence []string) {
	targets := contradictIndex[from]

	if len(targets) == 0 {
		return 0, nil
	}

	for _, metric := range targets[to] {
		value := index.metricMass(symbol, metric)

		if value <= 0 {
			continue
		}

		mass += value
		evidence = append(evidence, string(metric))
	}

	return mass, evidence
}

/*
sharedSupport returns the Jaccard overlap of supporting metric keys and the
shared key list. RedundantWith is exactly this overlap under live activation.
*/
func sharedSupport(left, right []string) (jaccard float64, shared []string) {
	if len(left) == 0 || len(right) == 0 {
		return 0, nil
	}

	seen := map[string]struct{}{}

	for _, metric := range left {
		seen[metric] = struct{}{}
	}

	union := len(seen)

	for _, metric := range right {
		if _, ok := seen[metric]; ok {
			shared = append(shared, metric)
			continue
		}

		seen[metric] = struct{}{}
		union++
	}

	if union == 0 || len(shared) == 0 {
		return 0, nil
	}

	return float64(len(shared)) / float64(union), shared
}

/*
conditionsMass is the strength contribution when A's supporting metrics fill
B's missing required evidence — A Conditions B.
*/
func conditionsMass(provider, dependent types.Category) (float64, []string) {
	if len(provider.Supporting) == 0 || len(dependent.Missing) == 0 {
		return 0, nil
	}

	have := map[string]struct{}{}

	for _, metric := range provider.Supporting {
		have[metric] = struct{}{}
	}

	filled := make([]string, 0, len(dependent.Missing))

	for _, metric := range dependent.Missing {
		if _, ok := have[metric]; ok {
			filled = append(filled, metric)
		}
	}

	if len(filled) == 0 {
		return 0, nil
	}

	mass := provider.Strength * dependent.Strength *
		float64(len(filled)) / float64(len(dependent.Missing))

	return mass, filled
}

/*
alignable reports whether two evidence clocks can be compared temporally.
Disjoint intervals with no horizon coverage are IncomparableWith.
*/
func alignable(left, right evidenceClock) bool {
	if !left.ok || !right.ok {
		return false
	}

	if !left.through.Before(right.from) && !right.through.Before(left.from) {
		return true
	}

	gap := right.from.Sub(left.through)

	if gap < 0 {
		gap = left.from.Sub(right.through)
	}

	cover := left.horizon

	if right.horizon > cover {
		cover = right.horizon
	}

	return cover > 0 && gap <= cover
}

/*
staleMass returns mass when left's evidence is stale relative to right: left's
latest sample is older than right's by more than left's own horizon.
*/
func staleMass(left, right evidenceClock, leftStrength, rightStrength float64) float64 {
	if !left.ok || !right.ok || left.horizon <= 0 {
		return 0
	}

	if !right.through.After(left.through) {
		return 0
	}

	if right.through.Sub(left.through) <= left.horizon {
		return 0
	}

	return math.Sqrt(leftStrength * rightStrength)
}

/*
leadMass returns mass when left's evidence envelope precedes right's on an
alignable clock. Contemporaneous envelopes (neither strictly before) yield zero.
*/
func leadMass(left, right evidenceClock, leftStrength, rightStrength float64) float64 {
	if !alignable(left, right) {
		return 0
	}

	if !left.through.Before(right.from) && !right.through.Before(left.from) {
		// Overlap: use earliest-from ordering only when from times differ.
		if left.from.Equal(right.from) {
			return 0
		}

		if left.from.Before(right.from) {
			return math.Sqrt(leftStrength * rightStrength)
		}

		return 0
	}

	if left.through.Before(right.from) || left.from.Before(right.from) {
		return math.Sqrt(leftStrength * rightStrength)
	}

	return 0
}

/*
linkPair derives every justified typed edge for one ordered observation of two
active categories on the same symbol. Edge types come from CategoryAffinity and
measurement temporal envelopes — not from trap/opportunity labels or top-winner
flips.
*/
func (graph *Graph) linkPair(
	at time.Time,
	index *evidenceIndex,
	symbol string,
	first, second types.Category,
) {
	if first.Strength <= 0 || second.Strength <= 0 {
		return
	}

	jaccard, shared := sharedSupport(first.Supporting, second.Supporting)

	if jaccard > 0 {
		graph.strengthen(
			at, symbol, first.Type, second.Type, RedundantWith,
			jaccard*math.Sqrt(first.Strength*second.Strength), shared,
		)
		graph.strengthen(
			at, symbol, second.Type, first.Type, RedundantWith,
			jaccard*math.Sqrt(first.Strength*second.Strength), shared,
		)
	}

	if mass, evidence := contradictMass(index, symbol, first.Type, second.Type); mass > 0 {
		graph.strengthen(at, symbol, first.Type, second.Type, Contradicts, mass, evidence)
	}

	if mass, evidence := contradictMass(index, symbol, second.Type, first.Type); mass > 0 {
		graph.strengthen(at, symbol, second.Type, first.Type, Contradicts, mass, evidence)
	}

	if mass, evidence := conditionsMass(first, second); mass > 0 {
		graph.strengthen(at, symbol, first.Type, second.Type, Conditions, mass, evidence)
	}

	if mass, evidence := conditionsMass(second, first); mass > 0 {
		graph.strengthen(at, symbol, second.Type, first.Type, Conditions, mass, evidence)
	}

	leftClock := index.clockFor(symbol, first.Supporting)
	rightClock := index.clockFor(symbol, second.Supporting)

	if leftClock.ok && rightClock.ok && !alignable(leftClock, rightClock) {
		evidence := append(append([]string{}, first.Supporting...), second.Supporting...)
		mass := math.Sqrt(first.Strength * second.Strength)
		graph.strengthen(at, symbol, first.Type, second.Type, IncomparableWith, mass, evidence)
		graph.strengthen(at, symbol, second.Type, first.Type, IncomparableWith, mass, evidence)
		return
	}

	if mass := staleMass(leftClock, rightClock, first.Strength, second.Strength); mass > 0 {
		graph.strengthen(
			at, symbol, first.Type, second.Type, StaleRelativeTo, mass, first.Supporting,
		)
	}

	if mass := staleMass(rightClock, leftClock, second.Strength, first.Strength); mass > 0 {
		graph.strengthen(
			at, symbol, second.Type, first.Type, StaleRelativeTo, mass, second.Supporting,
		)
	}

	if mass := leadMass(leftClock, rightClock, first.Strength, second.Strength); mass > 0 {
		graph.strengthen(at, symbol, first.Type, second.Type, Leads, mass, first.Supporting)
		graph.strengthen(at, symbol, second.Type, first.Type, Lags, mass, second.Supporting)
	}

	if mass := leadMass(rightClock, leftClock, second.Strength, first.Strength); mass > 0 {
		graph.strengthen(at, symbol, second.Type, first.Type, Leads, mass, second.Supporting)
		graph.strengthen(at, symbol, first.Type, second.Type, Lags, mass, first.Supporting)
	}

	contradicts := graph.Weight(symbol, first.Type, second.Type, Contradicts) > 0 ||
		graph.Weight(symbol, second.Type, first.Type, Contradicts) > 0

	if jaccard > 0 || contradicts {
		return
	}

	evidence := append(append([]string{}, first.Supporting...), second.Supporting...)
	metricMass, metricEvidence := index.independence(symbol)
	pairMass, independent := graph.pair.independent(
		symbol, first.Type, second.Type, first.Strength, second.Strength,
	)

	if independent || metricMass > 0 {
		mass := math.Sqrt(first.Strength * second.Strength)

		if independent {
			mass = pairMass
		}

		if metricMass > 0 {
			mass = math.Sqrt(mass * metricMass)
			evidence = append(evidence, metricEvidence...)
		}

		graph.strengthen(at, symbol, first.Type, second.Type, IndependentOf, mass, evidence)
		graph.strengthen(at, symbol, second.Type, first.Type, IndependentOf, mass, evidence)
		return
	}

	mass := math.Sqrt(first.Strength * second.Strength)
	graph.strengthen(at, symbol, first.Type, second.Type, Supports, mass, evidence)
	graph.strengthen(at, symbol, second.Type, first.Type, Supports, mass, evidence)
}
