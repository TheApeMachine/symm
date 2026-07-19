package types

import (
	"sort"
	"time"
)

/*
Compose derives the current relationship between the two newest observations
of each shared observable. Older measurements remain analysis inputs, but they
do not belong in the live graph once a newer direct relationship supersedes
them.
*/
func (evidenceGraph *Graph) Compose() {
	observables := make(map[string][]*Node)

	for _, node := range evidenceGraph.Nodes() {
		measurement := node.Measurement

		if measurement.Metric == "" || measurement.Subject == "" {
			continue
		}

		key := string(measurement.Metric) + "\x00" +
			string(measurement.Subject) + "\x00" + string(measurement.Side)
		observables[key] = append(observables[key], node)
	}

	for _, observable := range observables {
		sort.Slice(observable, func(olderIndex, newerIndex int) bool {
			older := observable[olderIndex]
			newer := observable[newerIndex]

			if older.Measurement.At.Equal(newer.Measurement.At) {
				return older.Key < newer.Key
			}

			return older.Measurement.At.Before(newer.Measurement.At)
		})

		if len(observable) > 1 {
			latest := len(observable) - 1
			evidenceGraph.compose(observable[latest-1], observable[latest])
		}
	}

	evidenceGraph.composeCategories()
	evidenceGraph.prune()
}

/*
composeCategories relates each measurement to the category hypotheses it is
evidence for or against. A measurement whose normalized magnitude is meaningful
casts a Supports edge to every category its metric supports and a Contradicts
edge to every category its metric opposes, per CategoryAffinity. This is the
cross-observable structure the graph exists to show: many signals converging on,
or disputing, the same category.
*/
func (evidenceGraph *Graph) composeCategories() {
	for _, node := range evidenceGraph.Nodes() {
		if node.Kind != NodeMeasurement {
			continue
		}

		measurement := node.Measurement

		if measurement.Validity.State != ValidityValid {
			continue
		}

		affinity, ok := AffinityFor(measurement.Metric)

		if !ok {
			continue
		}

		if !categoryEvidenceActive(measurement.Normalized) {
			continue
		}

		observedFrom, _ := measurement.Interval()

		for _, category := range affinity.Supports {
			categoryKey := evidenceGraph.ensureCategory(category, measurement.At)
			evidenceGraph.Relate(
				node.Key, categoryKey, Supports, measurement.At, observedFrom,
			)
		}

		for _, category := range affinity.Opposes {
			categoryKey := evidenceGraph.ensureCategory(category, measurement.At)
			evidenceGraph.Relate(
				node.Key, categoryKey, Contradicts, measurement.At, observedFrom,
			)
		}
	}
}

/*
categoryEvidenceActive reports whether a normalized reading is on strongly enough
to count as evidence. A nil or zero reading has no direction and stays out of the
category graph so absent evidence never lights a hypothesis.
*/
func categoryEvidenceActive(normalized *float64) bool {
	return normalized != nil && *normalized > 0
}

/*
compose derives every direct relationship justified between two neighboring
observations whose chronological order was established by Compose.
*/
func (evidenceGraph *Graph) compose(older, newer *Node) {
	if older.Measurement.Validity.State != ValidityValid ||
		newer.Measurement.Validity.State != ValidityValid {
		return
	}

	observedFrom, _ := older.Measurement.Interval()
	newerFrom, _ := newer.Measurement.Interval()

	if newerFrom.Before(observedFrom) {
		observedFrom = newerFrom
	}

	if older.Measurement.Unit != newer.Measurement.Unit ||
		older.Measurement.Scale.Kind != newer.Measurement.Scale.Kind {
		evidenceGraph.Relate(
			older.Key, newer.Key, Incomparable,
			newer.Measurement.At, observedFrom,
		)

		return
	}

	evidenceGraph.composeDirection(older, newer, observedFrom)
	evidenceGraph.composeTime(older, newer, observedFrom)
}

/*
composeDirection records exact repetition as redundancy and signed agreement or
conflict between distinct finite values. An unmatched zero has no direction.
*/
func (evidenceGraph *Graph) composeDirection(
	older, newer *Node,
	observedFrom time.Time,
) {
	olderValue := older.Measurement.Normalized
	newerValue := newer.Measurement.Normalized

	if olderValue == nil || newerValue == nil {
		return
	}

	if *olderValue == *newerValue {
		evidenceGraph.Relate(
			older.Key, newer.Key, Redundant,
			newer.Measurement.At, observedFrom,
		)

		return
	}

	if *olderValue == 0 || *newerValue == 0 {
		return
	}

	edgeType := Supports

	if (*olderValue > 0) != (*newerValue > 0) {
		edgeType = Contradicts
	}

	evidenceGraph.Relate(
		older.Key, newer.Key, edgeType,
		newer.Measurement.At, observedFrom,
	)
}

/*
composeTime records direct lead and lag lines when evidence intervals do not
overlap.
*/
func (evidenceGraph *Graph) composeTime(
	older, newer *Node,
	observedFrom time.Time,
) {
	olderStart, olderEnd := older.Measurement.Interval()
	newerStart, newerEnd := newer.Measurement.Interval()
	at := newer.Measurement.At

	if olderEnd.Before(newerStart) {
		evidenceGraph.Relate(older.Key, newer.Key, Leads, at, observedFrom)
		evidenceGraph.Relate(newer.Key, older.Key, Lags, at, observedFrom)
		evidenceGraph.composeStale(older, newer, observedFrom)

		return
	}

	if newerEnd.Before(olderStart) {
		evidenceGraph.Relate(newer.Key, older.Key, Leads, at, observedFrom)
		evidenceGraph.Relate(older.Key, newer.Key, Lags, at, observedFrom)
		evidenceGraph.composeStale(newer, older, observedFrom)
	}
}

/*
composeStale marks an older observation whose own horizon expired before the
neighboring observation arrived.
*/
func (evidenceGraph *Graph) composeStale(
	older, newer *Node,
	observedFrom time.Time,
) {
	horizon := older.Measurement.Horizon

	if horizon > 0 && older.Measurement.At.Add(horizon).Before(newer.Measurement.At) {
		evidenceGraph.Relate(
			older.Key, newer.Key, Stale,
			newer.Measurement.At, observedFrom,
		)
	}
}
