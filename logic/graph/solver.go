package graph

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
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
	KindMeasurement Kind = "measurement"
	KindCategory    Kind = "category"
	KindResonance   Kind = "resonance"
	KindCausal      Kind = "causal"
	KindCognition   Kind = "cognition"
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

	// 1. Extract and register all nodes from Thesis
	solver.extractMeasurementNodes(thesis, graph)
	solver.extractCategoryNodes(thesis, graph)
	solver.extractResonanceNodes(thesis, graph)
	solver.extractCausalNodes(thesis, graph)
	solver.extractCognitionNodes(thesis, graph)

	// 2. Infer directional relationships between nodes
	solver.inferStructuralEdges(thesis, graph)

	// 3. Store compiled Graph into thesis.Graphs
	thesis.Graphs.Store("market_graph", graph)

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

	rows := make([]datura.Map[any], 0, len(graph.Nodes))
	at := thesis.At.Format(time.RFC3339)

	for _, node := range graph.Nodes {
		if node == nil {
			continue
		}

		row := datura.NewMap(
			"source", "graph",
			"symbol", node.Symbol,
			"at", at,
			"id", node.ID,
			"kind", string(node.Kind),
			"value", node.Value,
			"confidence", node.Confidence,
		)

		if node.Source != "" {
			row["nodeSource"] = node.Source
		}

		if len(node.Metadata) > 0 {
			row["metadata"] = node.Metadata
		}

		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return
	}

	select {
	case solver.ui <- datura.NewMap("graph", rows).MarshalAndFree():
	default:
	}
}

