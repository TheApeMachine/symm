package graph

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
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
	KindMeasurement Kind = "measurement"
	KindCategory    Kind = "category"
	KindManifold    Kind = "manifold"
	KindResonance   Kind = "resonance"
	KindCausal      Kind = "causal"
	KindCognition   Kind = "cognition"
	KindPrediction  Kind = "prediction"
)

/*
Node represents a discrete market entity, metric, category, or latent state.
*/
type Node struct {
	ID            string                `json:"id"`
	Symbol        string                `json:"symbol,omitempty"`
	Peer          string                `json:"peer,omitempty"`
	Source        string                `json:"source,omitempty"`
	MeasurementID string                `json:"measurementId,omitempty"`
	Metric        types.MetricType      `json:"metric,omitempty"`
	Side          types.MeasurementSide `json:"side,omitempty"`
	Kind          Kind                  `json:"kind"`
	Value         float64               `json:"value"`
	Normalized    *float64              `json:"normalized,omitempty"`
	Quality       *float64              `json:"quality,omitempty"`
	Strength      float64               `json:"strength,omitempty"`
	Confidence    float64               `json:"confidence"`
	Maturity      float64               `json:"maturity,omitempty"`
	Unit          types.MeasurementUnit `json:"unit,omitempty"`
	ObservedFrom  time.Time             `json:"observedFrom,omitempty"`
	Horizon       time.Duration         `json:"horizon,omitempty"`
	At            time.Time             `json:"at"`
	Metadata      map[string]any        `json:"metadata,omitempty"`
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
	At              time.Time           `json:"at"`
	Forecast        *learning.RLSOutput `json:"-"`
	ForecastHorizon int                 `json:"forecastHorizon"`
	ForwardCurve    []float64           `json:"-"`
	TaskSkill       float64             `json:"taskSkill"`
	TaskSkillReady  bool                `json:"taskSkillReady"`
	Nodes           map[string]*Node    `json:"nodes"`
	Edges           []*Edge             `json:"edges"`
	Adjacency       map[string][]string `json:"adjacency"` // Fast lookup: NodeID -> []TargetNodeIDs
}

/*
CheckpointState includes the forecast omitted from the dashboard graph wire.
*/
func (graph *Graph) CheckpointState() any {
	if graph == nil {
		return nil
	}

	return struct {
		At              time.Time           `json:"at"`
		Forecast        *learning.RLSOutput `json:"forecast,omitempty"`
		ForecastHorizon int                 `json:"forecastHorizon"`
		ForwardCurve    []float64           `json:"forwardCurve"`
		TaskSkill       float64             `json:"taskSkill"`
		TaskSkillReady  bool                `json:"taskSkillReady"`
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
		Nodes:           graph.Nodes,
		Edges:           graph.Edges,
		Adjacency:       graph.Adjacency,
	}
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

	graph.Nodes[node.ID] = node
}

/*
AddEdge connects two nodes with a directional, weighted relationship.
*/
func (graph *Graph) AddEdge(edge *Edge) {
	if edge == nil || edge.From == "" || edge.To == "" {
		panic("graph: edge and endpoint IDs required")
	}

	if _, found := graph.Nodes[edge.From]; !found {
		panic("graph: source node not registered: " + edge.From)
	}

	if _, found := graph.Nodes[edge.To]; !found {
		panic("graph: target node not registered: " + edge.To)
	}

	for index, current := range graph.Edges {
		if current.From == edge.From && current.To == edge.To &&
			slices.Equal(current.Evidence, edge.Evidence) {
			graph.Edges[index] = edge
			return
		}
	}

	graph.Edges = append(graph.Edges, edge)

	if !slices.Contains(graph.Adjacency[edge.From], edge.To) {
		graph.Adjacency[edge.From] = append(graph.Adjacency[edge.From], edge.To)
	}
}

