package graph

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
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
	KindCategory  Kind = "category"
	KindResonance Kind = "resonance"
	KindCausal    Kind = "causal"
	KindCognition Kind = "cognition"
)

/*
Node represents a discrete market entity, metric, category, or latent state.
*/
type Node struct {
	ID         string         `json:"id"`
	Symbol     string         `json:"symbol,omitempty"`
	Source     string         `json:"source,omitempty"`
	Kind       Kind           `json:"kind"`
	Value      float64        `json:"value"`
	Confidence float64        `json:"confidence"`
	At         time.Time      `json:"at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

/*
Edge represents a directed, weighted relationship from Node A to Node B.
*/
type Edge struct {
	From       string       `json:"from"`
	To         string       `json:"to"`
	Relation   RelationType `json:"relation"`
	Weight     float64      `json:"weight"`
	Confidence float64      `json:"confidence"`
	At         time.Time    `json:"at"`
	Reason     string       `json:"reason,omitempty"`
}

/*
Graph is the full relational knowledge graph constructed for a Thesis cut.
*/
type Graph struct {
	At        time.Time           `json:"at"`
	Nodes     map[string]*Node    `json:"nodes"`
	Edges     []*Edge             `json:"edges"`
	Adjacency map[string][]string `json:"adjacency"` // Fast lookup: NodeID -> []TargetNodeIDs
}

var publishJSON = sonic.Config{
	EncodeNullForInfOrNan: true,
}.Froze()

type publishedNode struct {
	Source     string         `json:"source"`
	Symbol     string         `json:"symbol"`
	At         string         `json:"at"`
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Value      float64        `json:"value"`
	Confidence float64        `json:"confidence"`
	NodeSource string         `json:"nodeSource,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type publishedEdge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Relation   string  `json:"relation"`
	Weight     float64 `json:"weight"`
	Confidence float64 `json:"confidence"`
	At         string  `json:"at"`
	Reason     string  `json:"reason"`
}

type publishedGraph struct {
	At    string                   `json:"at"`
	Nodes map[string]publishedNode `json:"nodes"`
	Edges []publishedEdge          `json:"edges"`
}

