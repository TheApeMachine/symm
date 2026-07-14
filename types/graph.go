package types

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/gonum/graph/multi"
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
	graph  *multi.DirectedGraph
	nodes  map[string]int64
	lines  map[[3]int64]Edge
}

/*
NewGraph starts an empty relationship graph for one symbol.
*/
func NewGraph(symbol string) *Graph {
	return &Graph{
		Symbol: symbol,
		Nodes:  []Node{},
		Edges:  []Edge{},
		graph:  multi.NewDirectedGraph(),
		nodes:  map[string]int64{},
		lines:  map[[3]int64]Edge{},
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

	if _, exists := graph.nodes[key]; exists {
		return false
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
	topologyNode := graph.graph.NewNode()
	graph.graph.AddNode(topologyNode)
	graph.nodes[key] = topologyNode.ID()

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

	fromID, fromExists := graph.nodes[fromKey]
	toID, toExists := graph.nodes[toKey]

	if !fromExists || !toExists {
		return false
	}

	edge := Edge{
		Type:         edgeType,
		From:         fromKey,
		To:           toKey,
		At:           at,
		ObservedFrom: observedFrom,
	}

	lines := graph.graph.Lines(fromID, toID)

	for lines.Next() {
		if graph.lines[[3]int64{fromID, toID, lines.Line().ID()}] == edge {
			return false
		}
	}

	line := graph.graph.NewLine(graph.graph.Node(fromID), graph.graph.Node(toID))
	graph.graph.SetLine(line)
	graph.lines[[3]int64{fromID, toID, line.ID()}] = edge
	graph.Edges = append(graph.Edges, edge)

	if graph.At.IsZero() || at.After(graph.At) {
		graph.At = at
	}

	return true
}

/*
Compose derives relationships between chronological neighbors of each shared
observable. A neighbor chain preserves temporal order and conflict without
materializing the redundant transitive closure of a busy event stream.
*/
func (graph *Graph) Compose() {
	observables := map[string][]Node{}

	for _, node := range graph.Nodes {
		measurement := node.Measurement

		if measurement.Metric == "" || measurement.Subject == "" {
			continue
		}

		key := string(measurement.Metric) + "\x00" +
			string(measurement.Subject) + "\x00" + string(measurement.Side)
		observables[key] = append(observables[key], node)
	}

	for _, nodes := range observables {
		sort.Slice(nodes, func(leftIndex, rightIndex int) bool {
			left := nodes[leftIndex]
			right := nodes[rightIndex]

			if left.Measurement.At.Equal(right.Measurement.At) {
				return left.Key < right.Key
			}

			return left.Measurement.At.Before(right.Measurement.At)
		})

		for index := 1; index < len(nodes); index++ {
			graph.compose(nodes[index-1], nodes[index])
		}
	}
}

/*
compose derives every direct relationship justified between two neighboring
observations of the same metric, subject, and side.
*/
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

/*
composeDirection preserves signed agreement or contradiction independently of
the temporal relationship between the same observations.
*/
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

/*
composeTime records direct lead and lag edges when neighboring evidence
intervals do not overlap.
*/
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

/*
composeStale marks an older observation when its own horizon expired before
the neighboring observation arrived.
*/
func (graph *Graph) composeStale(
	older, newer Node, at, observedFrom time.Time,
) {
	horizon := older.Measurement.Horizon

	if horizon > 0 && older.Measurement.At.Add(horizon).Before(newer.Measurement.At) {
		graph.Relate(older.Key, newer.Key, Stale, at, observedFrom)
	}
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