func (graph *Graph) Roots() []string {
	incoming := make(map[string]bool)

	for _, edge := range graph.Edges {
		incoming[edge.To] = true
	}

	roots := make([]string, 0)

	for nodeID, node := range graph.Nodes {
		if !incoming[nodeID] || node.Kind == KindCognition {
			roots = append(roots, nodeID)
		}
	}

	slices.Sort(roots)
	return roots
}

/*
ReadyForSearch reports whether the accumulated lifecycle graph has a calibrated
return forecast and at least one reachable confidence-weighted relation.
*/
func (graph *Graph) ReadyForSearch() bool {
	if graph == nil || graph.Forecast == nil || !graph.Forecast.Ready ||
		graph.ForecastHorizon < 1 || graph.Forecast.Scale <= 0 ||
		graph.Forecast.DegreesOfFreedom <= 0 {
		return false
	}

	visited := make(map[string]bool)
	queue := append([]string(nil), graph.Roots()...)

	for len(queue) > 0 {
		source := queue[0]
		queue = queue[1:]

		if visited[source] {
			continue
		}

		visited[source] = true

		for _, edge := range graph.Edges {
			if edge.From != source {
				continue
			}

			queue = append(queue, edge.To)
			if graph.Nodes[edge.To] != nil && edge.Weight != 0 &&
				edge.Confidence != 0 {
				return true
			}
		}
	}

	return false
}

func (graph *Graph) Targets(nodeID string) []string {
	return slices.Clone(graph.Adjacency[nodeID])
}

func (graph *Graph) NodeValue(nodeID string) (float64, float64) {
	node := graph.Nodes[nodeID]
	return node.Value, node.Confidence
}

func (graph *Graph) EdgeValue(from, to string) (float64, float64) {
	evidenceMass := 0.0
	confidenceMass := 0.0
	relationCount := 0

	for _, edge := range graph.Edges {
		if edge.From != from || edge.To != to {
			continue
		}

		weight := edge.Weight

		if edge.Relation == RelationContradicts {
			weight = -weight
		}

		evidenceMass += weight * edge.Confidence
		confidenceMass += edge.Confidence
		relationCount++
	}

	if relationCount > 0 && confidenceMass > 0 {
		return evidenceMass / confidenceMass,
			confidenceMass / float64(relationCount)
	}

	panic("graph: edge not found from " + from + " to " + to)
}

/*
Solver compiles all upstream evidence (Measurements, Manifold, Resonance, Causal, Cognition)
into a Directed Knowledge Graph for the Strategy package.
*/
type Solver struct {
	recorder     *audit.Recorder
	measurements *measurementCompiler
	ui           chan []byte
}

