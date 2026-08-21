package types

import (
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/learning"
)

/*
RelationType describes directional edge relationships between market nodes.
*/
type RelationType string

const (
	RelationSupports         RelationType = "supports"
	RelationContradicts      RelationType = "contradicts"
	RelationConditions       RelationType = "conditions"
	RelationLeads            RelationType = "leads"
	RelationLags             RelationType = "lags"
	RelationRedundantWith    RelationType = "redundant_with"
	RelationIndependentOf    RelationType = "independent_of"
	RelationStaleRelativeTo  RelationType = "stale_relative_to"
	RelationIncomparableWith RelationType = "incomparable_with"
)

/*
Kind describes the type of node in the knowledge graph.
*/
type Kind string

const (
	KindMeasurement Kind = "measurement"
	KindCategory    Kind = "category"
	KindManifold    Kind = "manifold"
	KindResonance   Kind = "resonance"
	KindCausal      Kind = "causal"
	KindCognition   Kind = "cognition"
	KindPrediction  Kind = "prediction"
	KindHypothesis  Kind = "hypothesis"
)

/*
Node represents a discrete market entity, metric, category, or latent state.
*/
type Node struct {
	ID            string          `json:"id"`
	Symbol        string          `json:"symbol,omitempty"`
	Peer          string          `json:"peer,omitempty"`
	Source        string          `json:"source,omitempty"`
	MeasurementID string          `json:"measurementId,omitempty"`
	Metric        MetricType      `json:"metric,omitempty"`
	Side          MeasurementSide `json:"side,omitempty"`
	Kind          Kind            `json:"kind"`
	Value         float64         `json:"value"`
	Normalized    *float64        `json:"normalized,omitempty"`
	Quality       *float64        `json:"quality,omitempty"`
	Strength      float64         `json:"strength,omitempty"`
	Confidence    float64         `json:"confidence"`
	Maturity      float64         `json:"maturity,omitempty"`
	Unit          MeasurementUnit `json:"unit,omitempty"`
	ObservedFrom  time.Time       `json:"observedFrom,omitempty"`
	Horizon       time.Duration   `json:"horizon,omitempty"`
	At            time.Time       `json:"at"`
	Metadata      map[string]any  `json:"metadata,omitempty"`
}

/*
Edge represents a directed, weighted relationship from Node A to Node B.
*/
type Edge struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	Relation     RelationType  `json:"relation"`
	Weight       float64       `json:"weight"`
	Confidence   float64       `json:"confidence"`
	Quality      *float64      `json:"quality,omitempty"`
	Evidence     []string      `json:"evidence,omitempty"`
	ObservedFrom time.Time     `json:"observedFrom,omitempty"`
	Horizon      time.Duration `json:"horizon,omitempty"`
	At           time.Time     `json:"at"`
	Reason       string        `json:"reason,omitempty"`
}

/*
Graph is the relational knowledge graph accumulated during one Thesis lifecycle.
*/
type Graph struct {
	mu              sync.RWMutex
	At              time.Time           `json:"at"`
	Forecast        *learning.RLSOutput `json:"-"`
	ForecastHorizon int                 `json:"forecastHorizon"`
	ForwardCurve    []float64           `json:"-"`
	TaskSkill       float64             `json:"taskSkill"`
	TaskSkillReady  bool                `json:"taskSkillReady"`
	DecisionTarget  string              `json:"decisionTarget,omitempty"`
	Nodes           map[string]*Node    `json:"nodes"`
	Edges           []*Edge             `json:"edges"`
	Adjacency       map[string][]string `json:"adjacency"` // Fast lookup: NodeID -> []TargetNodeIDs
}

