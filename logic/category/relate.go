package category

import (
	"math"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
contradictMass sums indexed live mass for affinity contradictions from→to.
*/
func contradictMass(
	index *evidenceIndex,
	symbol string,
	from, to types.CategoryType,
) (mass float64) {
	targets := contradictIndex[from]

	if len(targets) == 0 {
		return 0
	}

	for _, metric := range targets[to] {
		value := index.metricMass(symbol, metric)

		if value <= 0 {
			continue
		}

		mass += value
	}

	return mass
}

/*
contradictEvidence writes contradiction metric names into graph scratch only when
the already-computed mass proves an edge exists. The hot no-edge path therefore
does not allocate or build evidence lists that will be thrown away.
*/
func (graph *Graph) contradictEvidence(
	index *evidenceIndex,
	symbol string,
	from, to types.CategoryType,
) []string {
	graph.evidenceScratch = graph.evidenceScratch[:0]

	for _, metric := range contradictIndex[from][to] {
		if index.metricMass(symbol, metric) > 0 {
			graph.evidenceScratch = append(graph.evidenceScratch, string(metric))
		}
	}

	return graph.evidenceScratch
}

/*
sharedSupport returns the Jaccard overlap of supporting metric keys and the
shared key list. RedundantWith is exactly this overlap under live activation.
*/
func (graph *Graph) sharedSupport(left, right []string) (float64, []string) {
	if len(left) == 0 || len(right) == 0 {
		return 0, nil
	}

	graph.scratch = graph.scratch[:0]

	for _, metric := range right {
		if hasMetric(left, metric) {
			graph.scratch = append(graph.scratch, metric)
		}
	}

	union := len(left) + len(right) - len(graph.scratch)

	if union == 0 || len(graph.scratch) == 0 {
		return 0, nil
	}

	return float64(len(graph.scratch)) / float64(union), graph.scratch
}

/*
conditionsMass is the strength contribution when A's supporting metrics fill
B's missing required evidence — A Conditions B.
*/
func (graph *Graph) conditionsMass(provider, dependent types.Category) (float64, []string) {
	if len(provider.Supporting) == 0 || len(dependent.Missing) == 0 {
		return 0, nil
	}

	graph.scratch = graph.scratch[:0]

	for _, metric := range dependent.Missing {
		if hasMetric(provider.Supporting, metric) {
			graph.scratch = append(graph.scratch, metric)
		}
	}

	if len(graph.scratch) == 0 {
		return 0, nil
	}

	mass := provider.Strength * dependent.Strength *
		float64(len(graph.scratch)) / float64(len(dependent.Missing))

	return mass, graph.scratch
}

/*
hasMetric reports whether a category evidence list contains metric. Category
support lists are tiny and hot, so a direct scan avoids allocating transient maps
inside every pair relation while keeping the evidence comparison explicit.
*/
func hasMetric(metrics []string, target string) bool {
	for _, metric := range metrics {
		if metric == target {
			return true
		}
	}

	return false
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

	jaccard, shared := graph.sharedSupport(first.Supporting, second.Supporting)
	graph.linkRedundantContradictsConditions(
		at, index, symbol, first, second, jaccard, shared,
	)

	leftClock := index.clockFor(symbol, first.Supporting)
	rightClock := index.clockFor(symbol, second.Supporting)

	if graph.linkIncomparableStaleLeads(
		at, symbol, first, second, leftClock, rightClock,
	) {
		return
	}

	graph.linkIndependentOrSupports(at, index, symbol, first, second, jaccard)
}

/*
linkRedundantContradictsConditions strengthens overlap, contradiction, and
conditional evidence edges between one category pair.
*/
func (graph *Graph) linkRedundantContradictsConditions(
	at time.Time,
	index *evidenceIndex,
	symbol string,
	first, second types.Category,
	jaccard float64,
	shared []string,
) {
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

	if mass := contradictMass(index, symbol, first.Type, second.Type); mass > 0 {
		graph.strengthen(
			at, symbol, first.Type, second.Type, Contradicts, mass,
			graph.contradictEvidence(index, symbol, first.Type, second.Type),
		)
	}

	if mass := contradictMass(index, symbol, second.Type, first.Type); mass > 0 {
		graph.strengthen(
			at, symbol, second.Type, first.Type, Contradicts, mass,
			graph.contradictEvidence(index, symbol, second.Type, first.Type),
		)
	}

	if mass, evidence := graph.conditionsMass(first, second); mass > 0 {
		graph.strengthen(at, symbol, first.Type, second.Type, Conditions, mass, evidence)
	}

	if mass, evidence := graph.conditionsMass(second, first); mass > 0 {
		graph.strengthen(at, symbol, second.Type, first.Type, Conditions, mass, evidence)
	}
}

/*
linkIncomparableStaleLeads strengthens temporal envelope edges and reports
whether the pair is incomparable and further coupling should stop.
*/
func (graph *Graph) linkIncomparableStaleLeads(
	at time.Time,
	symbol string,
	first, second types.Category,
	leftClock, rightClock evidenceClock,
) bool {
	if leftClock.ok && rightClock.ok && !alignable(leftClock, rightClock) {
		mass := math.Sqrt(first.Strength * second.Strength)
		graph.strengthenJoined(
			at, symbol, first.Type, second.Type, IncomparableWith, mass,
			first.Supporting, second.Supporting,
		)
		graph.strengthenJoined(
			at, symbol, second.Type, first.Type, IncomparableWith, mass,
			second.Supporting, first.Supporting,
		)
		return true
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

	return false
}

/*
linkIndependentOrSupports strengthens independence or default support after
overlap and contradiction edges are ruled out.
*/
func (graph *Graph) linkIndependentOrSupports(
	at time.Time,
	index *evidenceIndex,
	symbol string,
	first, second types.Category,
	jaccard float64,
) {
	contradicts := graph.Weight(symbol, first.Type, second.Type, Contradicts) > 0 ||
		graph.Weight(symbol, second.Type, first.Type, Contradicts) > 0

	if jaccard > 0 || contradicts {
		return
	}

	metricMass := index.independence(symbol, first.Type, second.Type)
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
			graph.scratch = append(graph.scratch[:0], second.Supporting...)
			graph.scratch = index.appendIndependence(graph.scratch, symbol, first.Type, second.Type)
			graph.strengthenJoined(
				at, symbol, first.Type, second.Type, IndependentOf, mass,
				first.Supporting, graph.scratch,
			)

			graph.scratch = append(graph.scratch[:0], first.Supporting...)
			graph.scratch = index.appendIndependence(graph.scratch, symbol, first.Type, second.Type)
			graph.strengthenJoined(
				at, symbol, second.Type, first.Type, IndependentOf, mass,
				second.Supporting, graph.scratch,
			)
			return
		}

		graph.strengthenJoined(
			at, symbol, first.Type, second.Type, IndependentOf, mass,
			first.Supporting, second.Supporting,
		)
		graph.strengthenJoined(
			at, symbol, second.Type, first.Type, IndependentOf, mass,
			second.Supporting, first.Supporting,
		)
		return
	}

	mass := math.Sqrt(first.Strength * second.Strength)
	graph.strengthenJoined(
		at, symbol, first.Type, second.Type, Supports, mass,
		first.Supporting, second.Supporting,
	)
	graph.strengthenJoined(
		at, symbol, second.Type, first.Type, Supports, mass,
		second.Supporting, first.Supporting,
	)
}