/*
NewSolver creates a graph solver.
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder) *Solver {
	solver := &Solver{
		recorder:     recorder,
		measurements: newMeasurementCompiler(),
		ui:           ui,
	}

	return solver
}

/*
Update streams newly available evidence into the graph owned by the current
Thesis lifecycle. The graph is replaced only after a completed planner evaluation.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	var graphErr error

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		symbolName, ok := key.(string)

		if !ok || symbolName == "" {
			return true
		}

		storedGraph, _ := symbol.Graphs.LoadOrStore(
			"market_graph",
			NewGraph(thesis.At),
		)
		graph, valid := storedGraph.(*Graph)

		if !valid || graph == nil {
			graphErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: invalid lifecycle graph for "+symbolName,
				nil,
			))
			return false
		}

		graph.At = thesis.At
		lifecycleEmpty := len(graph.Nodes) == 0
		measurementIndex, err := solver.measurements.addNodes(
			symbolName,
			symbol.MarketMeasurements("graph"),
			graph,
		)

		if err != nil {
			graphErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to extract measurement nodes - "+err.Error(),
				err,
			))
			return false
		}

		if lifecycleEmpty && len(measurementIndex.bySource) == 0 {
			return true
		}

		solver.extractCategoryNodes(symbol, graph)
		solver.extractManifoldNodes(symbol, graph)
		solver.extractResonanceNodes(symbol, graph)

		causalValue, causalFound := symbol.Causal.Load(symbolName)

		if causalFound {
			causalMap, mapOK := causalValue.(map[string]any)

			if !mapOK {
				graphErr = errnie.Error(errnie.Err(
					errnie.Validation,
					"graph: invalid causal artifact for "+symbolName,
					nil,
				))
				return false
			}

			if causalValuesPresent(causalMap) {
				if err := solver.extractCausalNodes(symbol, graph); err != nil {
					graphErr = errnie.Error(errnie.Err(
						errnie.Internal,
						"graph: failed to extract causal nodes - "+err.Error(),
						err,
					))
					return false
				}
			}
		}

		solver.extractCognitionNodes(symbol, graph)

		if err := solver.measurements.addCategoryEdges(
			symbol, graph, measurementIndex,
		); err != nil {
			graphErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to relate measurements and categories - "+err.Error(),
				err,
			))
			return false
		}

		if err := solver.measurements.addLeadLagEdges(
			symbol, graph, measurementIndex,
		); err != nil {
			graphErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to relate lead-lag measurements - "+err.Error(),
				err,
			))
			return false
		}

		if err := solver.inferStructuralEdges(symbol, graph); err != nil {
			graphErr = errnie.Error(errnie.Err(
				errnie.Internal,
				"graph: failed to infer structural edges - "+err.Error(),
				err,
			))
			return false
		}

		if symbolName == types.Focus() {
			utils.Publish(solver.ui, datura.NewMap("graph", graph))
		}

		return true
	})

	return graphErr
}

/*
extractCategoryNodes registers active categories as nodes.
*/
func (solver *Solver) extractCategoryNodes(
	symbol *types.Symbol, graph *Graph,
) {
	stored, found := symbol.Categories.Load(symbol.Symbol)

	if !found {
		return
	}

	categories := stored.([]types.Category)

	for _, cat := range categories {
		if cat.Type == types.CategoryTypeNone {
			continue
		}

		nodeID := fmt.Sprintf("cat:%s:%s", symbol.Symbol, string(cat.Type))

		metadata := map[string]any{
			"type":       string(cat.Type),
			"supporting": cat.Supporting,
			"opposing":   cat.Opposing,
		}

		if cat.Surprisal > 0 {
			metadata["surprisal"] = cat.Surprisal
		}

		if cat.Maturity > 0 {
			metadata["maturity"] = cat.Maturity
		}

		graph.AddNode(&Node{
			ID:         nodeID,
			Symbol:     symbol.Symbol,
			Source:     "category",
			Kind:       KindCategory,
			Value:      cat.Strength,
			Confidence: cat.Confidence,
			Maturity:   cat.Maturity,
			At:         graph.At,
			Metadata:   metadata,
		})
	}
}

/*
extractManifoldNodes registers the per-symbol phase alignment actually retained
by the manifold stage. The shared fluid field is not duplicated into each
pair's graph.
*/
func (solver *Solver) extractManifoldNodes(
	symbol *types.Symbol,
	graph *Graph,
) {
	stored, found := symbol.Phase.Load(symbol.Symbol)
	reading, readingOK := stored.(types.PhaseReading)

	if !found || !readingOK {
		return
	}

	alignment, aligned := reading.Alignment()

	if !aligned {
		return
	}

	graph.AddNode(&Node{
		ID:         fmt.Sprintf("man:%s:phase_alignment", symbol.Symbol),
		Symbol:     symbol.Symbol,
		Source:     "manifold",
		Kind:       KindManifold,
		Value:      alignment.Outcome.Return,
		Strength:   alignment.Similarity,
		Confidence: alignment.Similarity,
		At:         reading.At,
		Metadata: map[string]any{
			"angle":      alignment.Angle,
			"horizon":    alignment.Outcome.Horizon,
			"observedAt": alignment.ObservedAt,
			"outcome":    alignment.Outcome,
		},
	})
}

