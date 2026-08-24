package types

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/algorithm"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/types"
	graphtypes "github.com/theapemachine/symm/types/graph"
)

/*
Graph is the relational knowledge graph accumulated during one Thesis lifecycle.
*/
type Graph struct {
	mu              sync.RWMutex
	Symbol          string              `json:"symbol,omitempty"`
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
CanonicalizeDecisionEdges orders proposition edges by stable evidence identity
while preserving the construction order of every non-decision edge.
*/
func (graph *Graph) CanonicalizeDecisionEdges() {
	if graph == nil {
		return
	}

	graph.mu.Lock()
	defer graph.mu.Unlock()

	target := graph.DecisionTarget
	slices.SortStableFunc(graph.Edges, func(left, right *Edge) int {
		leftDecision := left.To == target
		rightDecision := right.To == target

		if !leftDecision && !rightDecision {
			return 0
		}

		if !leftDecision {
			return -1
		}

		if !rightDecision {
			return 1
		}

		if order := cmp.Compare(left.From, right.From); order != 0 {
			return order
		}

		return cmp.Compare(left.Relation, right.Relation)
	})
}

/*
Prune removes expired transient measurement nodes and stale edges while
preserving active structural anchors (categories, fields, SCM, hypotheses).
*/
func (graph *Graph) Prune(now time.Time) {
	if graph == nil || now.IsZero() {
		return
	}

	graph.mu.Lock()
	defer graph.mu.Unlock()

	const maxMeasurementRetention = 5 * time.Minute
	prunedNodeIDs := make(map[string]bool)

	for nodeID, node := range graph.Nodes {
		if node == nil || node.Kind != KindMeasurement || node.At.IsZero() {
			continue
		}

		horizon := node.Horizon

		if horizon <= 0 {
			horizon = 30 * time.Second
		}

		retention := 5 * horizon

		if retention > maxMeasurementRetention {
			retention = maxMeasurementRetention
		}

		if now.Sub(node.At) > retention {
			delete(graph.Nodes, nodeID)
			prunedNodeIDs[nodeID] = true
		}
	}

	if len(prunedNodeIDs) == 0 {
		return
	}

	activeEdges := make([]*Edge, 0, len(graph.Edges))

	for _, edge := range graph.Edges {
		if prunedNodeIDs[edge.From] || prunedNodeIDs[edge.To] {
			continue
		}

		activeEdges = append(activeEdges, edge)
	}

	graph.Edges = activeEdges
	graph.Adjacency = make(map[string][]string, len(graph.Nodes))

	for _, edge := range graph.Edges {
		if !slices.Contains(graph.Adjacency[edge.From], edge.To) {
			graph.Adjacency[edge.From] = append(graph.Adjacency[edge.From], edge.To)
		}
	}
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
SearchableEnough reports whether a structurally ready graph clears a caller's
relation-confidence floor. Relation confidence is not the directional forecast
probability used for trade admission.
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
func (graph *Graph) OpportunitySummary() graphtypes.OpportunitySummary {
	summary := graphtypes.OpportunitySummary{}

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
	var state types.Frame

	for _, edge := range graph.Edges {
		if edge.To != graph.DecisionTarget || edge.Weight <= 0 || edge.Confidence <= 0 {
			continue
		}

		input := types.Frame{}
		input.Put(algorithm.SymbolEdgeWeight, edge.Weight)
		input.Put(algorithm.SymbolEdgeConfidence, edge.Confidence)
		input.Put(algorithm.SymbolEdgeRelation, relationSign(edge.Relation))

		state, _, _ = algorithm.OpportunityReducer(state, input)
	}

	state, _, _ = algorithm.OpportunityScorer(state, types.Frame{})

	summary.Support, _ = state.Get(algorithm.SymbolOpportunitySupport)
	summary.Contradiction, _ = state.Get(algorithm.SymbolOpportunityContradiction)
	summary.Conditions, _ = state.Get(algorithm.SymbolOpportunityConditions)
	summary.Balance, _ = state.Get(algorithm.SymbolOpportunityBalance)
	summary.Confidence, _ = state.Get(algorithm.SymbolOpportunityConfidence)
	summary.Score, _ = state.Get(algorithm.SymbolOpportunityScore)
	summary.Direction, _ = state.Get(algorithm.SymbolOpportunityDirection)

	ready, _ := state.Get(algorithm.SymbolOpportunityReady)
	summary.Ready = ready > 0

	return summary
}
