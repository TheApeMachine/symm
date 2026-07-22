package types

import (
	"encoding/json"
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
NodeKind distinguishes a measurement node from a synthetic category node. A
category node has no reading of its own; it is the hypothesis that measurement
nodes support or contradict.
*/
type NodeKind string

const (
	NodeMeasurement NodeKind = "measurement"
	NodeCategory    NodeKind = "category"
	NodeConcept     NodeKind = "concept"
)

/*
Node retains one immutable measurement only while it participates in a typed
evidence relationship. A category node carries a descriptive Measurement shell
(Source category, Metric = category type) so the UI treats it uniformly.
*/
type Node struct {
	Key         string       `json:"key"`
	Kind        NodeKind     `json:"kind"`
	Category    CategoryType `json:"category,omitempty"`
	Measurement Measurement  `json:"measurement"`
}

/*
Edge connects two evidence keys with its relationship and observed interval.
*/
type Edge struct {
	From         string    `json:"from"`
	To           string    `json:"to"`
	Type         EdgeType  `json:"type"`
	At           time.Time `json:"at"`
	ObservedFrom time.Time `json:"observedFrom"`
}

/*
Graph owns generic typed nodes and relationships for one symbol. Its composed
Evidence object owns market-specific observation and category construction;
candidate nodes are pruned when no justified edge references them.
*/
type Graph struct {
	Symbol   string
	At       time.Time
	nodes    map[string]*Node
	edges    []*Edge
	seen     map[string]struct{}
	Evidence *EvidenceComposer `json:"-"`
}

/*
NewGraph starts an empty symbol-local evidence graph.
*/
func NewGraph(symbol string) *Graph {
	graph := &Graph{
		Symbol: symbol,
		nodes:  make(map[string]*Node),
		edges:  make([]*Edge, 0),
		seen:   make(map[string]struct{}),
	}
	graph.Evidence = newEvidenceComposer(graph)

	return graph
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

	node := &Node{
		Key:         key,
		Kind:        NodeMeasurement,
		Measurement: retained,
	}
	evidenceGraph.Evidence.stage(node)

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
		From:         fromKey,
		To:           toKey,
		Type:         edgeType,
		At:           at,
		ObservedFrom: observedFrom,
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
MarshalJSON encodes the symbol-local topology with nodes as a JSON array.
*/
func (evidenceGraph *Graph) MarshalJSON() ([]byte, error) {
	nodes := make([]*Node, 0, len(evidenceGraph.nodes))

	for _, node := range evidenceGraph.nodes {
		nodes = append(nodes, node)
	}

	return json.Marshal(struct {
		Symbol string    `json:"symbol"`
		At     time.Time `json:"at"`
		Nodes  []*Node   `json:"nodes"`
		Edges  []*Edge   `json:"edges"`
	}{
		Symbol: evidenceGraph.Symbol,
		At:     evidenceGraph.At,
		Nodes:  nodes,
		Edges:  evidenceGraph.edges,
	})
}

/*
UnmarshalJSON rebuilds a live graph (lookup maps and Evidence composer) from a
persisted topology snapshot.
*/
func (evidenceGraph *Graph) UnmarshalJSON(payload []byte) error {
	var frame struct {
		Symbol string    `json:"symbol"`
		At     time.Time `json:"at"`
		Nodes  []*Node   `json:"nodes"`
		Edges  []*Edge   `json:"edges"`
	}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return err
	}

	*evidenceGraph = Graph{
		Symbol: frame.Symbol,
		At:     frame.At,
		nodes:  make(map[string]*Node),
		edges:  make([]*Edge, 0, len(frame.Edges)),
		seen:   make(map[string]struct{}),
	}
	evidenceGraph.Evidence = newEvidenceComposer(evidenceGraph)

	for _, node := range frame.Nodes {
		if err := evidenceGraph.Evidence.RestoreNode(
			node.Key,
			node.Kind,
			node.Category,
			node.Measurement,
		); err != nil {
			return err
		}
	}

	for _, edge := range frame.Edges {
		evidenceGraph.Relate(
			edge.From,
			edge.To,
			edge.Type,
			edge.At,
			edge.ObservedFrom,
		)
	}

	return nil
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

/*
CategoryKey is the stable evidence reference for a category hypothesis node.
*/
func CategoryKey(category CategoryType) string {
	return string(SourceCategory) + "/" + string(category)
}

/*
ConceptKey is the stable evidence reference for a named causal concept node
(a treatment or outcome variable from a causal hypothesis).
*/
func ConceptKey(name string) string {
	return string(SourceCausal) + "/" + name
}