/*
extractResonanceNodes registers predictive coding outcomes (surprise, expected return forecast).
*/
func (solver *Solver) extractResonanceNodes(
	symbol *types.Symbol, graph *Graph,
) {
	if symbol == nil {
		return
	}

	stored, found := symbol.Resonance.Load(types.ResonanceReturnForecastKey)

	if !found {
		return
	}

	returnForecast, ok := stored.(*types.ResonanceReturnForecast)

	if !ok {
		return
	}

	if returnForecast.Horizon < 1 || !returnForecast.Distribution.Ready {
		return
	}

	if coderValue, coderFound := symbol.Resonance.Load(symbol.Symbol); coderFound {
		if coder, valid := coderValue.(*learning.ResonanceManifold); valid {
			graph.TaskSkill, graph.TaskSkillReady = coder.TaskSkill()
		}
	}

	graphForecast := returnForecast.Distribution
	graph.Forecast = &graphForecast
	graph.ForecastHorizon = returnForecast.Horizon
	graph.ForwardCurve = slices.Clone(returnForecast.ForwardCurve)

	coderValue, found := symbol.Resonance.Load(symbol.Symbol)

	if !found {
		return
	}

	coder, ok := coderValue.(*learning.ResonanceManifold)

	if !ok {
		return
	}

	layers, surprise, _ := coder.WireSnapshot()

	if len(layers) == 0 || math.IsNaN(surprise) || math.IsInf(surprise, 0) {
		return
	}

	graph.AddNode(&Node{
		ID:         fmt.Sprintf("res:%s:surprise", symbol.Symbol),
		Symbol:     symbol.Symbol,
		Source:     "resonance",
		Kind:       KindResonance,
		Value:      surprise,
		Confidence: 1,
		At:         graph.At,
	})

	graph.AddNode(&Node{
		ID:         fmt.Sprintf("res:%s:forecast", symbol.Symbol),
		Symbol:     symbol.Symbol,
		Source:     "resonance",
		Kind:       KindResonance,
		Value:      graphForecast.Value,
		Confidence: 1,
		At:         graph.At,
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
		Metadata: map[string]any{
			"horizon": 1,
		},
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
func (solver *Solver) extractCausalNodes(
	symbol *types.Symbol, graph *Graph,
) error {
	stored, found := symbol.Causal.Load(symbol.Symbol)
	causalMap, mapOK := stored.(map[string]any)

	if !found || !mapOK || !causalValuesPresent(causalMap) {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"causal: no causal values found for symbol "+symbol.Symbol,
			nil,
		))
	}

	probabilities, err := causalProbabilities(symbol.Symbol, causalMap)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"causal: failed to extract probabilities for symbol "+symbol.Symbol+" - "+err.Error(),
			err,
		))
	}

	precision, err := causalPrecision(symbol.Symbol, causalMap)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"causal: failed to extract precision for symbol "+symbol.Symbol+" - "+err.Error(),
			err,
		))
	}

	for _, field := range causalFields {
		node, found, err := field.node(
			symbol.Symbol,
			causalMap,
			probabilities,
			precision,
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"causal: failed to extract node for symbol "+symbol.Symbol+" - "+err.Error(),
				err,
			))
		}

		if found {
			node.At = graph.At
			graph.AddNode(node)
		}
	}

	return nil
}