/*
Clone returns an isolated point-in-time snapshot of the graph.
*/
func (graph *Graph) Clone() *Graph {
	if graph == nil {
		return nil
	}

	graph.mu.RLock()
	defer graph.mu.RUnlock()

	nodes := make(map[string]*Node, len(graph.Nodes))

	for id, node := range graph.Nodes {
		nodes[id] = node
	}

	edges := make([]*Edge, len(graph.Edges))
	copy(edges, graph.Edges)

	adjacency := make(map[string][]string, len(graph.Adjacency))

	for id, targets := range graph.Adjacency {
		adjacency[id] = slices.Clone(targets)
	}

	return &Graph{
		At:              graph.At,
		Forecast:        graph.Forecast,
		ForecastHorizon: graph.ForecastHorizon,
		ForwardCurve:    slices.Clone(graph.ForwardCurve),
		TaskSkill:       graph.TaskSkill,
		TaskSkillReady:  graph.TaskSkillReady,
		DecisionTarget:  graph.DecisionTarget,
		Nodes:           nodes,
		Edges:           edges,
		Adjacency:       adjacency,
	}
}

/*
CheckpointState includes the forecast omitted from the dashboard graph wire.
*/
func (graph *Graph) CheckpointState() any {
	if graph == nil {
		return nil
	}

	graph.mu.RLock()
	defer graph.mu.RUnlock()

	return struct {
		At              time.Time           `json:"at"`
		Forecast        *learning.RLSOutput `json:"forecast,omitempty"`
		ForecastHorizon int                 `json:"forecastHorizon"`
		ForwardCurve    []float64           `json:"forwardCurve"`
		TaskSkill       float64             `json:"taskSkill"`
		TaskSkillReady  bool                `json:"taskSkillReady"`
		DecisionTarget  string              `json:"decisionTarget,omitempty"`
		Nodes           map[string]*Node    `json:"nodes"`
		Edges           []*Edge             `json:"edges"`
		Adjacency       map[string][]string `json:"adjacency"`
	}{
		At:              graph.At,
		Forecast:        graph.Forecast,
		ForecastHorizon: graph.ForecastHorizon,
		ForwardCurve:    slices.Clone(graph.ForwardCurve),
		TaskSkill:       graph.TaskSkill,
		TaskSkillReady:  graph.TaskSkillReady,
		DecisionTarget:  graph.DecisionTarget,
		Nodes:           graph.Nodes,
		Edges:           graph.Edges,
		Adjacency:       graph.Adjacency,
	}
}

/*
SetResonanceOutput publishes the calibrated return-head foreground and its
supporting horizon under the graph's write lock.
*/
func (graph *Graph) SetResonanceOutput(
	forecast *learning.RLSOutput,
	horizon int,
) {
	if graph == nil {
		return
	}

	graph.mu.Lock()
	graph.Forecast = forecast
	graph.ForecastHorizon = horizon
	graph.ForwardCurve = nil
	graph.mu.Unlock()
}

/*
SetTaskSkill publishes the predictive coders' prequential skill readout under
the graph's write lock.
*/
func (graph *Graph) SetTaskSkill(skill float64, ready bool) {
	if graph == nil {
		return
	}

	graph.mu.Lock()
	graph.TaskSkill = skill
	graph.TaskSkillReady = ready
	graph.mu.Unlock()
}

/*
SetDecisionTarget names the archetype this graph currently reasons toward, under
the graph's write lock.
*/
func (graph *Graph) SetDecisionTarget(target string) {
	if graph == nil {
		return
	}

	graph.mu.Lock()
	graph.DecisionTarget = target
	graph.mu.Unlock()
}

/*
NewGraph creates an empty graph initialized with node and adjacency maps.
*/
func NewGraph(at time.Time) *Graph {
	return &Graph{
		At:        at,
		Nodes:     make(map[string]*Node),
		Edges:     make([]*Edge, 0),
		Adjacency: make(map[string][]string),
	}
}

/*
AddNode registers the latest value for a stable node identity.
*/
func (graph *Graph) AddNode(node *Node) {
	if node == nil || node.ID == "" {
		panic("graph: node and node ID required")
	}

	graph.mu.Lock()
	defer graph.mu.Unlock()

	graph.Nodes[node.ID] = node
}

