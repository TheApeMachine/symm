package types

import (
	"strconv"
	"time"
)

/*
edgeKey returns the stable identity used to deduplicate one typed relationship.
*/
func edgeKey(
	from string,
	to string,
	edgeType EdgeType,
	at time.Time,
	observedFrom time.Time,
) string {
	return from + "\x00" + to + "\x00" + string(edgeType) + "\x00" +
		strconv.FormatInt(at.UnixNano(), 10) + "\x00" +
		strconv.FormatInt(observedFrom.UnixNano(), 10)
}

/*
Compose derives the current relationship between the two newest observations
of each shared observable. Older measurements remain analysis inputs, but they
do not belong in the live graph once a newer direct relationship supersedes
them.
*/
func (evidenceGraph *Graph) Compose() {
	for _, window := range evidenceGraph.Evidence.observations {
		if window.older != nil && window.newer != nil {
			evidenceGraph.compose(window.older, window.newer)
		}
	}

	evidenceGraph.Evidence.composeCategories()
	evidenceGraph.prune()
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
