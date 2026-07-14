package types

import (
	"math"
	"sort"
	"time"
)

/*
Compose derives relationships between chronological neighbors of each shared
observable without materializing a redundant transitive closure.
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
		sort.Slice(observable, func(leftIndex, rightIndex int) bool {
			left := observable[leftIndex]
			right := observable[rightIndex]

			if left.Measurement.At.Equal(right.Measurement.At) {
				return left.Key < right.Key
			}

			return left.Measurement.At.Before(right.Measurement.At)
		})

		for index := 1; index < len(observable); index++ {
			evidenceGraph.compose(observable[index-1], observable[index])
		}
	}
}

/*
compose derives every direct relationship justified between two neighboring
observations of the same metric, subject, and side.
*/
func (evidenceGraph *Graph) compose(left, right *Node) {
	if left.Measurement.Validity.State != ValidityValid ||
		right.Measurement.Validity.State != ValidityValid {
		return
	}

	at, observedFrom := relationshipInterval(left.Measurement, right.Measurement)

	if left.Measurement.Unit != right.Measurement.Unit ||
		left.Measurement.Scale.Kind != right.Measurement.Scale.Kind {
		evidenceGraph.Relate(left.Key, right.Key, Incomparable, at, observedFrom)
		return
	}

	evidenceGraph.composeDirection(left, right, at, observedFrom)
	evidenceGraph.composeTime(left, right, at, observedFrom)
}

/*
composeDirection records signed agreement independently of temporal order.
*/
func (evidenceGraph *Graph) composeDirection(
	left, right *Node,
	at, observedFrom time.Time,
) {
	leftValue := left.Measurement.Normalized
	rightValue := right.Measurement.Normalized

	if leftValue == nil || rightValue == nil || *leftValue == 0 || *rightValue == 0 {
		return
	}

	if math.IsNaN(*leftValue) || math.IsInf(*leftValue, 0) ||
		math.IsNaN(*rightValue) || math.IsInf(*rightValue, 0) {
		return
	}

	from, to := chronologicalNodes(left, right)

	if (*leftValue > 0) != (*rightValue > 0) {
		evidenceGraph.Relate(from.Key, to.Key, Contradicts, at, observedFrom)
		return
	}

	evidenceGraph.Relate(from.Key, to.Key, Supports, at, observedFrom)
}

/*
composeTime records direct lead and lag lines when evidence intervals do not overlap.
*/
func (evidenceGraph *Graph) composeTime(
	left, right *Node,
	at, observedFrom time.Time,
) {
	leftStart, leftEnd := evidenceInterval(left.Measurement)
	rightStart, rightEnd := evidenceInterval(right.Measurement)

	if leftEnd.Before(rightStart) {
		evidenceGraph.Relate(left.Key, right.Key, Leads, at, observedFrom)
		evidenceGraph.Relate(right.Key, left.Key, Lags, at, observedFrom)
		evidenceGraph.composeStale(left, right, at, observedFrom)
		return
	}

	if rightEnd.Before(leftStart) {
		evidenceGraph.Relate(right.Key, left.Key, Leads, at, observedFrom)
		evidenceGraph.Relate(left.Key, right.Key, Lags, at, observedFrom)
		evidenceGraph.composeStale(right, left, at, observedFrom)
	}
}

/*
composeStale marks an older observation whose own horizon expired before the
neighboring observation arrived.
*/
func (evidenceGraph *Graph) composeStale(
	older, newer *Node,
	at, observedFrom time.Time,
) {
	horizon := older.Measurement.Horizon

	if horizon > 0 && older.Measurement.At.Add(horizon).Before(newer.Measurement.At) {
		evidenceGraph.Relate(older.Key, newer.Key, Stale, at, observedFrom)
	}
}

func chronologicalNodes(left, right *Node) (*Node, *Node) {
	if left.Measurement.At.Before(right.Measurement.At) {
		return left, right
	}

	if right.Measurement.At.Before(left.Measurement.At) || right.Key < left.Key {
		return right, left
	}

	return left, right
}

func relationshipInterval(left, right Measurement) (time.Time, time.Time) {
	at := left.At

	if right.At.After(at) {
		at = right.At
	}

	leftStart, _ := evidenceInterval(left)
	rightStart, _ := evidenceInterval(right)
	observedFrom := leftStart

	if rightStart.Before(observedFrom) {
		observedFrom = rightStart
	}

	return at, observedFrom
}

func evidenceInterval(measurement Measurement) (time.Time, time.Time) {
	start := measurement.ObservedFrom

	if start.IsZero() {
		start = measurement.Scale.From
	}

	if start.IsZero() {
		start = measurement.At
	}

	end := measurement.Scale.Through

	if end.IsZero() {
		end = measurement.At
	}

	return start, end
}