/*
AddEdge connects two nodes with a directional, weighted relationship.
*/
func (graph *Graph) AddEdge(edge *Edge) {
	if edge == nil || edge.From == "" || edge.To == "" {
		panic("graph: edge and endpoint IDs required")
	}

	graph.mu.Lock()
	defer graph.mu.Unlock()

	if _, found := graph.Nodes[edge.From]; !found {
		panic("graph: source node not registered: " + edge.From)
	}

	if _, found := graph.Nodes[edge.To]; !found {
		panic("graph: target node not registered: " + edge.To)
	}

	for index, current := range graph.Edges {
		sameDecisionClaim :=
			(current.Relation == RelationSupports ||
				current.Relation == RelationContradicts) &&
				(edge.Relation == RelationSupports ||
					edge.Relation == RelationContradicts)

		if current.From == edge.From && current.To == edge.To &&
			(current.Relation == edge.Relation || sameDecisionClaim) {
			graph.Edges[index] = edge
			return
		}
	}

	graph.Edges = append(graph.Edges, edge)

	if !slices.Contains(graph.Adjacency[edge.From], edge.To) {
		graph.Adjacency[edge.From] = append(graph.Adjacency[edge.From], edge.To)
	}
}

/*
OpportunitySummary is the dimensionless evidence balance for the graph's
explicit decision proposition. Conditions are reported separately and never
smuggled into directional support.
*/
type OpportunitySummary struct {
	Hypothesis    string
	Support       float64
	Contradiction float64
	Conditions    float64
	Balance       float64
	Confidence    float64
	Score         float64
	Direction     float64
	Ready         bool
}

/*
Roots returns only evidence roots that can reach the configured decision
proposition. The graph remains fully visible on the wire, but MCTS no longer
spends simulations on disconnected explanatory islands.
*/
func (graph *Graph) Roots() []string {
	if graph == nil {
		return nil
	}

	graph.mu.RLock()
	defer graph.mu.RUnlock()

	relevant := graph.relevantNodes()
	incoming := make(map[string]bool)

	for _, edge := range graph.Edges {
		if !relevant[edge.From] || !relevant[edge.To] {
			continue
		}

		incoming[edge.To] = true
	}

	roots := make([]string, 0)

	for nodeID := range relevant {
		if nodeID == graph.DecisionTarget {
			continue
		}

		node := graph.Nodes[nodeID]

		if node == nil {
			continue
		}

		if node.Kind == KindCognition {
			held, _ := node.Metadata["held"].(bool)

			if held {
				continue
			}
		}

		if !incoming[nodeID] {
			roots = append(roots, nodeID)
		}
	}

	/*
		A relevant cycle can have no conventional root. In that case the direct
		predecessors of the decision proposition are honest entry points: every
		one is an evidence statement the search can evaluate immediately.
	*/
	if len(roots) == 0 && graph.DecisionTarget != "" {
		for _, edge := range graph.Edges {
			if edge.To == graph.DecisionTarget && relevant[edge.From] {
				roots = append(roots, edge.From)
			}
		}
	}

	slices.Sort(roots)
	roots = slices.Compact(roots)
	return roots
}

func (graph *Graph) relevantNodes() map[string]bool {
	relevant := make(map[string]bool)

	if graph == nil {
		return relevant
	}

	if graph.DecisionTarget == "" || graph.Nodes[graph.DecisionTarget] == nil {
		for nodeID := range graph.Nodes {
			relevant[nodeID] = true
		}

		return relevant
	}

	reverse := make(map[string][]string)

	for _, edge := range graph.Edges {
		reverse[edge.To] = append(reverse[edge.To], edge.From)
	}

	queue := []string{graph.DecisionTarget}

	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]

		if relevant[nodeID] {
			continue
		}

		relevant[nodeID] = true
		queue = append(queue, reverse[nodeID]...)
	}

	return relevant
}

/*
ReadyForSearch reports whether the lifecycle has an explicit proposition,
directional evidence for or against it, and at least one explanatory root that
can reach it. Predictive coding may contribute, but it is not a prerequisite.
*/
func (graph *Graph) ReadyForSearch() bool {
	if graph == nil {
		return false
	}

	// OpportunitySummary and Roots each take their own read lock, so no field
	// is read here outside a lock. Reading graph.Nodes/DecisionTarget directly
	// would race the analyzer's Update and fatally corrupt the map.
	summary := graph.OpportunitySummary()
	return summary.Ready && len(graph.Roots()) > 0
}

