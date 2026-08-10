package graph

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
	Strength   float64        `json:"strength,omitempty"`
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

	if _, found := graph.Nodes[edge.From]; !found {
		return
	}

	if _, found := graph.Nodes[edge.To]; !found {
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
	ui             chan []byte
}

/*
NewSolver creates a graph solver.
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
	readySymbols := make([]string, 0)
	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil || symbol.Stamped(types.SourceGraph) ||
			!symbol.Stamped(types.SourceCategory) ||
			!symbol.Stamped(types.SourceResonance) ||
			!symbol.Stamped(types.SourceManifold) ||
			!symbol.Stamped(types.SourceCausal) ||
			!symbol.Stamped(types.SourceCognition) {
			return true
		}

		symbolName, ok := key.(string)

		if ok && symbolName != "" {
			readySymbols = append(readySymbols, symbolName)
		}

		return true
	})

	if len(readySymbols) == 0 {
		return nil
	}

	graph := NewGraph(thesis.At)
	solver.extractCategoryNodes(thesis, graph)
	solver.extractResonanceNodes(thesis, graph)

	if err := solver.extractCausalNodes(thesis, graph); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph: failed to extract causal nodes - "+err.Error(),
			err,
		))
	}

	solver.extractCognitionNodes(thesis, graph)

	if err := solver.inferStructuralEdges(thesis, graph); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph: failed to infer structural edges - "+err.Error(),
			err,
		))
	}

	thesis.Graphs.Store("market_graph", graph)

	for _, symbol := range readySymbols {
		thesis.Stamp(symbol, types.SourceGraph)
	}

	utils.Publish(solver.ui, datura.NewMap("graph", graph))
	thesis.Fanout(types.SourceGraph)
	return nil
}

/*
extractCategoryNodes registers active categories as nodes.
*/
func (solver *Solver) extractCategoryNodes(
	thesis *types.Thesis, graph *Graph,
) {
	thesis.Categories.Range(func(key, value any) bool {
		symbol := key.(string)
		categories := value.([]types.Category)

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

		return true
	})
}

/*
extractResonanceNodes registers predictive coding outcomes (surprise, expected return forecast).
*/
func (solver *Solver) extractResonanceNodes(
	thesis *types.Thesis, graph *Graph,
) {
	if thesis == nil || thesis.Resonance == nil {
		return
	}

	thesis.Resonance.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		reading, readingOK := value.(types.ResonanceReading)

		if !symbolOK || !readingOK || symbol == "" || reading.Forecast == nil {
			return true
		}

		if err := reading.Forecast.Validate(); err != nil ||
			math.IsNaN(reading.Surprise) || math.IsInf(reading.Surprise, 0) {
			return true
		}

		graph.AddNode(&Node{
			ID:         fmt.Sprintf("res:%s:surprise", symbol),
			Symbol:     symbol,
			Source:     "resonance",
			Kind:       "resonance",
			Value:      reading.Surprise,
			Confidence: reading.Forecast.Confidence,
			At:         thesis.At,
		})

		graph.AddNode(&Node{
			ID:         fmt.Sprintf("res:%s:forecast", symbol),
			Symbol:     symbol,
			Source:     "resonance",
			Kind:       "resonance",
			Value:      reading.Forecast.ExpectedReturn,
			Confidence: reading.Forecast.Confidence,
			At:         thesis.At,
		})

		return true
	})
}

/*
causalField maps one Pearl output value to its standardized score and channel
probability. Do-expectation belongs to the intervention channel because it is
the target estimate produced by that intervention.
*/
type causalField struct {
	value            string
	score            string
	probabilityIndex int
}

const causalProbabilityCount = 4

var causalFields = [...]causalField{
	{value: "association", score: "associationScore", probabilityIndex: 0},
	{value: "intervention", score: "interventionScore", probabilityIndex: 1},
	{value: "doExpectation", score: "interventionScore", probabilityIndex: 1},
	{value: "uplift", score: "upliftScore", probabilityIndex: 2},
}