type publishedGraphFrame struct {
	Graph publishedGraph `json:"graph"`
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
AddNode registers a node in the graph if it doesn't already exist.
*/
func (graph *Graph) AddNode(node *Node) {
	if node == nil || node.ID == "" {
		return
	}
	graph.Nodes[node.ID] = node
}

/*
AddEdge connects two nodes with a directional, weighted relationship.
*/
func (graph *Graph) AddEdge(edge *Edge) {
	if edge == nil || edge.From == "" || edge.To == "" {
		return
	}
	graph.Edges = append(graph.Edges, edge)
	graph.Adjacency[edge.From] = append(graph.Adjacency[edge.From], edge.To)
}

/*
Option configures the Graph solver.
*/
type Option func(*Solver)

/*
WithStaleThreshold sets the time threshold after which a node is considered stale relative to another.
*/
func WithStaleThreshold(threshold time.Duration) Option {
	return func(s *Solver) {
		s.staleThreshold = threshold
	}
}

/*
Solver compiles all upstream evidence (Measurements, Manifold, Resonance, Causal, Cognition)
into a Directed Knowledge Graph for the Strategy package.
*/
type Solver struct {
	recorder       *audit.Recorder
	staleThreshold time.Duration
	mu             sync.RWMutex
	ui             chan []byte
}

/*
NewSolver creates a graph solver wired to audit recordingraph.
Default stale threshold: 5 seconds.
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder, opts ...Option) *Solver {
	solver := &Solver{
		recorder:       recorder,
		staleThreshold: 5 * time.Second,
		ui:             ui,
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update extracts nodes and infers directional edges from Thesis, publishing the compiled
Graph into thesis.Graphs.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	graph := NewGraph(thesis.At)

	// 1. Extract and register all non-measurement nodes from Thesis
	solver.extractCategoryNodes(thesis, graph)
	solver.extractResonanceNodes(thesis, graph)
	solver.extractCausalNodes(thesis, graph)
	solver.extractCognitionNodes(thesis, graph)

	// 2. Infer directional relationships between nodes
	solver.inferStructuralEdges(thesis, graph)

	// 3. Store compiled Graph into thesis.Graphs
	thesis.Graphs.Store("market_graph", graph)
	thesis.Readiness.Graph = true

	// 4. Record audit snapshot
	if solver.recorder != nil {
		err := audit.Record(solver.recorder, "predictive", map[string]any{
			"stage":     "graph",
			"nodeCount": len(graph.Nodes),
			"edgeCount": len(graph.Edges),
			"at":        graph.At,
		})

		if err != nil {
			return err
		}
	}

	solver.publish(thesis, graph)

	return nil
}

func (solver *Solver) publish(thesis *types.Thesis, graph *Graph) {
	if solver == nil || solver.ui == nil || thesis == nil || graph == nil {
		return
	}

	if cap(solver.ui) > 0 && len(solver.ui) == cap(solver.ui) {
		return
	}

	at := thesis.At.Format(time.RFC3339)
	nodes := make(map[string]publishedNode, len(graph.Nodes))

	for _, node := range graph.Nodes {
		if node == nil {
			continue
		}

		nodes[node.ID] = publishedNode{
			Source:     "graph",
			Symbol:     node.Symbol,
			At:         at,
			ID:         node.ID,
			Kind:       string(node.Kind),
			Value:      node.Value,
			Confidence: node.Confidence,
			NodeSource: node.Source,
			Metadata:   node.Metadata,
		}
	}

	if len(nodes) == 0 {
		return
	}

	/*
		Edges travel with the nodes they connect. A graph is the relationships
		it encodes, so publishing the nodes alone would leave the display with
		a list of readings and no way to show how any of them relate.
	*/
	edges := make([]publishedEdge, 0, len(graph.Edges))

	for _, edge := range graph.Edges {
		if edge == nil {
			continue
		}

		edges = append(edges, publishedEdge{
			From:       edge.From,
			To:         edge.To,
			Relation:   string(edge.Relation),
			Weight:     edge.Weight,
			Confidence: edge.Confidence,
			At:         at,
			Reason:     edge.Reason,
		})
	}

	payload, err := publishJSON.Marshal(publishedGraphFrame{
		Graph: publishedGraph{
			At:    at,
			Nodes: nodes,
			Edges: edges,
		},
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"graph: failed to marshal publication",
			err,
		))

		return
	}

	select {
	case solver.ui <- payload:
	default:
	}
}

/*
extractCategoryNodes registers active categories as nodes.
*/
func (solver *Solver) extractCategoryNodes(thesis *types.Thesis, graph *Graph) {
	for symbol, categories := range thesis.Categories {
		for _, cat := range categories {
			nodeID := fmt.Sprintf("cat:%s:%s", symbol, string(cat.Type))

			graph.AddNode(&Node{
				ID:         nodeID,
				Symbol:     symbol,
				Source:     "category",
				Kind:       "category",
				Value:      cat.Strength,
				Confidence: cat.Confidence,
				At:         thesis.At,
				Metadata: map[string]any{
					"type":       string(cat.Type),
					"surprisal":  cat.Surprisal,
					"maturity":   cat.Maturity,
					"supporting": cat.Supporting,
					"opposing":   cat.Opposing,
				},
			})
		}
	}
}

/*
extractResonanceNodes registers predictive coding outcomes (surprise, expected return forecast).
*/
func (solver *Solver) extractResonanceNodes(thesis *types.Thesis, graph *Graph) {
	if thesis == nil || thesis.Resonance == nil {
		return
	}

	thesis.Resonance.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		row, rowOK := value.(map[string]any)

		if !symbolOK || !rowOK || symbol == "" {
			return true
		}

		surprise, _ := row["surprise"].(float64)
		predValue := 0.0

		if curve, ok := row["forwardCurve"].([]float64); ok && len(curve) > 0 {
			predValue = curve[0]
		}

		graph.AddNode(&Node{
			ID:         fmt.Sprintf("res:%s:surprise", symbol),
			Symbol:     symbol,
			Source:     "resonance",
			Kind:       "resonance",
			Value:      surprise,
			Confidence: 1.0,
			At:         thesis.At,
		})

		graph.AddNode(&Node{
			ID:         fmt.Sprintf("res:%s:forecast", symbol),
			Symbol:     symbol,
			Source:     "resonance",
			Kind:       "resonance",
			Value:      predValue,
			Confidence: 1.0,
			At:         thesis.At,
		})

		return true
	})
}

