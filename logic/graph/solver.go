package graph

import (
	"context"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
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
	ui           *runtime.Channel[*types.UIFrame]
	graphs       *runtime.Channel[*types.Graph]
	evidence     sync.Map
	phase        types.PhaseReading
	building     sync.Map
}

/*
NewSolver creates a graph solver.
*/
func NewSolver(thesis *types.Thesis, recorder *audit.Recorder, bus *runtime.Workspace) *Solver {
	ctx, cancel := context.WithCancel(context.Background())

	solver := &Solver{
		ctx:          ctx,
		cancel:       cancel,
		thesis:       thesis,
		recorder:     recorder,
		measurements: newMeasurementCompiler(),
		building:     sync.Map{},
	}

	solver.ui = runtime.ChannelOf[*types.UIFrame](
		bus, types.ChannelUI,
		func(frame *types.UIFrame) string { return "" },
	)
	solver.graphs = runtime.ChannelOf[*types.Graph](
		bus, types.ChannelGraphs,
		func(graph *types.Graph) string { return graph.Symbol },
	)
	measurementsCh := runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	measurementsCh.Subscribe(solver.Name(), func(measurement *nmtypes.Measurement) error {
		return solver.observe(measurement.Symbol, "measurement", measurement)
	})
	runtime.ChannelOf[[]types.Category](
		bus, types.ChannelCategories,
		func(batch []types.Category) string {
			if len(batch) == 0 {
				return ""
			}
			return batch[0].Symbol
		},
	).Subscribe(solver.Name(), func(batch []types.Category) error {
		if len(batch) == 0 {
			return nil
		}
		return solver.observe(batch[0].Symbol, "categories", batch)
	})
	runtime.ChannelOf[types.ResonanceArtifact](
		bus, types.ChannelResonance,
		func(artifact types.ResonanceArtifact) string { return artifact.Symbol },
	).Subscribe(solver.Name(), func(artifact types.ResonanceArtifact) error {
		return solver.observe(artifact.Symbol, "resonance", artifact)
	})
	runtime.ChannelOf[types.CausalOutput](
		bus, types.ChannelCausal,
		func(output types.CausalOutput) string { return output.Symbol },
	).Subscribe(solver.Name(), func(output types.CausalOutput) error {
		return solver.observe(output.Symbol, "causal", output)
	})
	runtime.ChannelOf[types.Cognition](
		bus, types.ChannelCognition,
		func(reading types.Cognition) string { return reading.Symbol },
	).Subscribe(solver.Name(), func(reading types.Cognition) error {
		return solver.observe(reading.Symbol, "cognition", reading)
	})
	runtime.ChannelOf[types.PhaseReading](
		bus, types.ChannelPhase,
		func(reading types.PhaseReading) string { return "" },
	).Subscribe(solver.Name(), func(reading types.PhaseReading) error {
		solver.phase = reading
		return nil
	})

	return solver
}

func (solver *Solver) Name() string {
	return "graph"
}

func (solver *Solver) Error() error { return solver.err }

/*
consume rebuilds the graph whenever one of its upstream evidence cursors becomes
ready. Graph output flows one way to the planner.
*/
/*
buildGraph adds current upstream evidence to one in-progress graph. It
publishes that graph once it is informed enough for planner search, then starts
a fresh accumulation for the symbol. A later ready graph replaces any
unpublished planner artifact so measurements keep draining while search is
busy. The in-progress registry is a sync.Map, so parallel symbol workers can
load-or-store their own building graph while evidence compilation and
publication run outside any lock.
*/
/*
symbolEvidence is one symbol's freshest graph inputs: a bounded measurement
window and the latest derived readings from every upstream stage.
*/
type symbolEvidence struct {
	measurements []*nmtypes.Measurement
	categories   []types.Category
	artifact     types.ResonanceArtifact
	causal       types.CausalOutput
	cognition    types.Cognition
}

func iterSeq[T any](values []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, value := range values {
			if !yield(value) {
				return
			}
		}
	}
}

// observe folds one evidence item into the symbol's evidence snapshot and
// rebuilds the symbol graph from the freshest state, publishing it when it
// is informed enough for planner search.
func (solver *Solver) observe(symbolName string, kind string, value any) error {
	if symbolName == "" {
		return nil
	}

	state := solver.symbolEvidence(symbolName)

	switch kind {
	case "measurement":
		if measurement, ok := value.(*nmtypes.Measurement); ok {
			state.measurements = append(state.measurements, measurement)
			state.measurements = trimTail(state.measurements, graphMeasurementWindow)
		}
	case "categories":
		if batch, ok := value.([]types.Category); ok {
			state.categories = batch
		}
	case "resonance":
		if artifact, ok := value.(types.ResonanceArtifact); ok {
			state.artifact = artifact
		}
	case "causal":
		if output, ok := value.(types.CausalOutput); ok {
			state.causal = output
		}
	case "cognition":
		if reading, ok := value.(types.Cognition); ok {
			state.cognition = reading
		}
	}

	return solver.rebuild(symbolName, state)
}