func (field causalField) node(
	symbol string,
	at time.Time,
	causalMap map[string]any,
	probabilities []float64,
	precision float64,
) (*Node, bool, error) {
	fieldValue, found := causalMap[field.value].(float64)

	if !found {
		return nil, false, nil
	}

	if math.IsNaN(fieldValue) || math.IsInf(fieldValue, 0) {
		return nil, false, fmt.Errorf(
			"finite causal value %s required for %s", field.value, symbol,
		)
	}

	strength, found := causalMap[field.score].(float64)

	if !found || math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 {
		return nil, false, fmt.Errorf(
			"finite causal score %s required for %s", field.score, symbol,
		)
	}

	return &Node{
		ID:         fmt.Sprintf("causal:%s:%s", symbol, field.value),
		Symbol:     symbol,
		Source:     "causal",
		Kind:       KindCausal,
		Value:      fieldValue,
		Strength:   strength,
		Confidence: probabilities[field.probabilityIndex] * precision,
		At:         at,
	}, true, nil
}

func causalPrecision(symbol string, causalMap map[string]any) (float64, error) {
	precision, ok := causalMap["precision"].(float64)

	if !ok || math.IsNaN(precision) || math.IsInf(precision, 0) ||
		precision < 0 || precision > 1 {
		return 0, fmt.Errorf(
			"causal precision for %s must be within [0,1]", symbol,
		)
	}

	return precision, nil
}

func causalProbabilities(symbol string, causalMap map[string]any) ([]float64, error) {
	probabilities, ok := causalMap["probabilities"].([]float64)

	if !ok || len(probabilities) != causalProbabilityCount {
		return nil, fmt.Errorf(
			"%d causal probabilities required for %s", causalProbabilityCount, symbol,
		)
	}

	for index, confidence := range probabilities {
		if math.IsNaN(confidence) || math.IsInf(confidence, 0) ||
			confidence < 0 || confidence > 1 {
			return nil, fmt.Errorf(
				"causal probability %d for %s must be within [0,1]", index, symbol,
			)
		}
	}

	return probabilities, nil
}

func causalValuesPresent(causalMap map[string]any) bool {
	for _, field := range causalFields {
		if _, found := causalMap[field.value]; found {
			return true
		}
	}

	return false
}

/*
extractCausalNodes registers Pearl do-calculus and counterfactual uplift outputs.
*/
func (solver *Solver) extractCausalNodes(thesis *types.Thesis, graph *Graph) error {
	var extractErr error

	thesis.Causal.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		causalMap, mapOK := value.(map[string]any)

		if !symbolOK || !mapOK || !causalValuesPresent(causalMap) {
			return true
		}

		probabilities, err := causalProbabilities(symbol, causalMap)

		if err != nil {
			extractErr = err
			return false
		}

		precision, err := causalPrecision(symbol, causalMap)

		if err != nil {
			extractErr = err
			return false
		}

		for _, field := range causalFields {
			node, found, err := field.node(
				symbol,
				thesis.At,
				causalMap,
				probabilities,
				precision,
			)

			if err != nil {
				extractErr = err
				return false
			}

			if found {
				graph.AddNode(node)
			}
		}

		return true
	})

	return extractErr
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
func agreementWeight(left, right float64) (float64, error) {
	leftWeight, err := magnitudeWeight(math.Abs(left))

	if err != nil {
		return 0, fmt.Errorf("left agreement strength: %w", err)
	}

	rightWeight, err := magnitudeWeight(math.Abs(right))

	if err != nil {
		return 0, fmt.Errorf("right agreement strength: %w", err)
	}

	return math.Min(leftWeight, rightWeight), nil
}

