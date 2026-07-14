package types

import (
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
)

var graphMeasurementValidator = errnie.New()

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
Node is a Gonum node carrying one immutable measurement as graph evidence.
*/
type Node struct {
	id          int64
	Key         string
	Measurement Measurement
}

/*
ID returns the Gonum identity allocated by the graph that owns the node.
*/
func (node *Node) ID() int64 {
	return node.id
}

/*
Edge is a Gonum line carrying the relationship and temporal provenance that
connect two measurement nodes.
*/
type Edge struct {
	line         graph.Line
	Type         EdgeType
	At           time.Time
	ObservedFrom time.Time
}

/*
From returns the source evidence node for this directed relationship.
*/
func (edge *Edge) From() graph.Node {
	return edge.line.From()
}

/*
To returns the destination evidence node for this directed relationship.
*/
func (edge *Edge) To() graph.Node {
	return edge.line.To()
}

/*
ReversedLine returns the same evidence relationship with its direction reversed.
*/
func (edge *Edge) ReversedLine() graph.Line {
	reversed := *edge
	reversed.line = edge.line.ReversedLine()

	return &reversed
}

/*
ID returns the Gonum identity allocated for this line between its endpoints.
*/
func (edge *Edge) ID() int64 {
	return edge.line.ID()
}

/*
Graph embeds Gonum's directed multigraph as the sole topology for one symbol.
The wrapper only supplies the domain operations used to compose evidence.
*/
type Graph struct {
	*multi.DirectedGraph
	Symbol  string
	At      time.Time
	nodeIDs map[string]int64
}

/*
NewGraph starts an empty Gonum relationship graph for one symbol.
*/
func NewGraph(symbol string) *Graph {
	return &Graph{
		DirectedGraph: multi.NewDirectedGraph(),
		Symbol:        symbol,
		nodeIDs:       make(map[string]int64),
	}
}

/*
AddNode validates and retains one immutable measurement as a Gonum node when it
belongs to this graph's symbol. Repeated evidence is an idempotent no-op.
*/
func (evidenceGraph *Graph) AddNode(measurement *Measurement) error {
	if measurement == nil {
		return graphMeasurementValidator.Validate((*Measurement)(nil))
	}

	if err := graphMeasurementValidator.Validate(measurement); err != nil {
		return err
	}

	if measurement.Symbol != evidenceGraph.Symbol {
		return errnie.Err(
			errnie.Validation, "measurement symbol does not match graph", nil,
		).With("graph", evidenceGraph.Symbol, "measurement", measurement.Symbol)
	}

	key := MeasurementKey(measurement)

	if _, exists := evidenceGraph.nodeIDs[key]; exists {
		return nil
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

	identity := evidenceGraph.NewNode().ID()
	evidenceGraph.DirectedGraph.AddNode(&Node{
		id: identity, Key: key, Measurement: retained,
	})
	evidenceGraph.nodeIDs[key] = identity

	if evidenceGraph.At.IsZero() || measurement.At.After(evidenceGraph.At) {
		evidenceGraph.At = measurement.At
	}

	return nil
}

/*
Relate adds one typed Gonum line between existing measurement node keys while
rejecting an equivalent relationship already present between those nodes.
*/
func (evidenceGraph *Graph) Relate(
	fromKey string,
	toKey string,
	edgeType EdgeType,
	at time.Time,
	observedFrom time.Time,
) bool {
	if fromKey == "" || toKey == "" || edgeType == "" {
		return false
	}

	from := evidenceGraph.node(fromKey)
	to := evidenceGraph.node(toKey)

	if from == nil || to == nil {
		return false
	}

	lines := evidenceGraph.Lines(from.ID(), to.ID())

	for lines.Next() {
		existing := lines.Line().(*Edge)

		if existing.Type == edgeType && existing.At.Equal(at) &&
			existing.ObservedFrom.Equal(observedFrom) {
			return false
		}
	}

	line := evidenceGraph.NewLine(from, to)
	evidenceGraph.SetLine(&Edge{
		line: line, Type: edgeType, At: at, ObservedFrom: observedFrom,
	})

	if evidenceGraph.At.IsZero() || at.After(evidenceGraph.At) {
		evidenceGraph.At = at
	}

	return true
}

/*
node resolves one key through its Gonum node ID. The index contains identities
only; Gonum remains the sole owner of node values and graph topology.
*/
func (evidenceGraph *Graph) node(key string) *Node {
	identity, exists := evidenceGraph.nodeIDs[key]

	if !exists {
		return nil
	}

	return evidenceGraph.Node(identity).(*Node)
}

/*
MeasurementKey returns the stable identity used by graph nodes and category
evidence references.
*/
func MeasurementKey(measurement *Measurement) string {
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