/*
SearchableEnough is the defer gate the planner runs before spending search
effort. A graph is searchable only once its decision proposition carries
directional evidence whose confidence mass clears minimumConfidence. Anything
below that is a sparse proposition — the honest move is to defer and let the
thesis accumulate more observations, not to search an opportunity that barely
exists yet.
*/
func (graph *Graph) SearchableEnough(minimumConfidence float64) bool {
	if graph == nil {
		return false
	}

	summary := graph.OpportunitySummary()

	if !summary.Ready {
		return false
	}

	return summary.Confidence >= minimumConfidence
}

func (graph *Graph) Targets(nodeID string) []string {
	return slices.Clone(graph.Adjacency[nodeID])
}

func (graph *Graph) NodeValue(nodeID string) (float64, float64) {
	node := graph.Nodes[nodeID]

	if node == nil {
		return 0, 0
	}

	return node.Value, node.Confidence
}

/*
EdgeValue states graph relations in the reward domain used by MCTS. Supports
and contradictions are signed evidence. Conditions, temporal links,
redundancy, and independence remain traversable context with zero directional
reward. Stale or incomparable claims count against a decision because they
cannot justify risking current capital.
*/
func (graph *Graph) EdgeValue(from, to string) (float64, float64) {
	evidenceMass := 0.0
	confidenceMass := 0.0
	relationCount := 0

	for _, edge := range graph.Edges {
		if edge.From != from || edge.To != to {
			continue
		}

		sign := relationSign(edge.Relation)
		evidenceMass += sign * edge.Weight * edge.Confidence
		confidenceMass += edge.Confidence
		relationCount++
	}

	if relationCount > 0 && confidenceMass > 0 {
		return evidenceMass / confidenceMass,
			confidenceMass / float64(relationCount)
	}

	panic("graph: edge not found from " + from + " to " + to)
}

func relationSign(relation RelationType) float64 {
	switch relation {
	case RelationSupports:
		return 1
	case RelationContradicts, RelationStaleRelativeTo, RelationIncomparableWith:
		return -1
	default:
		return 0
	}
}

/*
OpportunitySummary reduces only edges that directly address the proposition.
Intermediate graph structure remains available to MCTS but is not counted a
second time in the thesis balance.
*/
func (graph *Graph) OpportunitySummary() OpportunitySummary {
	summary := OpportunitySummary{}

	if graph == nil {
		return summary
	}

	graph.mu.RLock()
	defer graph.mu.RUnlock()

	if graph.DecisionTarget == "" ||
		graph.Nodes == nil ||
		graph.Nodes[graph.DecisionTarget] == nil {
		return summary
	}

	summary.Hypothesis = graph.DecisionTarget
	confidenceMass := 0.0
	confidenceWeight := 0.0

	for _, edge := range graph.Edges {
		if edge.To != graph.DecisionTarget || edge.Weight <= 0 || edge.Confidence <= 0 {
			continue
		}

		mass := edge.Weight * edge.Confidence

		switch relationSign(edge.Relation) {
		case 1:
			summary.Support += mass
			confidenceMass += edge.Weight * edge.Confidence
			confidenceWeight += edge.Weight
		case -1:
			summary.Contradiction += mass
			confidenceMass += edge.Weight * edge.Confidence
			confidenceWeight += edge.Weight
		default:
			summary.Conditions += mass
		}
	}

	directional := summary.Support + summary.Contradiction

	if !(directional > 0) || !(confidenceWeight > 0) {
		return summary
	}

	summary.Balance = (summary.Support - summary.Contradiction) / directional
	summary.Confidence = confidenceMass / confidenceWeight
	summary.Score = summary.Balance * summary.Confidence
	summary.Ready = true

	if summary.Score > 0 {
		summary.Direction = 1
	} else if summary.Score < 0 {
		summary.Direction = -1
	}

	return summary
}
