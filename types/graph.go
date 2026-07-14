package types

import (
	"math"
	"strconv"
	"strings"
	"time"
)

/*
EdgeType names how two measurement nodes relate within one symbol epoch.
*/
type EdgeType string

const (
	Supports     EdgeType = "supports"
	Contradicts  EdgeType = "contradicts"
	Conditions   EdgeType = "conditions"
	Leads        EdgeType = "leads"
	Lags         EdgeType = "lags"
	Redundant    EdgeType = "redundant"
	Independent  EdgeType = "independent"
	Stale        EdgeType = "stale"
	Incomparable EdgeType = "incomparable"
)

/*
Node is one typed measurement reference retained for graph provenance.
*/
type Node struct {
	Key         string
	Measurement Measurement
}

/*
Edge records a directed relationship with the evaluation time and the evidence
interval that justified it.
*/
type Edge struct {
	Type         EdgeType
	From         string
	To           string
	At           time.Time
	ObservedFrom time.Time
}

/*
Graph holds measurement nodes and their relationships for one symbol epoch.
*/
type Graph struct {
	Symbol string
	At     time.Time
	Nodes  []Node
	Edges  []Edge
}

/*
NewGraph starts an empty relationship graph for one symbol.
*/
func NewGraph(symbol string) *Graph {
	return &Graph{
		Symbol: symbol,
		Nodes:  []Node{},
		Edges:  []Edge{},
	}
}

/*
AddNode retains one measurement when it belongs to this graph's symbol.
*/
func (graph *Graph) AddNode(measurement *Measurement) bool {
	if measurement == nil || measurement.Symbol != graph.Symbol {
		return false
	}

	key := measurementKey(measurement)

	for _, node := range graph.Nodes {
		if node.Key == key {
			return false
		}
	}

	retained := *measurement

	if measurement.Normalized != nil {
		normalized := *measurement.Normalized
		retained.Normalized = &normalized
	}

	if measurement.Uncertainty != nil {
		uncertainty := *measurement.Uncertainty
		retained.Uncertainty = &uncertainty
	}

	graph.Nodes = append(graph.Nodes, Node{
		Key:         key,
		Measurement: retained,
	})

	if graph.At.IsZero() || measurement.At.After(graph.At) {
		graph.At = measurement.At
	}

	return true
}

/*
Relate appends one directed edge between existing node keys.
*/
func (graph *Graph) Relate(
	fromKey string,
	toKey string,
	edgeType EdgeType,
	at time.Time,
	observedFrom time.Time,
) bool {
	if fromKey == "" || toKey == "" || edgeType == "" {
		return false
	}

	if !graph.hasNode(fromKey) || !graph.hasNode(toKey) {
		return false
	}

	for _, edge := range graph.Edges {
		if edge.Type == edgeType && edge.From == fromKey && edge.To == toKey &&
			edge.At.Equal(at) && edge.ObservedFrom.Equal(observedFrom) {
			return false
		}
	}

	graph.Edges = append(graph.Edges, Edge{
		Type:         edgeType,
		From:         fromKey,
		To:           toKey,
		At:           at,
		ObservedFrom: observedFrom,
	})

	if graph.At.IsZero() || at.After(graph.At) {
		graph.At = at
	}

	return true
}

/*
Compose derives only relationships proven by shared measurement identity,
signed normalization, and event-time intervals.
*/
func (graph *Graph) Compose() {
	for leftIndex, left := range graph.Nodes {
		for rightIndex := leftIndex + 1; rightIndex < len(graph.Nodes); rightIndex++ {
			graph.compose(left, graph.Nodes[rightIndex])
		}
	}
}

func (graph *Graph) compose(left, right Node) {
	if left.Measurement.Validity.State != ValidityValid ||
		right.Measurement.Validity.State != ValidityValid {
		return
	}

	if !sameObservable(left.Measurement, right.Measurement) {
		return
	}

	at, observedFrom := relationshipInterval(left.Measurement, right.Measurement)

	if left.Measurement.Unit != right.Measurement.Unit ||
		left.Measurement.Scale.Kind != right.Measurement.Scale.Kind {
		graph.Relate(left.Key, right.Key, Incomparable, at, observedFrom)
		return
	}

	graph.composeDirection(left, right, at, observedFrom)
	graph.composeTime(left, right, at, observedFrom)
}

func (graph *Graph) composeDirection(
	left, right Node, at, observedFrom time.Time,
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
		graph.Relate(from.Key, to.Key, Contradicts, at, observedFrom)
		return
	}

	graph.Relate(from.Key, to.Key, Supports, at, observedFrom)
}

func (graph *Graph) composeTime(
	left, right Node, at, observedFrom time.Time,
) {
	leftStart, leftEnd := evidenceInterval(left.Measurement)
	rightStart, rightEnd := evidenceInterval(right.Measurement)

	if leftEnd.Before(rightStart) {
		graph.Relate(left.Key, right.Key, Leads, at, observedFrom)
		graph.Relate(right.Key, left.Key, Lags, at, observedFrom)
		graph.composeStale(left, right, at, observedFrom)
		return
	}

	if rightEnd.Before(leftStart) {
		graph.Relate(right.Key, left.Key, Leads, at, observedFrom)
		graph.Relate(left.Key, right.Key, Lags, at, observedFrom)
		graph.composeStale(right, left, at, observedFrom)
	}
}

func (graph *Graph) composeStale(
	older, newer Node, at, observedFrom time.Time,
) {
	horizon := older.Measurement.Horizon

	if horizon > 0 && older.Measurement.At.Add(horizon).Before(newer.Measurement.At) {
		graph.Relate(older.Key, newer.Key, Stale, at, observedFrom)
	}
}

func (graph *Graph) hasNode(key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}

	return false
}

func sameObservable(left, right Measurement) bool {
	return left.Metric != "" &&
		left.Subject != "" &&
		left.Metric == right.Metric &&
		left.Subject == right.Subject &&
		left.Side == right.Side
}

func chronologicalNodes(left, right Node) (Node, Node) {
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

/*
MeasurementKey returns the stable identity used by graph nodes and category
evidence references.
*/
func MeasurementKey(measurement *Measurement) string {
	return measurementKey(measurement)
}

func measurementKey(measurement *Measurement) string {
	var key strings.Builder

	writeText := func(value string) {
		if key.Len() > 0 {
			key.WriteByte('/')
		}

		key.WriteString(value)
	}
	writeInt := func(value int64) {
		var buffer [32]byte

		key.WriteByte('/')
		key.Write(strconv.AppendInt(buffer[:0], value, 10))
	}

	writeText(string(measurement.Source))
	writeText(string(measurement.Stream))
	writeText(string(measurement.Metric))
	writeText(string(measurement.Subject))
	writeText(string(measurement.Side))
	writeText(measurement.Symbol)
	writeInt(measurement.At.UnixNano())
	writeInt(measurement.ObservedFrom.UnixNano())
	writeInt(int64(measurement.Horizon))
	writeText(string(measurement.Unit))
	writeText(string(measurement.Scale.Kind))
	writeInt(measurement.Scale.From.UnixNano())
	writeInt(measurement.Scale.Through.UnixNano())

	return key.String()
}