func (solver *Solver) symbolEvidence(symbolName string) *symbolEvidence {
	loaded, _ := solver.evidence.LoadOrStore(symbolName, &symbolEvidence{})

	return loaded.(*symbolEvidence)
}

/*
graphMeasurementWindow bounds the per-symbol measurement history retained for
graph compilation, so a hot symbol cannot grow unbounded evidence state.
*/
const graphMeasurementWindow = 64

func trimTail[T any](values []T, limit int) []T {
	if len(values) <= limit {
		return values
	}

	return append([]T(nil), values[len(values)-limit:]...)
}

func (solver *Solver) rebuild(symbolName string, state *symbolEvidence) error {
	symbol := solver.thesis.Symbol(symbolName)
	graph := types.NewGraph(solver.thesis.At)
	graph.Symbol = symbolName
	graph.Prune(solver.thesis.At)

	measurementIndex, err := solver.measurements.addNodes(
		symbolName,
		iterSeq(state.measurements),
		graph,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"graph: failed to extract measurement nodes - "+err.Error(),
			err,
		))
	}

	solver.extractCategoryNodes(symbol, state.categories, graph)
	solver.extractManifoldNodes(solver.thesis, graph)

	if state.artifact.Manifold != nil {
		solver.extractResonanceNodes(symbol, state.artifact, graph)
	}

	if state.causal.Rows != nil {
		if err := solver.extractCausalNodes(symbol, state.causal.Rows, graph); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"graph: failed to extract causal nodes - "+err.Error(),
				err,
			))
		}
	}

	solver.extractCognitionNodes(symbol, state.cognition, graph)

	if len(graph.Nodes) == 0 {
		return nil
	}

	if err := solver.measurements.addCategoryEdges(
		state.categories, symbolName, graph, measurementIndex,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"graph: failed to relate measurements and categories - "+err.Error(),
			err,
		))
	}

	if err := solver.measurements.addLeadLagEdges(
		symbol, graph, measurementIndex,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"graph: failed to relate lead-lag measurements - "+err.Error(),
			err,
		))
	}

	if err := solver.inferStructuralEdges(
		symbol, state.categories, state.cognition, graph,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph: failed to infer structural edges - "+err.Error(),
			err,
		))
	}

	if err := solver.connectLongOpportunity(symbol, graph); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph: failed to connect long-opportunity hypothesis - "+err.Error(),
			err,
		))
	}

	if symbolName == types.Focus() && solver.ui != nil {
		solver.ui.Publish(&types.UIFrame{
			Type:  wire.FrameGraphFrame,
			Value: graph.Wire(),
		})
	}

	if !graph.ReadyForSearch() {
		return nil
	}

	if solver.graphs != nil {
		solver.graphs.Publish(graph.Clone())
	}

	return nil
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

		confidence := cat.Confidence
		node := &Node{
			ID:       nodeID,
			Symbol:   symbol.Symbol,
			Source:   "category",
			Kind:     KindCategory,
			Value:    cat.Strength,
			Maturity: cat.Maturity,
			At:       graph.At,
			Metadata: metadata,
		}

		if confidence >= 0 && confidence <= 1 {
			node.Normalized = &confidence
		}

		node.Confidence = types.ObservationMass(node, graph.At)
		graph.AddNode(node)
	}
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
			confidence := inference.Confidence
			node := &Node{
				ID:     "man:universe:phase_direction",
				Source: "manifold",
				Kind:   KindManifold,
				Value:  inference.Direction,
				At:     reading.At,
				Metadata: map[string]any{
					"support":       inference.Support,
					"contradiction": inference.Contradiction,
					"balance":       inference.Balance,
					"responses":     inference.Responses,
				},
			}

			if confidence >= 0 && confidence <= 1 {
				node.Normalized = &confidence
			}

			node.Confidence = types.ObservationMass(node, graph.At)
			graph.AddNode(node)
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
		{name: "kuramoto_r", value: reading.KuramotoR},
	}

	for _, field := range fields {
		node := &Node{
			ID:     "man:universe:" + field.name,
			Source: "manifold",
			Kind:   KindManifold,
			Value:  field.value,
			At:     graph.At,
			Metadata: map[string]any{
				"observer": "relaxed physical field",
			},
		}
		node.Confidence = types.ObservationMass(node, graph.At)
		graph.AddNode(node)
	}

	scores, scored := thesis.InterventionSnapshot()

	if !scored {
		return
	}

	for _, score := range scores {
		node := &Node{
			ID:     "man:universe:do_" + score.Action,
			Source: "manifold",
			Kind:   KindManifold,
			Value:  score.Score,
			At:     graph.At,
			Metadata: map[string]any{
				"observer":           "bvp crystallization",
				"mass_gain":          score.MassGain,
				"energy_gain":        score.EnergyGain,
				"heat_shock":         score.HeatShock,
				"spectral_resonance": score.SpectralResonance,
			},
		}
		node.Confidence = types.ObservationMass(node, graph.At)
		graph.AddNode(node)
	}
}