/*
inferStructuralEdges indexes nodes by the domains that can produce an
evidence-bearing relationship. Zero-confidence pair relations are not
materialized because their decision weight is necessarily zero.
*/
func (solver *Solver) inferStructuralEdges(thesis *types.Thesis, graph *Graph) error {
	nodes := graph.Nodes
	resonanceBySymbol := make(map[string][]*Node)
	causalBySymbol := make(map[string][]*Node)
	interventions := make([]*Node, 0)
	expectationsBySymbol := make(map[string][]*Node)
	var inferenceErr error

	for _, node := range nodes {
		switch node.Kind {
		case KindResonance:
			if node.ID == "res:"+node.Symbol+":forecast" {
				resonanceBySymbol[node.Symbol] = append(resonanceBySymbol[node.Symbol], node)
			}
		case KindCausal:
			if node.ID == "causal:"+node.Symbol+":uplift" ||
				node.ID == "causal:"+node.Symbol+":doExpectation" {
				causalBySymbol[node.Symbol] = append(causalBySymbol[node.Symbol], node)
			}

			if node.ID == "causal:"+node.Symbol+":intervention" {
				interventions = append(interventions, node)
			}

			if node.ID == "causal:"+node.Symbol+":doExpectation" {
				expectationsBySymbol[node.Symbol] = append(
					expectationsBySymbol[node.Symbol],
					node,
				)
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
				agreement, err := agreementWeight(resonanceNode.Value, causalNode.Value)

				if err != nil {
					return fmt.Errorf(
						"agreement weight from %s to %s: %w",
						resonanceNode.ID,
						causalNode.ID,
						err,
					)
				}

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
			}
		}
	}

	for _, intervention := range interventions {
		for _, expectation := range expectationsBySymbol[intervention.Symbol] {
			weight, err := magnitudeWeight(intervention.Strength)

			if err != nil {
				return fmt.Errorf(
					"condition weight for %s: %w", intervention.ID, err,
				)
			}

			if weight == 0 || intervention.Confidence == 0 {
				continue
			}

			graph.AddEdge(&Edge{
				From:     intervention.ID,
				To:       expectation.ID,
				Relation: RelationConditions,

				/*
					The causal solver standardizes this score against the
					target's robust scale. MagnitudeMargin keeps finite
					evidence below certainty without introducing a fixed cap.
				*/
				Weight:     weight,
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

	// 3. Relate categories through the evidence they actually share.
	thesis.Categories.Range(func(key, value interface{}) bool {
		symbol := key.(string)
		categories := value.([]types.Category)

		for _, category := range categories {
			for _, peer := range categories {
				if category.Type == peer.Type {
					continue
				}

				relation := RelationType("")
				reason := ""

				for _, evidence := range category.Supporting {
					if slices.Contains(peer.Opposing, evidence) {
						relation = RelationContradicts
						reason = "category evidence conflicts on " + evidence
						break
					}

					if relation == "" && slices.Contains(peer.Supporting, evidence) {
						relation = RelationRedundantWith
						reason = "categories share supporting evidence " + evidence
					}
				}

				if relation == "" {
					continue
				}

				weight, err := agreementWeight(category.Strength, peer.Strength)

				if err != nil {
					inferenceErr = fmt.Errorf("category weight for %s: %w", category.Type, err)
					return false
				}

				graph.AddEdge(&Edge{
					From:       fmt.Sprintf("cat:%s:%s", symbol, category.Type),
					To:         fmt.Sprintf("cat:%s:%s", symbol, peer.Type),
					Relation:   relation,
					Weight:     weight,
					Confidence: category.Confidence * peer.Confidence,
					At:         graph.At,
					Reason:     reason,
				})
			}
		}

		return true
	})

	if inferenceErr != nil {
		return inferenceErr
	}

	return nil
}

/*
magnitudeWeight maps a finite dimensionless strength to an open unit interval.
Zero strength carries no edge mass; finite evidence never asserts certainty.
*/
func magnitudeWeight(strength float64) (float64, error) {
	if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"graph: finite, non-negative strength required",
			nil,
		))
	}

	if strength == 0 {
		return 0, nil
	}

	weight, err := probability.MagnitudeMargin(strength)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"graph: invalid weight - "+err.Error(),
			err,
		))
	}

	if weight >= 1 {
		return math.Nextafter(1, 0), nil
	}

	return weight, nil
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	return nil
}
