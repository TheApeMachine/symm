package graph

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
Solver compiles all upstream evidence (Measurements, Manifold, Resonance, Causal, Cognition)
into a Directed Knowledge Graph for the Strategy package.
*/
type Solver struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	recorder     *audit.Recorder
	measurements *measurementCompiler
	ui           *transport.MapReduce[*types.UIFrame]
	lastBuilt    map[string]*types.Graph
}

/*
NewSolver creates a graph solver.
*/
func NewSolver(thesis *types.Thesis, ui *transport.MapReduce[*types.UIFrame], recorder *audit.Recorder) *Solver {
	ctx, cancel := context.WithCancel(context.Background())

	solver := &Solver{
		ctx:          ctx,
		cancel:       cancel,
		thesis:       thesis,
		recorder:     recorder,
		measurements: newMeasurementCompiler(),
		ui:           ui,
		lastBuilt:    make(map[string]*types.Graph),
	}

	return solver
}

func (solver *Solver) Name() string {
	return "graph"
}

func (solver *Solver) Error() error { return solver.err }

/*
Run consumes each symbol's lifecycle-graph stream (the planner backfeed pushed
onto the Graphs MapReduce) and rebuilds the fully-connected graph from all
upstream evidence, writing the result back to the same Graphs output MapReduce
the next stage consumes.
*/
func (solver *Solver) Run() error {
	if solver.thesis == nil {
		return nil
	}

	for solver.err == nil {
		symbol, available := solver.thesis.Work(types.SourceGraph).WaitPop(
			solver.ctx,
			string(types.SourceGraph),
		)

		if !available {
			return solver.ctx.Err()
		}

		if symbol == nil ||
			symbol.Graphs.ConsumerLength(string(types.SourcePlanner)) == 0 {
			continue
		}

		solver.buildGraph(symbol.Symbol, symbol)
	}

	return solver.err
}

/*
buildGraph streams every lifecycle graph queued for this symbol and rebuilds
each into an isolated clone before publishing it back to the Graphs output.
*/
func (solver *Solver) buildGraph(symbolName string, symbol *types.Symbol) {
	graphs := make([]*types.Graph, 0, 1)

	for graph := range symbol.MarketGraphs(SourcePlanner) {
		if graph == nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: invalid lifecycle graph for "+symbolName,
				nil,
			))

			return
		}

		// The re-published graph sits back on the same output MapReduce the
		// planner pushes requests onto. Skip our own output so the stage does
		// not livelock rebuilding the graph it just wrote.
		if graph == solver.lastBuilt[symbolName] {
			continue
		}

		graphs = append(graphs, graph)
	}

	if len(graphs) == 0 {
		graph := solver.lastBuilt[symbolName]

		if graph == nil {
			graph = types.NewGraph(solver.thesis.At)
		}

		graphs = append(graphs, graph)
	}

	for _, graph := range graphs {

		// Build this pass into an isolated clone and publish it only when
		// complete. Readers (the planner's ReadyForSearch/OpportunitySummary/
		// Roots) can therefore never observe a half-built graph, and no map is
		// ever read while it is being written — the fatal concurrent map
		// read-and-write cannot happen. The published graph is never mutated
		// again; the next pass clones it.
		graph = graph.Clone()
		graph.At = solver.thesis.At
		lifecycleEmpty := len(graph.Nodes) == 0
		categories := solver.popCategories(symbol)
		cognition, _ := solver.popCognition(symbol)
		measurementIndex, err := solver.measurements.addNodes(
			symbolName,
			symbol.MarketMeasurements("graph"),
			graph,
		)

		if err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to extract measurement nodes - "+err.Error(),
				err,
			))

			return
		}

		if lifecycleEmpty && len(measurementIndex.bySource) == 0 {
			continue
		}

		solver.extractCategoryNodes(symbol, categories, graph)
		solver.extractManifoldNodes(solver.thesis, graph)
		solver.extractResonanceNodes(symbol, graph)

		if err := solver.extractCausalNodes(symbol, graph); err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Internal,
				"graph: failed to extract causal nodes - "+err.Error(),
				err,
			))

			return
		}

		solver.extractCognitionNodes(symbol, cognition, graph)

		if err := solver.measurements.addCategoryEdges(
			categories, symbol.Symbol, graph, measurementIndex,
		); err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to relate measurements and categories - "+err.Error(),
				err,
			))

			return
		}

		if err := solver.measurements.addLeadLagEdges(
			symbol, graph, measurementIndex,
		); err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"graph: failed to relate lead-lag measurements - "+err.Error(),
				err,
			))

			return
		}

		if err := solver.inferStructuralEdges(
			symbol, categories, cognition, graph,
		); err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Internal,
				"graph: failed to infer structural edges - "+err.Error(),
				err,
			))

			return
		}

		if err := solver.connectLongOpportunity(symbol, graph); err != nil {
			solver.err = errnie.Error(errnie.Err(
				errnie.Internal,
				"graph: failed to connect long-opportunity hypothesis - "+err.Error(),
				err,
			))

			return
		}

		if symbolName == types.Focus() {
			solver.thesis.Publish(&wire.FrameT{
				Type:  wire.FrameGraphFrame,
				Value: graph.Wire(),
			})
		}

		solver.lastBuilt[symbolName] = graph
		symbol.Graphs.Push(graph)
	}
}