/*
extractMeasurementNodes normalizes Thesis measurement storage before materializing
metric nodes so graph extraction accepts both singleton rows and the slice-backed
values AppendMeasuremnts stores per source. That normalization keeps graph
building tolerant of older or ad-hoc thesis producers while emitting the same
measurement-node surface for downstream reasoning.
*/
func (solver *Solver) extractMeasurementNodes(thesis *types.Thesis, graph *Graph) {
	thesis.Measurements.Range(func(key, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			if single, singleOK := value.(*types.Measurement); singleOK && single != nil {
				rows = []*types.Measurement{single}
			} else {
				return true
			}
		}

		for _, m := range rows {
			if m == nil || m.Symbol == "" {
				continue
			}

			for metricKey, metric := range m.Metrics {
				nodeID := fmt.Sprintf("meas:%s:%s:%s", m.Symbol, m.Source, metricKey)
				confidence := 1.0
				if m.Uncertainty != nil {
					confidence = m.Uncertainty.Confidence
				}

				graph.AddNode(&Node{
					ID:         nodeID,
					Symbol:     m.Symbol,
					Source:     string(m.Source),
					Kind:       "measurement",
					Value:      metric.Raw,
					Confidence: confidence,
					At:         m.At,
					Metadata: map[string]any{
						"readiness": string(m.Validity.Readiness),
						"state":     string(m.Validity.State),
						"unit":      string(metric.Unit),
					},
				})
			}
		}

		return true
	})
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
	thesis.Resonance.Range(func(key, value any) bool {
		resMap, ok := value.(map[string]any)
		if !ok {
			return true
		}

		symbol, _ := key.(string)

		if surprise, ok := resMap["surprise"].(float64); ok {
			nodeID := fmt.Sprintf("res:%s:surprise", symbol)
			graph.AddNode(&Node{
				ID:         nodeID,
				Symbol:     symbol,
				Source:     "resonance",
				Kind:       "resonance",
				Value:      surprise,
				Confidence: 1.0,
				At:         thesis.At,
			})
		}

		var predValue float64
		if pred, ok := resMap["taskPrediction"].([]float64); ok && len(pred) > 0 {
			predValue = pred[0]
		} else if curve, ok := resMap["forwardCurve"].([]float64); ok && len(curve) > 0 {
			predValue = curve[0]
		}

		nodeID := fmt.Sprintf("res:%s:forecast", symbol)
		graph.AddNode(&Node{
			ID:         nodeID,
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

		for _, field := range []string{"doExpectation", "uplift", "association", "intervention"} {
			if val, ok := causalMap[field].(float64); ok {
				nodeID := fmt.Sprintf("causal:%s:%s", symbol, field)
				graph.AddNode(&Node{
					ID:         nodeID,
					Symbol:     symbol,
					Source:     "causal",
					Kind:       "causal",
					Value:      val,
					Confidence: 1.0,
					At:         thesis.At,
				})
			}
		}

		return true
	})
}

/*
extractCognitionNodes registers active category sequences and lookahead predictions.
*/
func (solver *Solver) extractCognitionNodes(thesis *types.Thesis, graph *Graph) {
	thesis.Cognition.Range(func(key, value any) bool {
		symbol, _ := key.(string)
		cogMap, ok := value.(map[string]any)
		if !ok {
			return true
		}

		if winner, ok := cogMap["winnerRegime"].(string); ok && winner != "" {
			nodeID := fmt.Sprintf("cog:%s:winner_regime", symbol)
			conf, _ := cogMap["confidence"].(float64)

			graph.AddNode(&Node{
				ID:         nodeID,
				Symbol:     symbol,
				Source:     "cognition",
				Kind:       "cognition",
				Value:      conf,
				Confidence: conf,
				At:         thesis.At,
				Metadata: map[string]any{
					"regime": winner,
				},
			})
		}

		return true
	})
}

/*
inferStructuralEdges evaluates all 9 relational edge types across registered nodes.
*/
func (solver *Solver) inferStructuralEdges(thesis *types.Thesis, graph *Graph) {
	nodes := graph.Nodes

	// 1. Evaluate StaleRelative, Incomparable, Supports, Contradicts, Conditions, Independence
	for idA, nodeA := range nodes {
		for idB, nodeB := range nodes {
			if idA == idB {
				continue
			}

			// Relation: Stale Relative To
			if !nodeA.At.IsZero() && !nodeB.At.IsZero() {
				if nodeA.At.Add(solver.staleThreshold).Before(nodeB.At) {
					graph.AddEdge(&Edge{
						From:     idA,
						To:       idB,
						Relation: RelationStaleRelativeTo,
						Weight:   nodeB.At.Sub(nodeA.At).Seconds(),
						At:       graph.At,
						Reason:   "timestamp outdated relative to target node",
					})
				}
			}

			// Relation: Incomparable With (Low confidence or invalid readiness)
			if nodeA.Confidence <= 0 || nodeB.Confidence <= 0 {
				graph.AddEdge(&Edge{
					From:     idA,
					To:       idB,
					Relation: RelationIncomparableWith,
					Weight:   0.0,
					At:       graph.At,
					Reason:   "zero confidence or unvalidated metric scale",
				})
			}

			// Cross-Domain Inference: Resonance Forecast vs Causal Uplift
			if nodeA.Kind == "resonance" && nodeB.Kind == "causal" && nodeA.Symbol == nodeB.Symbol {
				if nodeA.Value > 0 && nodeB.Value > 0 { // Both agree +
					graph.AddEdge(&Edge{
						From:       idA,
						To:         idB,
						Relation:   RelationSupports,
						Weight:     math.Min(nodeA.Value, nodeB.Value),
						Confidence: nodeA.Confidence * nodeB.Confidence,
						At:         graph.At,
						Reason:     "predictive forecast and causal uplift agree directionally (+)",
					})
				} else if (nodeA.Value > 0 && nodeB.Value < 0) || (nodeA.Value < 0 && nodeB.Value > 0) { // Conflict!
					graph.AddEdge(&Edge{
						From:       idA,
						To:         idB,
						Relation:   RelationContradicts,
						Weight:     math.Abs(nodeA.Value - nodeB.Value),
						Confidence: nodeA.Confidence * nodeB.Confidence,
						At:         graph.At,
						Reason:     "predictive forecast and causal uplift conflict in direction",
					})
				} else if math.Abs(nodeA.Value) < 0.01 && math.Abs(nodeB.Value) < 0.01 {
					graph.AddEdge(&Edge{
						From:       idA,
						To:         idB,
						Relation:   RelationIndependentOf,
						Weight:     1.0,
						Confidence: 1.0,
						At:         graph.At,
						Reason:     "zero forecast magnitude and zero causal uplift",
					})
				}
			}

			// Relation: Conditions (Pearl Condition Number / Energy Precondition)
			if idA == fmt.Sprintf("causal:%s:intervention", nodeA.Symbol) &&
				idB == fmt.Sprintf("causal:%s:doExpectation", nodeB.Symbol) {
				graph.AddEdge(&Edge{
					From:       idA,
					To:         idB,
					Relation:   RelationConditions,
					Weight:     nodeA.Value,
					Confidence: nodeA.Confidence,
					At:         graph.At,
					Reason:     "interventional level conditions do-expectation",
				})
			}
		}
	}

	// 2. Evaluate Leads & Lags from Cognition Beam Search
	thesis.Cognition.Range(func(key, value any) bool {
		symbol, _ := key.(string)
		cogMap, ok := value.(map[string]any)

		if !ok {
			return true
		}

		if preds, ok := cogMap["predictions"].([]map[string]any); ok {
			currNodeID := fmt.Sprintf("cog:%s:winner_regime", symbol)

			for index, pred := range preds {
				if path, ok := pred["predictedPath"].(string); ok && path != "" {
					targetNodeID := fmt.Sprintf("cat:%s:%s", symbol, path)

					// Relation: Leads (Current Regime leads Predicted Category)
					graph.AddEdge(&Edge{
						From:       currNodeID,
						To:         targetNodeID,
						Relation:   RelationLeads,
						Weight:     float64(index + 1),
						Confidence: pred["probability"].(float64),
						At:         graph.At,
						Reason:     "cognition beam search lookahead prediction",
					})

					// Relation: Lags (Inverse Edge)
					graph.AddEdge(&Edge{
						From:       targetNodeID,
						To:         currNodeID,
						Relation:   RelationLags,
						Weight:     float64(index + 1),
						Confidence: pred["probability"].(float64),
						At:         graph.At,
						Reason:     "inverse temporal lag of beam search lookahead",
					})
				}
			}
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