/*
extractCausalNodes registers Pearl do-calculus and counterfactual uplift outputs.
*/
func (solver *Solver) extractCausalNodes(thesis *types.Thesis, graph *Graph) {
	thesis.Causal.Range(func(key, value any) bool {
		symbol, _ := key.(string)
		causalMap, ok := value.(map[string]any)
		if !ok {
			return true
		}

		confidence := graphConfidence(causalMap["confidence"])

		for _, field := range []string{"doExpectation", "uplift", "association", "intervention"} {
			if val, ok := causalMap[field].(float64); ok {
				nodeID := fmt.Sprintf("causal:%s:%s", symbol, field)
				graph.AddNode(&Node{
					ID:         nodeID,
					Symbol:     symbol,
					Source:     "causal",
					Kind:       "causal",
					Value:      val,
					Confidence: confidence,
					At:         thesis.At,
				})
			}
		}

		return true
	})
}

func graphConfidence(value any) float64 {
	confidence, ok := value.(float64)

	if !ok || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return 0
	}

	if confidence < 0 {
		return 0
	}

	if confidence > 1 {
		return 1
	}

	return confidence
}

/*
extractCognitionNodes registers active category sequences and lookahead predictions.
*/
func (solver *Solver) extractCognitionNodes(thesis *types.Thesis, graph *Graph) {
	thesis.Cognition.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		cognition, cognitionOK := value.(types.Cognition)

		if !symbolOK || !cognitionOK || cognition.Winner == "" {
			return true
		}

		nodeID := fmt.Sprintf("cog:%s:winner_regime", symbol)
		graph.AddNode(&Node{
			ID:         nodeID,
			Symbol:     symbol,
			Source:     cognition.Source,
			Kind:       KindCognition,
			Value:      cognition.Confidence,
			Confidence: cognition.Confidence,
			At:         cognition.At,
			Metadata: map[string]any{
				"regime": cognition.Winner,
			},
		})

		return true
	})
}

/*
agreementWeight scores how strongly two readings agree in direction, on a
scale of zero to one, without regard to the units either of them uses.

Nodes come from heads that measure different things: a resonance forecast is a
fractional return, while a causal uplift is an unbounded score. Comparing their
magnitudes directly would make the relation a statement about scale rather than
about agreement, so each side is squashed to a bounded strength first and the
weaker of the two decides how much the pair can claim.
*/
func agreementWeight(left, right float64) float64 {
	if math.IsNaN(left) || math.IsNaN(right) {
		return 0
	}

	return math.Min(
		math.Tanh(math.Abs(left)),
		math.Tanh(math.Abs(right)),
	)
}