/*
extractCognitionNodes registers active category sequences and lookahead predictions.
*/
func (solver *Solver) extractCognitionNodes(
	symbol *types.Symbol, graph *Graph,
) {
	stored, found := symbol.Cognition.Load(symbol.Symbol)
	cognition, cognitionOK := stored.(types.Cognition)

	if !found || !cognitionOK ||
		cognition.Winner == "" {
		return
	}

	nodeID := fmt.Sprintf("cog:%s:winner_regime", symbol.Symbol)
	graph.AddNode(&Node{
		ID:         nodeID,
		Symbol:     symbol.Symbol,
		Source:     cognition.Source,
		Kind:       KindCognition,
		Value:      cognition.Confidence,
		Confidence: cognition.Confidence,
		At:         cognition.At,
		Metadata: map[string]any{
			"regime":   cognition.Winner,
			"sequence": cognition.Sequence,
		},
	})

	for path, probability := range cognition.Predictions {
		if path == "" || probability <= 0 {
			continue
		}

		graph.AddNode(&Node{
			ID:         fmt.Sprintf("cog:%s:prediction:%s", symbol.Symbol, path),
			Symbol:     symbol.Symbol,
			Source:     cognition.Source,
			Kind:       KindPrediction,
			Value:      probability,
			Confidence: probability,
			At:         cognition.At,
			Metadata: map[string]any{
				"path": path,
			},
		})
	}
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
func (solver *Solver) inferStructuralEdges(
	symbol *types.Symbol, graph *Graph,
) error {
	nodes := graph.Nodes
	resonanceBySymbol := make(map[string][]*Node)
	causalBySymbol := make(map[string][]*Node)
	interventions := make([]*Node, 0)
	expectationsBySymbol := make(map[string][]*Node)

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
						Evidence:   []string{resonanceNode.ID, causalNode.ID},
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
						Evidence:   []string{resonanceNode.ID, causalNode.ID},
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
				Evidence:   []string{intervention.ID, expectation.ID},
				At:         graph.At,
				Reason:     "interventional level conditions do-expectation",
			})
		}
	}

	// 2. Evaluate Leads & Lags from Cognition Beam Search
	stored, found := symbol.Cognition.Load(symbol.Symbol)
	cognition, cognitionOK := stored.(types.Cognition)

	if found && cognitionOK &&
		cognition.Winner != "" {
		currentNodeID := fmt.Sprintf("cog:%s:winner_regime", symbol.Symbol)

		for path, probability := range cognition.Predictions {
			if path == "" || probability <= 0 {
				continue
			}

			targetNodeID := fmt.Sprintf("cog:%s:prediction:%s", symbol.Symbol, path)
			graph.AddEdge(&Edge{
				From:       currentNodeID,
				To:         targetNodeID,
				Relation:   RelationLeads,
				Weight:     probability,
				Confidence: probability,
				Evidence:   []string{currentNodeID, targetNodeID},
				At:         graph.At,
				Reason:     "cognition beam search lookahead prediction",
			})
			graph.AddEdge(&Edge{
				From:       targetNodeID,
				To:         currentNodeID,
				Relation:   RelationLags,
				Weight:     probability,
				Confidence: probability,
				Evidence:   []string{currentNodeID, targetNodeID},
				At:         graph.At,
				Reason:     "inverse temporal lag of beam search lookahead",
			})
		}
	}

	// 3. Relate categories through the evidence they actually share.
	stored, found = symbol.Categories.Load(symbol.Symbol)

	if !found {
		return nil
	}

	categories := stored.([]types.Category)

	for _, category := range categories {
		for _, peer := range categories {
			if category.Type == types.CategoryTypeNone ||
				peer.Type == types.CategoryTypeNone || category.Type == peer.Type {
				continue
			}

			relation := RelationType("")
			reason := ""
			evidenceReference := ""

			for _, evidence := range category.Supporting {
				if slices.Contains(peer.Opposing, evidence) {
					relation = RelationContradicts
					reason = "category evidence conflicts on " + evidence
					evidenceReference = evidence
					break
				}

				if relation == "" && slices.Contains(peer.Supporting, evidence) {
					relation = RelationRedundantWith
					reason = "categories share supporting evidence " + evidence
					evidenceReference = evidence
				}
			}

			if relation == "" {
				continue
			}

			weight, err := agreementWeight(category.Strength, peer.Strength)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"graph: failed to compute agreement weight between categories - "+err.Error(),
					err,
				))
			}

			graph.AddEdge(&Edge{
				From:       fmt.Sprintf("cat:%s:%s", symbol.Symbol, category.Type),
				To:         fmt.Sprintf("cat:%s:%s", symbol.Symbol, peer.Type),
				Relation:   relation,
				Weight:     weight,
				Confidence: category.Confidence * peer.Confidence,
				Evidence:   []string{evidenceReference},
				At:         graph.At,
				Reason:     reason,
			})
		}
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
