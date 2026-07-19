package types

import (
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
EdgeType names a relationship justified between two evidence references.
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
Node retains one immutable measurement only while it participates in a typed
evidence relationship.
*/
type Node struct {
	Key         string
	Measurement Measurement
}

/*
Edge connects two evidence keys with its relationship and observed interval.
*/
type Edge struct {
	From         string
	To           string
	Type         EdgeType
	At           time.Time
	ObservedFrom time.Time
}

/*
Graph owns the typed evidence relationships for one symbol. Candidate nodes are
retained during composition and pruned when no justified edge references them.
*/
type Graph struct {
	Symbol string
	At     time.Time
	nodes  map[string]*Node
	edges  []*Edge
	seen   map[string]struct{}
}

/*
NewGraph starts an empty symbol-local evidence graph.
*/
func NewGraph(symbol string) *Graph {
	return &Graph{
		Symbol: symbol,
		nodes:  make(map[string]*Node),
		edges:  make([]*Edge, 0),
		seen:   make(map[string]struct{}),
	}
}

/*
AddNode validates and stages one immutable measurement for relationship
composition. Staged evidence without a relationship is removed by Compose.
*/
func (evidenceGraph *Graph) AddNode(measurement *Measurement) error {
	if measurement == nil {
		return errnie.Validate((*Measurement)(nil))
	}

	if err := errnie.Validate(measurement); err != nil {
		return err
	}

	if measurement.Symbol != evidenceGraph.Symbol {
		return errnie.Err(
			errnie.Validation, "measurement symbol does not match graph", nil,
		).With("graph", evidenceGraph.Symbol, "measurement", measurement.Symbol)
	}

	key := MeasurementKey(measurement)

	if _, exists := evidenceGraph.nodes[key]; exists {
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

	evidenceGraph.nodes[key] = &Node{Key: key, Measurement: retained}

	if evidenceGraph.At.IsZero() || measurement.At.After(evidenceGraph.At) {
		evidenceGraph.At = measurement.At
	}

	return nil
}

/*
Relate records one typed relationship between existing evidence references and
rejects an equivalent relationship already present.
*/
func (evidenceGraph *Graph) Relate(
	fromKey string,
	toKey string,
	edgeType EdgeType,
	at time.Time,
	observedFrom time.Time,
) bool {
	if fromKey == "" || toKey == "" || edgeType == "" ||
		evidenceGraph.nodes[fromKey] == nil || evidenceGraph.nodes[toKey] == nil {
		return false
	}

	identity := edgeKey(fromKey, toKey, edgeType, at, observedFrom)

	if _, exists := evidenceGraph.seen[identity]; exists {
		return false
	}

	evidenceGraph.edges = append(evidenceGraph.edges, &Edge{
		From: fromKey, To: toKey, Type: edgeType,
		At: at, ObservedFrom: observedFrom,
	})
	evidenceGraph.seen[identity] = struct{}{}

	if evidenceGraph.At.IsZero() || at.After(evidenceGraph.At) {
		evidenceGraph.At = at
	}

	return true
}

/*
Nodes returns the evidence currently owned by the graph.
*/
func (evidenceGraph *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(evidenceGraph.nodes))

	for _, node := range evidenceGraph.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

/*
Edges returns the typed relationships currently owned by the graph.
*/
func (evidenceGraph *Graph) Edges() []*Edge {
	return evidenceGraph.edges
}

/*
prune removes staged measurements that did not participate in a relationship.
*/
func (evidenceGraph *Graph) prune() {
	referenced := make(map[string]struct{}, len(evidenceGraph.edges)*2)

	for _, edge := range evidenceGraph.edges {
		referenced[edge.From] = struct{}{}
		referenced[edge.To] = struct{}{}
	}

	for key := range evidenceGraph.nodes {
		if _, ok := referenced[key]; !ok {
			delete(evidenceGraph.nodes, key)
		}
	}
}

/*
MeasurementKey returns the stable evidence reference used by nodes and edges.
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
