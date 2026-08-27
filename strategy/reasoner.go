package strategy

import (
	"fmt"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
CausalState is the typed handoff from the reasoner to strategic search: the
current observational market state, the fitted causal transition models for
every schema market variable (the time-sliced system the rollout evolves),
identification status, and the relevant Influence Graph edges. It is
published on ChannelCausalState.
*/
type CausalState struct {
	Symbol          string
	At              time.Time
	Epoch           uint64
	SchemaVersion   uint64
	ModelVersion    string
	Identification  causal.IdentificationStatus
	// MarketState is the temporal market state: current values plus the
	// timestamped trajectory the as-of transition evaluation reads.
	MarketState     mcts.MarketState
	OutcomeVariable causal.VariableID
	// Transition is the outcome variable's fitted transition (convenience).
	Transition *causal.TransitionModel
	// Transitions is the fitted transition of every schema market variable;
	// the causal rollout evolves this whole system.
	Transitions map[relation.Coordinate]*causal.TransitionModel
	// StepLag is the causal time step of one rollout transition.
	StepLag        time.Duration
	InfluenceEdges []*graph.InfluenceEdge
}

type reasonerSymbolState struct {
	mu      sync.Mutex
	lastRun time.Time
	model   *causal.CausalModel
}

/*
Reasoner is the live observational reasoning stage. It owns the coordinate
store, the Relation engine, the Influence Graph, and the per-symbol Causal
models, and it publishes CausalState per symbol with symbol-local synchronization.
It is the explicit dataflow:

	ChannelMeasurements
	    → Observational Coordinate Store
	    → Relation Engine
	    → Influence updates
	    → Influence Graph
	    → Causal Model / CausalState

Feature selection is query-local; every valid Measurement coordinate stays in
the store. Simulated states never enter the store.
*/
type Reasoner struct {
	epoch          uint64
	store          *relation.ObservationStore
	estimator      *relation.InfluenceEstimator
	influenceGraph *graph.InfluenceGraph
	schemaTemplate *causal.CausalSchema
	plans          []*relation.RelationPlan
	interval       time.Duration
	symbolStates   sync.Map
	compiledPlans  sync.Map
	onState        atomic.Pointer[func(*CausalState)]
}

/*
NewReasoner builds the reasoning stage. historyCapacity bounds each
coordinate's retained observations (infrastructure provenance). plans are the
explicit Relation plans; schemaTemplate is the symbol-agnostic CausalSchema
instantiated per symbol. interval bounds per-symbol Relation refresh rate. A
nil schemaTemplate is a validation error.
*/
func NewReasoner(
	epoch uint64,
	historyCapacity int,
	plans []*relation.RelationPlan,
	schemaTemplate *causal.CausalSchema,
	interval time.Duration,
) (*Reasoner, error) {
	if schemaTemplate == nil {
		return nil, errStrategy("causal schema template is required")
	}

	if interval <= 0 {
		interval = time.Second
	}

	return &Reasoner{
		epoch:          epoch,
		store:          relation.NewObservationStore(historyCapacity),
		estimator:      relation.NewInfluenceEstimator("prequential-linear-v1"),
		influenceGraph: graph.NewInfluenceGraph(epoch, schemaTemplate.Version, planVersion(plans), 64),
		schemaTemplate: schemaTemplate,
		plans:          plans,
		interval:       interval,
	}, nil
}

/*
SetOnState installs the per-symbol state callback so it can be invoked safely
while measurements are being ingested concurrently.
*/
func (reasoner *Reasoner) SetOnState(callback func(*CausalState)) {
	if reasoner == nil {
		return
	}

	reasoner.onState.Store(&callback)
}

func (reasoner *Reasoner) getSymbolState(symbol string) *reasonerSymbolState {
	if stored, found := reasoner.symbolStates.Load(symbol); found {
		return stored.(*reasonerSymbolState)
	}

	candidate := &reasonerSymbolState{}
	actual, _ := reasoner.symbolStates.LoadOrStore(symbol, candidate)

	return actual.(*reasonerSymbolState)
}

func errStrategy(format string, arguments ...any) error {
	return fmt.Errorf("strategy: "+format, arguments...)
}

func planVersion(plans []*relation.RelationPlan) uint64 {
	version := uint64(0)

	for _, plan := range plans {
		if plan != nil && plan.Version > version {
			version = plan.Version
		}
	}

	return version
}

/*
Ingest appends one Measurement to the coordinate store and refreshes the
Relation estimates for its symbol using symbol-local locking.
*/
func (reasoner *Reasoner) Ingest(measurement *nmtypes.Measurement) {
	if reasoner == nil || measurement == nil || measurement.Err != nil {
		return
	}

	observations := relation.AppendMeasurement(measurement, reasoner.epoch)

	if len(observations) == 0 {
		return
	}

	reasoner.store.AppendObservations(observations)

	symbolState := reasoner.getSymbolState(measurement.Symbol)
	symbolState.mu.Lock()

	due := false
	if !symbolState.lastRun.IsZero() && measurement.At.Sub(symbolState.lastRun) >= reasoner.interval {
		symbolState.lastRun = measurement.At
		due = true
	} else if symbolState.lastRun.IsZero() {
		symbolState.lastRun = measurement.At
		due = true
	}

	var state *CausalState
	onStatePtr := reasoner.onState.Load()

	if due {
		reasoner.updateRelations(measurement.Symbol)

		if onStatePtr != nil && *onStatePtr != nil {
			state = reasoner.snapshotSymbol(symbolState, measurement.Symbol, measurement.At)
		}
	}

	symbolState.mu.Unlock()

	if onStatePtr != nil && *onStatePtr != nil && state != nil {
		(*onStatePtr)(state)
	}
}

/*
Refresh forces one Relation update round for a symbol and republishes its
causal state.
*/
func (reasoner *Reasoner) Refresh(symbol string, at time.Time) {
	if reasoner == nil || symbol == "" {
		return
	}

	symbolState := reasoner.getSymbolState(symbol)
	symbolState.mu.Lock()

	reasoner.updateRelations(symbol)

	var state *CausalState
	onStatePtr := reasoner.onState.Load()

	if onStatePtr != nil && *onStatePtr != nil {
		state = reasoner.snapshotSymbol(symbolState, symbol, at)
	}

	symbolState.mu.Unlock()

	if onStatePtr != nil && *onStatePtr != nil && state != nil {
		(*onStatePtr)(state)
	}
}

type reasonerCompiledPlanEntry struct {
	coordinateCount int
	candidates      []relation.CompiledCandidate
}

/*
updateRelations estimates every planned pair for one symbol and records the
Influence edges using precompiled candidate topology.
*/
func (reasoner *Reasoner) updateRelations(symbol string) {
	coordinateCount := reasoner.store.CoordinateCount()

	var candidates []relation.CompiledCandidate

	if cachedValue, found := reasoner.compiledPlans.Load(symbol); found {
		entry := cachedValue.(reasonerCompiledPlanEntry)

		if entry.coordinateCount == coordinateCount {
			candidates = entry.candidates
		}
	}

	if candidates == nil {
		candidates = relation.CompilePlansForSymbol(reasoner.plans, symbol, reasoner.epoch, reasoner.store)
		reasoner.compiledPlans.Store(symbol, reasonerCompiledPlanEntry{
			coordinateCount: coordinateCount,
			candidates:      candidates,
		})
	}

	for _, candidate := range candidates {
		reasoner.estimateCandidate(candidate)
	}
}

/*
estimateCandidate estimates one precompiled candidate pair and records the Influence edge.
*/
func (reasoner *Reasoner) estimateCandidate(candidate relation.CompiledCandidate) {
	_ = reasoner.influenceGraph.RegisterCandidate(graph.EdgeInfluence, candidate.Source, candidate.Target, reasoner.epoch)

	if !candidate.ControlsComplete {
		_ = reasoner.influenceGraph.SetUnavailable(graph.EdgeInfluence, candidate.Source, candidate.Target, reasoner.epoch)
		return
	}

	result, err := reasoner.estimator.Estimate(reasoner.store, relation.InfluenceRequest{
		Source:   candidate.Source,
		Target:   candidate.Target,
		Controls: candidate.Controls,
		Lag:      candidate.Lag,
	})

	if err != nil {
		return
	}

	if result.Defined() {
		_ = reasoner.influenceGraph.UpsertEdge(&graph.InfluenceEdge{
			Type:   graph.EdgeInfluence,
			Source: candidate.Source,
			Target: candidate.Target,
			Result: result,
			From:   result.From,
			At:     result.At,
			Epoch:  reasoner.epoch,
		})

		return
	}

	_ = reasoner.influenceGraph.SetUnavailable(graph.EdgeInfluence, candidate.Source, candidate.Target, reasoner.epoch)
}

/*
resolveAllSelectors returns every stored coordinate matching one structural
selector for the symbol and epoch.
*/
func resolveAllSelectors(
	selector relation.Selector,
	symbol string,
	peer string,
	epoch uint64,
	coordinates []relation.Coordinate,
) []relation.Coordinate {
	matches := make([]relation.Coordinate, 0)

	for _, coordinate := range coordinates {
		if coordinate.Symbol != symbol || coordinate.Epoch != epoch {
			continue
		}

		if peer != "" && coordinate.Peer != peer {
			continue
		}

		if !selector.Matches(coordinate) {
			continue
		}

		matches = append(matches, coordinate)
	}

	return matches
}

/*
Symbols returns every symbol with retained observational data, sorted.
*/
func (reasoner *Reasoner) Symbols() []string {
	if reasoner == nil {
		return nil
	}

	seen := make(map[string]bool)

	reasoner.store.RangeCoordinates(func(coordinate relation.Coordinate) bool {
		seen[coordinate.Symbol] = true
		return true
	})

	symbols := make([]string, 0, len(seen))

	for symbol := range seen {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	return symbols
}

/*
CausalState computes the current causal state for one symbol at or before at.
*/
func (reasoner *Reasoner) CausalState(symbol string, at time.Time) *CausalState {
	if reasoner == nil {
		return nil
	}

	symbolState := reasoner.getSymbolState(symbol)
	symbolState.mu.Lock()
	defer symbolState.mu.Unlock()

	return reasoner.snapshotSymbol(symbolState, symbol, at)
}

/*
snapshotSymbol computes the causal state for a symbol.
*/
func (reasoner *Reasoner) snapshotSymbol(symbolState *reasonerSymbolState, symbol string, at time.Time) *CausalState {
	model := reasoner.ensureModel(symbolState, symbol)

	if model == nil {
		return nil
	}

	schema := model.Schema()

	if len(schema.Outcomes) == 0 {
		return nil
	}

	outcome := schema.Outcomes[0]
	transitions := model.TransitionModels(at)
	outcomeTransition := transitions[outcome.Coordinate]
	market := reasoner.buildMarketState(symbol, at, model)
	edges := reasoner.symbolInfluenceEdges(symbol)

	identification := outcomeTransition.Status

	if outcomeTransition.Status == causal.IdentificationIdentified {
		for _, coordinate := range reasoner.sortedPresentCoordinates(market) {
			transition := transitions[coordinate]

			if transition == nil || transition.Status != causal.IdentificationIdentified {
				status := causal.IdentificationUndefined

				if transition != nil {
					status = transition.Status
				}

				identification = status
				break
			}
		}
	}

	stepLag := time.Second

	if outcomeTransition != nil && outcomeTransition.SelfLag > 0 {
		stepLag = outcomeTransition.SelfLag
	}

	return &CausalState{
		Symbol:          symbol,
		At:              at,
		Epoch:           reasoner.epoch,
		SchemaVersion:   schema.Version,
		ModelVersion:    model.ModelVersion(),
		Identification:  identification,
		MarketState:     market,
		OutcomeVariable: outcome,
		Transition:      outcomeTransition,
		Transitions:     transitions,
		StepLag:         stepLag,
		InfluenceEdges:  append([]*graph.InfluenceEdge(nil), edges...),
	}
}

/*
buildMarketState materializes the temporal market state for one symbol.
*/
func (reasoner *Reasoner) buildMarketState(
	symbol string,
	at time.Time,
	model *causal.CausalModel,
) mcts.MarketState {
	state := mcts.MarketState{
		At:      at,
		Current: make(map[relation.Coordinate]float64),
		History: make(map[relation.Coordinate][]mcts.MarketSample),
	}

	schema := model.Schema()

	for _, marketVariable := range schema.MarketVariables {
		coordinate := marketVariable.Variable.Coordinate

		if coordinate.Symbol != symbol {
			continue
		}

		history := make([]mcts.MarketSample, 0)

		reasoner.store.RangeHistory(coordinate, func(observation relation.Observation) bool {
			if observation.At.After(at) {
				return true
			}

			history = append(history, mcts.MarketSample{
				At:    observation.At,
				Value: observation.Raw,
			})

			return true
		})

		if len(history) > 0 {
			state.History[coordinate] = history
			state.Current[coordinate] = history[len(history)-1].Value
		}
	}

	return state
}

/*
sortedPresentCoordinates returns the coordinates present in the market state
in deterministic order for stable identification gating.
*/
func (reasoner *Reasoner) sortedPresentCoordinates(state mcts.MarketState) []relation.Coordinate {
	coordinates := make([]relation.Coordinate, 0, len(state.Current))

	for coordinate := range state.Current {
		coordinates = append(coordinates, coordinate)
	}

	slices.SortFunc(coordinates, relation.CompareCoordinate)

	return coordinates
}

/*
symbolInfluenceEdges returns every current Influence edge for one symbol,
spanning all four tiers of the mediated DAG rather than only the inbound
edges to the outcome. The audit trace and the frontend graph need the full
cascade, not the single-sink fan into price return.
*/
func (reasoner *Reasoner) symbolInfluenceEdges(symbol string) []*graph.InfluenceEdge {
	if reasoner == nil || reasoner.influenceGraph == nil {
		return nil
	}

	edges := reasoner.influenceGraph.Edges()
	filtered := make([]*graph.InfluenceEdge, 0, len(edges))

	for _, edge := range edges {
		if edge.Type == graph.EdgeInfluence && edge.Source.Symbol == symbol {
			filtered = append(filtered, edge)
		}
	}

	return filtered
}

/*
modelFor returns the per-symbol causal model, instantiating it lazily from
the schema template.
*/
func (reasoner *Reasoner) modelFor(symbol string) *causal.CausalModel {
	symbolState := reasoner.getSymbolState(symbol)
	symbolState.mu.Lock()
	defer symbolState.mu.Unlock()

	return reasoner.ensureModel(symbolState, symbol)
}

func (reasoner *Reasoner) ensureModel(symbolState *reasonerSymbolState, symbol string) *causal.CausalModel {
	if symbolState.model != nil {
		return symbolState.model
	}

	schema := reasoner.schemaTemplate.ForSymbol(symbol)
	model := causal.NewCausalModel(schema, reasoner.store, reasoner.influenceGraph, "causal-linear-v1")
	symbolState.model = model

	return model
}

/*
Store exposes the observational coordinate store.
*/
func (reasoner *Reasoner) Store() *relation.ObservationStore {
	if reasoner == nil {
		return nil
	}

	return reasoner.store
}

/*
Graph exposes the Influence Graph.
*/
func (reasoner *Reasoner) Graph() *graph.InfluenceGraph {
	if reasoner == nil {
		return nil
	}

	return reasoner.influenceGraph
}

/*
Epoch returns the reasoning model epoch.
*/
func (reasoner *Reasoner) Epoch() uint64 {
	if reasoner == nil {
		return 0
	}

	return reasoner.epoch
}
