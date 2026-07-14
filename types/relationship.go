package types

import (
	"sort"
	"time"
)

/*
Compose derives relationships between chronological neighbors of each shared
observable without materializing a redundant transitive closure. AddNode has
already established each measurement's structural invariants.
*/
func (evidenceGraph *Graph) Compose() {
	observables := make(map[string][]*Node)
	nodes := evidenceGraph.Nodes()

	for nodes.Next() {
		node := nodes.Node().(*Node)
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

		for index := 1; index < len(observable); index++ {
			evidenceGraph.compose(observable[index-1], observable[index])
		}
	}
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
composeDirection records signed agreement between validated finite values.
Zero and absent normalization carry no direction and therefore add no edge.
*/
func (evidenceGraph *Graph) composeDirection(
	older, newer *Node,
	observedFrom time.Time,
) {
	olderValue := older.Measurement.Normalized
	newerValue := newer.Measurement.Normalized

	if olderValue == nil || newerValue == nil ||
		*olderValue == 0 || *newerValue == 0 {
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