/*
inferStructuralEdges indexes nodes by the domains that can produce an
evidence-bearing relationship. Zero-confidence pair relations are not
materialized because their decision weight is necessarily zero.
*/
func (solver *Solver) inferStructuralEdges(thesis *types.Thesis, graph *Graph) {
	nodes := graph.Nodes
	resonanceBySymbol := make(map[string][]*Node)
	causalBySymbol := make(map[string][]*Node)
	interventions := make([]*Node, 0)
	expectations := make([]*Node, 0)

	for _, node := range nodes {
		switch node.Kind {
		case KindResonance:
			resonanceBySymbol[node.Symbol] = append(resonanceBySymbol[node.Symbol], node)
		case KindCausal:
			causalBySymbol[node.Symbol] = append(causalBySymbol[node.Symbol], node)

			if node.ID == "causal:"+node.Symbol+":intervention" {
				interventions = append(interventions, node)
			}

			if node.ID == "causal:"+node.Symbol+":doExpectation" {
				expectations = append(expectations, node)
			}
		}
	}

	for symbol, resonanceNodes := range resonanceBySymbol {
		for _, resonanceNode := range resonanceNodes {
			for _, causalNode := range causalBySymbol[symbol] {
				/*
					This edge reads the two heads for directional agreement
					only. They score on unrelated scales, so the weight is the
					strength of the agreement rather than any difference
					between the values; a raw magnitude here would let the
					head with the larger units decide the relation by itself.
				*/
				agreement := agreementWeight(resonanceNode.Value, causalNode.Value)

				if resonanceNode.Value > 0 && causalNode.Value > 0 {
					graph.AddEdge(&Edge{
						From:       resonanceNode.ID,
						To:         causalNode.ID,
						Relation:   RelationSupports,
						Weight:     agreement,
						Confidence: resonanceNode.Confidence * causalNode.Confidence,
						At:         graph.At,
						Reason:     "predictive forecast and causal uplift agree directionally (+)",
					})

					continue
				}

				if (resonanceNode.Value > 0 && causalNode.Value < 0) ||
					(resonanceNode.Value < 0 && causalNode.Value > 0) {
					graph.AddEdge(&Edge{
						From:       resonanceNode.ID,
						To:         causalNode.ID,
						Relation:   RelationContradicts,
						Weight:     agreement,
						Confidence: resonanceNode.Confidence * causalNode.Confidence,
						At:         graph.At,
						Reason:     "predictive forecast and causal uplift conflict in direction",
					})

					continue
				}

				if math.Abs(resonanceNode.Value) < 0.01 && math.Abs(causalNode.Value) < 0.01 {
					graph.AddEdge(&Edge{
						From:       resonanceNode.ID,
						To:         causalNode.ID,
						Relation:   RelationIndependentOf,
						Weight:     1.0,
						Confidence: 1.0,
						At:         graph.At,
						Reason:     "zero forecast magnitude and zero causal uplift",
					})
				}
			}
		}
	}

	for _, intervention := range interventions {
		for _, expectation := range expectations {
			graph.AddEdge(&Edge{
				From:     intervention.ID,
				To:       expectation.ID,
				Relation: RelationConditions,

				/*
					The interventional level is an unbounded causal score,
					so it states how strongly it conditions rather than by
					how much, keeping this edge comparable to the others.
				*/
				Weight:     math.Tanh(math.Abs(intervention.Value)),
				Confidence: intervention.Confidence,
				At:         graph.At,
				Reason:     "interventional level conditions do-expectation",
			})
		}
	}

	// 2. Evaluate Leads & Lags from Cognition Beam Search
	thesis.Cognition.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		cognition, cognitionOK := value.(types.Cognition)

		if !symbolOK || !cognitionOK {
			return true
		}

		currentNodeID := fmt.Sprintf("cog:%s:winner_regime", symbol)

		for path, probability := range cognition.Predictions {
			if path == "" {
				continue
			}

			targetNodeID := fmt.Sprintf("cat:%s:%s", symbol, path)
			graph.AddEdge(&Edge{
				From:       currentNodeID,
				To:         targetNodeID,
				Relation:   RelationLeads,
				Weight:     probability,
				Confidence: probability,
				At:         graph.At,
				Reason:     "cognition beam search lookahead prediction",
			})
			graph.AddEdge(&Edge{
				From:       targetNodeID,
				To:         currentNodeID,
				Relation:   RelationLags,
				Weight:     probability,
				Confidence: probability,
				At:         graph.At,
				Reason:     "inverse temporal lag of beam search lookahead",
			})
		}

		return true
	})

	// 3. Evaluate Category Supporting and Opposing lists
	for symbol, categories := range thesis.Categories {
		for _, cat := range categories {
			catNodeID := fmt.Sprintf("cat:%s:%s", symbol, string(cat.Type))

			for _, supp := range cat.Supporting {
				targetNodeID := fmt.Sprintf("cat:%s:%s", symbol, supp)

				graph.AddEdge(&Edge{
					From:       catNodeID,
					To:         targetNodeID,
					Relation:   RelationSupports,
					Weight:     cat.Strength,
					Confidence: cat.Confidence,
					At:         graph.At,
					Reason:     "category explicit supporting list",
				})
			}

			for _, opp := range cat.Opposing {
				targetNodeID := fmt.Sprintf("cat:%s:%s", symbol, opp)

				graph.AddEdge(&Edge{
					From:       catNodeID,
					To:         targetNodeID,
					Relation:   RelationOpposingRelation(catNodeID, targetNodeID),
					Weight:     cat.Strength,
					Confidence: cat.Confidence,
					At:         graph.At,
					Reason:     "category explicit opposing list",
				})
			}
		}
	}
}

/*
RelationOpposingRelation returns RelationContradicts for opposing categories.
*/
func RelationOpposingRelation(_, _ string) RelationType {
	return RelationContradicts
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return nil
}
