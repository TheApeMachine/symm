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
(Source category, Metric = category type) so the wire and UI treat it uniformly.
*/
type Node struct {
	Key         string
	Kind        NodeKind
	Category    CategoryType
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

	evidenceGraph.nodes[key] = &Node{
		Key: key, Kind: NodeMeasurement, Measurement: retained,
	}

	if evidenceGraph.At.IsZero() || measurement.At.After(evidenceGraph.At) {
		evidenceGraph.At = measurement.At
	}

	return nil
}

/*
RestoreNode reinstates one node under its original wire key when a graph is
rebuilt from a snapshot frame. Unlike AddNode it does not recompute the key or
validate a reading, so category and concept nodes round-trip faithfully and the
edge references from the frame still resolve.
*/
func (evidenceGraph *Graph) RestoreNode(
	key string,
	kind NodeKind,
	category CategoryType,
	measurement Measurement,
) {
	if key == "" {
		return
	}

	if _, exists := evidenceGraph.nodes[key]; exists {
		return
	}

	if kind == "" {
		kind = NodeMeasurement
	}

	evidenceGraph.nodes[key] = &Node{
		Key: key, Kind: kind, Category: category, Measurement: measurement,
	}

	if evidenceGraph.At.IsZero() || measurement.At.After(evidenceGraph.At) {
		evidenceGraph.At = measurement.At
	}
}

/*
StagePeerNode inserts a referenced measurement owned by another symbol's graph
so a cross-symbol relationship (lead-lag direction) can be drawn to it with a
real key. The peer keeps its own Symbol; it is evidence borrowed into this
graph, not a reading of this symbol. Returns the stable key to relate against.
*/
func (evidenceGraph *Graph) StagePeerNode(measurement Measurement) string {
	key := MeasurementKey(&measurement)

	if _, exists := evidenceGraph.nodes[key]; exists {
		return key
	}

	evidenceGraph.nodes[key] = &Node{
		Key: key, Kind: NodeMeasurement, Measurement: measurement,
	}

	return key
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
CategoryEvidence walks the composed edges for one category hypothesis and
returns the measurement keys supporting it, opposing it, and the metrics whose
affinity names this category but which produced no active reading this tick
(Missing). It is the per-category evidence surface the terminal renders.
*/
func (evidenceGraph *Graph) CategoryEvidence(category CategoryType) (
	supporting []string, opposing []string, missing []string,
) {
	categoryKey := CategoryKey(category)
	present := make(map[string]struct{})

	for _, edge := range evidenceGraph.edges {
		if edge.To != categoryKey {
			continue
		}

		switch edge.Type {
		case Supports:
			supporting = append(supporting, edge.From)
			present[edge.From] = struct{}{}
		case Contradicts:
			opposing = append(opposing, edge.From)
			present[edge.From] = struct{}{}
		}
	}

	for _, node := range evidenceGraph.nodes {
		if node.Kind != NodeMeasurement {
			continue
		}

		affinity, ok := AffinityFor(node.Measurement.Metric)

		if !ok {
			continue
		}

		if _, seen := present[node.Key]; seen {
			continue
		}

		if categoryListed(affinity, category) {
			missing = append(missing, node.Key)
		}
	}

	return supporting, opposing, missing
}

/*
categoryListed reports whether an affinity names the category on either side.
*/
func categoryListed(affinity MetricAffinity, category CategoryType) bool {
	for _, candidate := range affinity.Supports {
		if candidate == category {
			return true
		}
	}

	for _, candidate := range affinity.Opposes {
		if candidate == category {
			return true
		}
	}

	return false
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
ensureCategory stages a synthetic category node once per composition so
measurement nodes can attach Supports and Contradicts edges to it. The category
node carries a descriptive Measurement shell rather than a reading of its own.
*/
func (evidenceGraph *Graph) ensureCategory(category CategoryType, at time.Time) string {
	key := CategoryKey(category)

	if _, exists := evidenceGraph.nodes[key]; exists {
		return key
	}

	evidenceGraph.nodes[key] = &Node{
		Key:      key,
		Kind:     NodeCategory,
		Category: category,
		Measurement: Measurement{
			Source: SourceCategory,
			Metric: MetricType(category),
			Symbol: evidenceGraph.Symbol,
			At:     at,
		},
	}

	return key
}

/*
ConceptKey is the stable evidence reference for a named causal concept node
(a treatment or outcome variable from a causal hypothesis).
*/
func ConceptKey(name string) string {
	return string(SourceCausal) + "/" + name
}

/*
ensureConcept stages a synthetic concept node once so a causal Conditions edge
can reference a named treatment or outcome variable.
*/
func (evidenceGraph *Graph) ensureConcept(name string, at time.Time) string {
	key := ConceptKey(name)

	if _, exists := evidenceGraph.nodes[key]; exists {
		return key
	}

	evidenceGraph.nodes[key] = &Node{
		Key:  key,
		Kind: NodeConcept,
		Measurement: Measurement{
			Source: SourceCausal,
			Metric: MetricType(name),
			Symbol: evidenceGraph.Symbol,
			At:     at,
		},
	}

	return key
}

/*
RelateConditions records a directed causal claim that a treatment variable
conditions an outcome variable, staging both concept nodes. It is the graph
projection of a ready causal hypothesis.
*/
func (evidenceGraph *Graph) RelateConditions(
	treatment string,
	outcome string,
	at time.Time,
	observedFrom time.Time,
) bool {
	if treatment == "" || outcome == "" {
		return false
	}

	treatmentKey := evidenceGraph.ensureConcept(treatment, at)
	outcomeKey := evidenceGraph.ensureConcept(outcome, at)

	return evidenceGraph.Relate(
		treatmentKey, outcomeKey, Conditions, at, observedFrom,
	)
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