/*
extractCategoryNodes registers active categories as nodes.
*/
func (solver *Solver) extractCategoryNodes(
	symbol *types.Symbol,
	categories []types.Category,
	graph *types.Graph,
) {
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

func (solver *Solver) popCategories(symbol *types.Symbol) []types.Category {
	var categories []types.Category

	for batch := range symbol.MarketCategories(types.SourceGraph) {
		categories = batch
	}

	return categories
}

/*
extractManifoldNodes registers the universe phase alignment retained by the
manifold stage. The shared fluid field is not duplicated into a per-symbol
fingerprint; each graph reads the same sweep.
*/
func (solver *Solver) extractManifoldNodes(
	thesis *types.Thesis,
	graph *types.Graph,
) {
	if thesis == nil || graph == nil {
		return
	}

	if reading, found := thesis.PhaseSnapshot(); found {
		if inference, ready := reading.Inference(); ready {
			graph.AddNode(&Node{
				ID:         "man:universe:phase_direction",
				Source:     "manifold",
				Kind:       KindManifold,
				Value:      inference.Direction,
				Strength:   inference.Confidence,
				Confidence: inference.Confidence,
				At:         reading.At,
				Metadata: map[string]any{
					"support":       inference.Support,
					"contradiction": inference.Contradiction,
					"balance":       inference.Balance,
					"responses":     inference.Responses,
				},
			})
		}
	}

	reading, found := thesis.ManifoldSnapshot()

	if !found || !reading.IsFinite() {
		return
	}

	fields := []struct {
		name  string
		value float64
	}{
		{name: "divergence", value: reading.Divergence},
		{name: "guidance_speed", value: reading.GuidanceSpeed},
		{name: "coherence", value: reading.CoherenceMag2},
		{name: "pressure_gradient", value: reading.PressureGradNorm},
		{name: "viscosity", value: reading.ViscosityProxy},
	}

	for _, field := range fields {
		graph.AddNode(&Node{
			ID:         "man:universe:" + field.name,
			Source:     "manifold",
			Kind:       KindManifold,
			Value:      field.value,
			Strength:   math.Abs(field.value),
			Confidence: 1,
			At:         graph.At,
			Metadata: map[string]any{
				"observer": "relaxed physical field",
			},
		})
	}
}

/*
extractResonanceNodes registers predictive coding outcomes (surprise and the
direction call). The call is not a priced return.
*/
func (solver *Solver) extractResonanceNodes(
	symbol *types.Symbol, graph *types.Graph,
) {
	if symbol == nil || graph == nil {
		return
	}

	var (
		returnForecast *types.ResonanceReturnForecast
		coder          *learning.ResonanceManifold
		dynamics       nomagique.Frame
	)

	for stored := range symbol.MarketResonance(types.SourceGraph) {
		switch value := stored.(type) {
		case *types.ResonanceReturnForecast:
			returnForecast = value
		case *learning.ResonanceManifold:
			coder = value
		case nomagique.Frame:
			dynamics = value
		}
	}

	if returnForecast != nil && returnForecast.Distribution.Ready {
		graphForecast := returnForecast.Distribution
		graph.SetResonanceOutput(&graphForecast, max(0, returnForecast.Horizon))

		if returnForecast.Call != 0 {
			confidence := directionPosteriorConfidence(
				returnForecast.Distribution,
				returnForecast.Call,
			)

			if confidence > 0 {
				graph.AddNode(&types.Node{
					ID:         fmt.Sprintf("res:%s:forecast", symbol.Symbol),
					Symbol:     symbol.Symbol,
					Source:     "resonance",
					Kind:       KindResonance,
					Value:      returnForecast.Call,
					Strength:   confidence,
					Confidence: confidence,
					At:         graph.At,
					Metadata: map[string]any{
						"horizon":   returnForecast.Horizon,
						"held":      returnForecast.Held,
						"candidate": returnForecast.CandidateCall,
					},
				})
			}
		}
	}

	if coder == nil {
		return
	}

	skill, skillReady := coder.TaskSkill()
	graph.SetTaskSkill(skill, skillReady)
	layers, surprise, _ := coder.WireSnapshot()

	if len(layers) == 0 || math.IsNaN(surprise) || math.IsInf(surprise, 0) {
		return
	}

	graph.AddNode(&types.Node{
		ID:         fmt.Sprintf("res:%s:surprise", symbol.Symbol),
		Symbol:     symbol.Symbol,
		Source:     "resonance",
		Kind:       KindResonance,
		Value:      surprise,
		Strength:   math.Abs(surprise),
		Confidence: 1,
		At:         graph.At,
	})

	solver.extractPredictiveDynamicsNodes(
		symbol.Symbol,
		dynamics,
		graph,
	)
}

func (solver *Solver) popCognition(symbol *types.Symbol) (types.Cognition, bool) {
	var cognition types.Cognition

	for stored := range symbol.MarketCognition(types.SourceGraph) {
		cognition = stored
	}

	if cognition.Winner == "" {
		return types.Cognition{}, false
	}

	return cognition, true
}

func (solver *Solver) extractPredictiveDynamicsNodes(
	symbol string,
	dynamics nomagique.Frame,
	graph *types.Graph,
) {
	ready, _ := dynamics.Get(learning.SymbolDynamicsReady)
	sampleCount, _ := dynamics.Get(learning.SymbolDynamicsSampleCount)
	confidence := sampleCount / (sampleCount + 1)

	if ready > confidence {
		confidence = ready
	}

	fields := []struct {
		name   string
		symbol nomagique.Symbol
	}{
		{name: "generalized_velocity", symbol: learning.SymbolDynamicsVelocity},
		{name: "generalized_acceleration", symbol: learning.SymbolDynamicsAcceleration},
		{name: "liquid_memory", symbol: learning.SymbolDynamicsMemory},
		{name: "memory_scale", symbol: learning.SymbolDynamicsMemoryScale},
		{name: "stored_energy", symbol: learning.SymbolDynamicsStoredEnergy},
		{name: "passivity_residue", symbol: learning.SymbolDynamicsPassivityResidue},
		{name: "continuous_variance", symbol: learning.SymbolDynamicsContinuousVariance},
		{name: "jump_amplitude", symbol: learning.SymbolDynamicsJumpAmplitude},
		{name: "jump_variance", symbol: learning.SymbolDynamicsJumpVariance},
		{name: "equivariance_norm", symbol: learning.SymbolDynamicsEquivarianceNorm},
	}

	for _, field := range fields {
		value, found := dynamics.Get(field.symbol)

		if !found {
			continue
		}

		graph.AddNode(&types.Node{
			ID:         fmt.Sprintf("res:%s:%s", symbol, field.name),
			Symbol:     symbol,
			Source:     "resonance_dynamics",
			Kind:       types.KindResonance,
			Value:      value,
			Strength:   math.Abs(value),
			Confidence: confidence,
			At:         graph.At,
			Metadata: map[string]any{
				"continuous_time": true,
				"frame_symbol":    field.name,
			},
		})
	}
}

func directionPosteriorConfidence(
	forecast learning.RLSOutput,
	call float64,
) float64 {
	if !forecast.Ready || forecast.Scale <= 0 || forecast.DegreesOfFreedom <= 0 ||
		math.IsNaN(forecast.Value) || math.IsInf(forecast.Value, 0) {
		return 0
	}

	distribution := distuv.StudentsT{
		Mu:    forecast.Value,
		Sigma: forecast.Scale,
		Nu:    forecast.DegreesOfFreedom,
	}
	positive := 1 - distribution.CDF(0)

	if call > 0 {
		return min(max(positive, 0), 1)
	}

	if call < 0 {
		return min(max(1-positive, 0), 1)
	}

	return 0
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
) (*types.Node, bool, error) {
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

	return &types.Node{
		ID:         fmt.Sprintf("causal:%s:%s", symbol, field.value),
		Symbol:     symbol,
		Source:     "causal",
		Kind:       types.KindCausal,
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
	symbol *types.Symbol, graph *types.Graph,
) error {
	var causalMap map[string]any

	for stored := range symbol.MarketCausal(types.SourceGraph) {
		causalMap = stored
	}

	if !causalValuesPresent(causalMap) {
		return nil
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
	symbol *types.Symbol,
	cognition types.Cognition,
	graph *types.Graph,
) {
	if cognition.Winner == "" {
		return
	}

	nodeID := fmt.Sprintf("cog:%s:winner_regime", symbol.Symbol)
	graph.AddNode(&types.Node{
		ID:         nodeID,
		Symbol:     symbol.Symbol,
		Source:     cognition.Source,
		Kind:       types.KindCognition,
		Value:      cognition.Confidence,
		Confidence: cognition.Confidence,
		At:         cognition.At,
		Metadata: map[string]any{
			"regime":           cognition.Winner,
			"candidate":        cognition.CandidateWinner,
			"sequence":         cognition.Sequence,
			"held":             cognition.StateHeld,
			"switchConfidence": cognition.SwitchConfidence,
			"switchThreshold":  cognition.SwitchThreshold,
		},
	})

	if cognition.PredictionsHeld {
		return
	}

	for path, probability := range cognition.Predictions {
		if path == "" || probability <= 0 {
			continue
		}

		graph.AddNode(&types.Node{
			ID:         fmt.Sprintf("cog:%s:prediction:%s", symbol.Symbol, path),
			Symbol:     symbol.Symbol,
			Source:     cognition.Source,
			Kind:       types.KindPrediction,
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
	symbol *types.Symbol,
	categories []types.Category,
	cognition types.Cognition,
	graph *types.Graph,
) error {
	nodes := graph.Nodes
	resonanceBySymbol := make(map[string][]*types.Node)
	causalBySymbol := make(map[string][]*types.Node)
	interventions := make([]*types.Node, 0)
	expectationsBySymbol := make(map[string][]*types.Node)

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
					graph.AddEdge(&types.Edge{
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
					graph.AddEdge(&types.Edge{
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

			graph.AddEdge(&types.Edge{
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
	if cognition.Winner != "" && !cognition.PredictionsHeld {
		currentNodeID := fmt.Sprintf("cog:%s:winner_regime", symbol.Symbol)

		for path, probability := range cognition.Predictions {
			if path == "" || probability <= 0 {
				continue
			}

			targetNodeID := fmt.Sprintf("cog:%s:prediction:%s", symbol.Symbol, path)
			graph.AddEdge(&types.Edge{
				From:       currentNodeID,
				To:         targetNodeID,
				Relation:   RelationLeads,
				Weight:     probability,
				Confidence: probability,
				Evidence:   []string{currentNodeID, targetNodeID},
				At:         graph.At,
				Reason:     "cognition beam search lookahead prediction",
			})
			graph.AddEdge(&types.Edge{
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

			graph.AddEdge(&types.Edge{
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
connectLongOpportunity gives every analytical module one explicit proposition
to address: whether current conditioned evidence supports risking capital on a
long position in this symbol. The graph remains an interpretation mechanism;
no relation here is converted into a price forecast.
*/
func (solver *Solver) connectLongOpportunity(
	symbol *types.Symbol,
	graph *Graph,
) error {
	if symbol == nil || graph == nil {
		return nil
	}

	target := fmt.Sprintf("hyp:%s:long_opportunity", symbol.Symbol)
	graph.SetDecisionTarget(target)
	graph.AddNode(&types.Node{
		ID:         target,
		Symbol:     symbol.Symbol,
		Source:     "strategy",
		Kind:       KindHypothesis,
		Confidence: 1,
		At:         graph.At,
		Metadata: map[string]any{
			"question": "does the conditioned evidence support risking capital on a long position now?",
		},
	})

	for _, node := range graph.Nodes {
		if node == nil || node.ID == target || node.Confidence <= 0 {
			continue
		}

		relation, reason := opportunityRelation(node)

		if relation == "" {
			continue
		}

		weight, err := nodeEvidenceWeight(node)

		if err != nil {
			return fmt.Errorf("opportunity weight for %s: %w", node.ID, err)
		}

		if weight <= 0 {
			continue
		}

		confidence := min(max(node.Confidence, 0), 1)
		graph.AddEdge(&types.Edge{
			From:       node.ID,
			To:         target,
			Relation:   relation,
			Weight:     weight,
			Confidence: confidence,
			Evidence:   []string{node.ID, target},
			At:         graph.At,
			Reason:     reason,
		})
	}

	return nil
}

func opportunityRelation(node *types.Node) (types.RelationType, string) {
	switch node.Kind {
	case KindMeasurement:
		return measurementOpportunityRelation(node), "measurement addresses the long-opportunity thesis on its own evidence"
	case KindCategory:
		category, _ := node.Metadata["type"].(string)
		relation := categoryOpportunityRelation(types.CategoryType(category))
		return relation, "category " + category + " addresses the long-opportunity thesis"
	case types.KindResonance:
		if strings.HasSuffix(node.ID, ":forecast") {
			return signedOpportunityRelation(node.Value),
				"predictive coding contributes a direction opinion"
		}

		return RelationConditions, "predictive-coding surprise conditions confidence"
	case types.KindCausal:
		if strings.HasSuffix(node.ID, ":doExpectation") ||
			strings.HasSuffix(node.ID, ":uplift") {
			return signedOpportunityRelation(node.Value),
				"Pearl intervention evidence addresses the direction of the thesis"
		}

		return RelationConditions, "causal ladder context conditions the thesis"
	case types.KindManifold:
		if node.ID == "man:universe:phase_direction" {
			return signedOpportunityRelation(node.Value),
				"phase-geodesic consensus addresses universe direction"
		}

		return RelationConditions, "relaxed physical field conditions the thesis"
	case types.KindCognition:
		regime, _ := node.Metadata["regime"].(string)
		return categoryOpportunityRelation(categoryFromText(regime)),
			"cognition regime addresses thesis persistence"
	case types.KindPrediction:
		path, _ := node.Metadata["path"].(string)
		return categoryOpportunityRelation(categoryFromText(path)),
			"cognition lookahead addresses the next structural regime"
	default:
		return "", ""
	}
}

func signedOpportunityRelation(value float64) RelationType {
	switch {
	case value > 0:
		return RelationSupports
	case value < 0:
		return RelationContradicts
	default:
		return RelationConditions
	}
}

func categoryFromText(value string) types.CategoryType {
	if value == "" {
		return types.CategoryTypeNone
	}

	for _, category := range types.CategoryOrder {
		if value == string(category) || strings.Contains(value, string(category)) {
			return category
		}
	}

	return types.CategoryTypeNone
}

/*
measurementOpportunityRelation states how one raw measurement addresses the
long-opportunity thesis on its own signed semantics. Aggressor-side alignment is
the voice: buy-side pressure supports, sell-side pressure contradicts. Metrics
whose semantic group is explicitly contextual or uninformative stay conditions.
The verdict is independent of what a category classifier later claims about the
same measurement; the category path is additional enrichment.
*/
func measurementOpportunityRelation(node *types.Node) types.RelationType {
	groups, known := types.SignalMetricGroups[types.SourceType(node.Source)]

	if !known {
		return types.RelationConditions
	}

	membership, known := groups[types.MetricKey(node.Metric, node.Side)]

	if !known || !membership.Competes {
		return types.RelationConditions
	}

	switch node.Side {
	case types.SideBuy, types.SideBuyToBuy, types.SideBuyToSell:
		return signedOpportunityRelation(node.Value)
	case types.SideSell, types.SideSellToSell, types.SideSellToBuy:
		relation := signedOpportunityRelation(node.Value)

		if relation == types.RelationSupports {
			return types.RelationContradicts
		}

		if relation == types.RelationContradicts {
			return types.RelationSupports
		}

		return types.RelationConditions
	default:
		return signedOpportunityRelation(node.Value)
	}
}

/*
categoryOpportunityRelation states category semantics for a long-only account.
Directional precursor and authenticity states support; manipulation, decay, and
adverse systemic states contradict; states whose direction depends on another
module remain conditions rather than being guessed bullish or bearish.
*/
func categoryOpportunityRelation(category types.CategoryType) RelationType {
	switch category {
	case types.VerticalIgnition,
		types.CoiledCompression,
		types.OrganicTrend,
		types.AggressiveDrive,
		types.LoadedImbalance,
		types.InefficientLag,
		types.DecoupledMove,
		types.RiskOnSurge,
		types.DivergentMove,
		types.DecoupledAlpha,
		types.EndogenousAlpha,
		types.HardSupport,
		types.Laminar,
		types.Inertial,
		types.Frenzy,
		types.Organic:
		return RelationSupports
	case types.SpoofTrap,
		types.BookThinning,
		types.AnchorStall,
		types.FadedExhaustion,
		types.SystemicSlump,
		types.ToxicBluff,
		types.SystemicBeta,
		types.CausalNoise,
		types.MechanicalCollapse,
		types.ThermalExhaustion,
		types.FragileExpansion,
		types.ActiveReversal,
		types.VolumeStarvation,
		types.StochasticNoise,
		types.DivergentStress,
		types.Turbulent,
		types.Saturation,
		types.Exhaustion:
		return RelationContradicts
	case types.CategoryTypeNone:
		return ""
	default:
		return RelationConditions
	}
}

func nodeEvidenceWeight(node *Node) (float64, error) {
	if node == nil {
		return 0, nil
	}

	strength := node.Strength

	if !(strength > 0) {
		strength = math.Abs(node.Value)
	}

	return magnitudeWeight(strength)
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
	solver.cancel()
	return nil
}