/*
extractResonanceNodes registers predictive coding outcomes (surprise and the
direction call). The call is not a priced return.
*/
func (solver *Solver) extractResonanceNodes(
	symbol *types.Symbol, artifact types.ResonanceArtifact, graph *types.Graph,
) {
	if symbol == nil || graph == nil {
		return
	}

	returnForecast := artifact.Forecast
	coder := artifact.Manifold
	dynamics := artifact.Dynamics

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

func (solver *Solver) extractPredictiveDynamicsNodes(
	symbol string,
	dynamics nmtypes.Frame,
	graph *types.Graph,
) {
	ready, _ := dynamics.Get(learning.SymbolDynamicsReady)
	sampleCount, _ := dynamics.Get(learning.SymbolDynamicsSampleCount)
	maturity := sampleCount / (sampleCount + 1)

	if ready > maturity {
		maturity = ready
	}

	fields := []struct {
		name   string
		symbol nmtypes.Symbol
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

		node := &types.Node{
			ID:       fmt.Sprintf("res:%s:%s", symbol, field.name),
			Symbol:   symbol,
			Source:   "resonance_dynamics",
			Kind:     types.KindResonance,
			Value:    value,
			Strength: math.Abs(value),
			Maturity: maturity,
			At:       graph.At,
			Metadata: map[string]any{
				"continuous_time": true,
				"frame_symbol":    field.name,
			},
		}
		node.Confidence = types.ObservationMass(node, graph.At)
		graph.AddNode(node)
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

	node := &types.Node{
		ID:         fmt.Sprintf("causal:%s:%s", symbol, field.value),
		Symbol:     symbol,
		Source:     "causal",
		Kind:       types.KindCausal,
		Value:      fieldValue,
		Strength:   strength,
		Confidence: probabilities[field.probabilityIndex] * precision,
		Maturity:   precision,
		Metadata: map[string]any{
			"horizon":               1,
			"hypothesis_separation": precision,
		},
	}

	return node, true, nil
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
	if causalMap == nil {
		return false
	}

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
	symbol *types.Symbol, causalMap map[string]any, graph *types.Graph,
) error {
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

	for sym, resonanceNodes := range resonanceBySymbol {
		for _, resonanceNode := range resonanceNodes {
			for _, causalNode := range causalBySymbol[sym] {
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

	// 4. Manifold, Resonance dynamics, and Cognition condition the causal SCM layer (backdoor covariate set Z)
	for _, node := range nodes {
		if node.Kind != KindManifold && node.Kind != KindResonance && node.Kind != KindCognition {
			continue
		}

		if node.ID == "res:"+symbol.Symbol+":forecast" {
			continue
		}

		for _, causalNode := range causalBySymbol[symbol.Symbol] {
			omega := types.ObservationMass(node, graph.At)
			weight := types.NodeInfluence(node)

			if omega <= 0 || weight <= 0 {
				continue
			}

			graph.AddEdge(&types.Edge{
				From:       node.ID,
				To:         causalNode.ID,
				Relation:   RelationConditions,
				Weight:     omega * weight,
				Confidence: omega,
				Evidence:   []string{node.ID, causalNode.ID},
				At:         graph.At,
				Reason:     "field dynamics condition causal intervention context (backdoor covariate)",
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
		if node == nil || node.ID == target {
			continue
		}

		relation, reason := opportunityRelation(node)

		if relation == "" {
			continue
		}

		omega := types.ObservationMass(node, graph.At)
		weight := types.NodeInfluence(node)

		if omega <= 0 || weight <= 0 {
			continue
		}

		graph.AddEdge(&types.Edge{
			From:         node.ID,
			To:           target,
			Relation:     relation,
			Weight:       omega * weight,
			Confidence:   omega,
			Evidence:     []string{node.ID, target},
			ObservedFrom: node.ObservedFrom,
			Horizon:      node.Horizon,
			At:           graph.At,
			Reason:       reason,
		})
	}

	graph.CanonicalizeDecisionEdges()

	return nil
}

func opportunityRelation(node *types.Node) (types.RelationType, string) {
	switch node.Kind {
	case KindCategory:
		category, _ := node.Metadata["type"].(string)
		relation := categoryOpportunityRelation(types.CategoryType(category))
		return relation, "category " + category + " addresses the long-opportunity thesis"
	case types.KindCausal:
		if strings.HasSuffix(node.ID, ":doExpectation") ||
			strings.HasSuffix(node.ID, ":uplift") {
			return signedOpportunityRelation(node.Value),
				"Pearl intervention evidence addresses the direction of the thesis"
		}

		return "", ""
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
