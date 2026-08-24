package strategy

import (
	"fmt"
	"sort"
	"sync"
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
	StepLag         time.Duration
	InfluenceEdges  []*graph.InfluenceEdge
}

/*
Reasoner is the live observational reasoning stage. It owns the coordinate
store, the Relation engine, the Influence Graph, and the per-symbol Causal
models, and it publishes CausalState per symbol. It is the explicit dataflow:

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
	mu sync.Mutex

	epoch          uint64
	store          *relation.ObservationStore
	estimator      *relation.InfluenceEstimator
	influenceGraph *graph.InfluenceGraph
	schemaTemplate *causal.CausalSchema
	plans          []*relation.RelationPlan
	interval       time.Duration
	models         map[string]*causal.CausalModel
	lastRun        map[string]time.Time

	// onState, when set, receives the refreshed CausalState of a symbol
	// after its Relation estimates update.
	onState func(*CausalState)
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
		models:         make(map[string]*causal.CausalModel),
		lastRun:        make(map[string]time.Time),
	}, nil
}

/*
SetOnState installs the per-symbol state callback under the reasoner lock so
it can be assigned safely while measurements are being ingested.
*/
func (reasoner *Reasoner) SetOnState(callback func(*CausalState)) {
	if reasoner == nil {
		return
	}

	reasoner.mu.Lock()
	reasoner.onState = callback
	reasoner.mu.Unlock()
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
Relation estimates for its symbol. It is safe for concurrent use by the bus
worker pool.
*/
func (reasoner *Reasoner) Ingest(measurement *nmtypes.Measurement) {
	if reasoner == nil || measurement == nil || measurement.Err != nil {
		return
	}

	observations := relation.AppendMeasurement(measurement, reasoner.epoch)

	if len(observations) == 0 {
		return
	}

	reasoner.mu.Lock()

	reasoner.store.AppendObservations(observations)

	if reasoner.due(measurement.Symbol, measurement.At) {
		reasoner.updateRelations(measurement.Symbol)
	}

	onState := reasoner.onState

	var state *CausalState

	if onState != nil {
		state = reasoner.snapshotLocked(measurement.Symbol, measurement.At)
	}

	reasoner.mu.Unlock()

	if onState != nil && state != nil {
		onState(state)
	}
}

/*
due reports whether the Relation refresh interval has elapsed for a symbol.
*/
func (reasoner *Reasoner) due(symbol string, at time.Time) bool {
	last, ran := reasoner.lastRun[symbol]

	if !ran || at.Sub(last) >= reasoner.interval {
		reasoner.lastRun[symbol] = at
		return true
	}

	return false
}

/*
Refresh forces one Relation update round for a symbol and republishes its
causal state. It is the explicit online refresh point; Ingest stays cheap by
throttling automatic refreshes to the configured interval.
*/
func (reasoner *Reasoner) Refresh(symbol string, at time.Time) {
	if reasoner == nil || symbol == "" {
		return
	}

	reasoner.mu.Lock()

	reasoner.updateRelations(symbol)

	onState := reasoner.onState

	var state *CausalState

	if onState != nil {
		state = reasoner.snapshotLocked(symbol, at)
	}

	reasoner.mu.Unlock()

	if onState != nil && state != nil {
		onState(state)
	}
}

/*
updateRelations estimates every planned pair for one symbol and records the
Influence edges. Selectors expand to every matching stored coordinate, so a
plan can declare "all configured same-symbol compatible coordinate pairs"
without enumerating combinations. Self-pairs (identical source and target
coordinate) are excluded: Influence requires a positive lag between distinct
coordinates. Unavailable estimates are recorded as unavailable; they are
never deleted and never treated as measured zero.
*/
func (reasoner *Reasoner) updateRelations(symbol string) {
	coordinates := reasoner.store.Coordinates()

	for _, plan := range reasoner.plans {
		if plan == nil || plan.Epoch != reasoner.epoch {
			continue
		}

		for _, pair := range plan.PairsForSymbol(symbol) {
			sources := resolveAllSelectors(pair.Source, symbol, plan.Peer, reasoner.epoch, coordinates)
			targets := resolveAllSelectors(pair.Target, symbol, plan.Peer, reasoner.epoch, coordinates)

			for _, source := range sources {
				for _, target := range targets {
					if source == target {
						continue
					}

					reasoner.estimatePair(plan, symbol, source, target, coordinates)
				}
			}
		}
	}
}

/*
estimatePair estimates one Source→Target pair and records the Influence edge.
The pair is a structurally scheduled candidate first; an undefined estimate
marks that candidate unavailable (never deleted, never a measured zero).
*/
func (reasoner *Reasoner) estimatePair(
	plan *relation.RelationPlan,
	symbol string,
	source relation.Coordinate,
	target relation.Coordinate,
	coordinates []relation.Coordinate,
) {
	_ = reasoner.influenceGraph.RegisterCandidate(graph.EdgeInfluence, source, target, reasoner.epoch)

	controls, controlsComplete := plan.ResolveControls(symbol, coordinates)

	if !controlsComplete {
		_ = reasoner.influenceGraph.SetUnavailable(graph.EdgeInfluence, source, target, reasoner.epoch)
		return
	}

	result, err := reasoner.estimator.Estimate(reasoner.store, relation.InfluenceRequest{
		Source:   source,
		Target:   target,
		Controls: controls,
		Lag:      plan.Lag,
	})

	if err != nil {
		return
	}

	if result.Defined() {
		_ = reasoner.influenceGraph.UpsertEdge(&graph.InfluenceEdge{
			Type:   graph.EdgeInfluence,
			Source: source,
			Target: target,
			Result: result,
			From:   result.From,
			At:     result.At,
			Epoch:  reasoner.epoch,
		})

		return
	}

	_ = reasoner.influenceGraph.SetUnavailable(graph.EdgeInfluence, source, target, reasoner.epoch)
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

	reasoner.mu.Lock()
	defer reasoner.mu.Unlock()

	seen := make(map[string]bool)

	for _, coordinate := range reasoner.store.Coordinates() {
		seen[coordinate.Symbol] = true
	}

	symbols := make([]string, 0, len(seen))

	for symbol := range seen {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	return symbols
}

/*
CausalState computes the current causal state for one symbol at or before at.
It is the snapshot the strategy search consumes.
*/
func (reasoner *Reasoner) CausalState(symbol string, at time.Time) *CausalState {
	if reasoner == nil {
		return nil
	}

	reasoner.mu.Lock()
	defer reasoner.mu.Unlock()

	return reasoner.snapshotLocked(symbol, at)
}

/*
snapshotLocked computes the causal state assuming the reasoner lock is held.
The state's Identification reflects the whole time-sliced system: a market
variable present in the observed state whose transition cannot be estimated
makes the causal evaluation unavailable, because the rollout cannot evolve
it without silently substituting persistence.
*/
func (reasoner *Reasoner) snapshotLocked(symbol string, at time.Time) *CausalState {
	model := reasoner.modelFor(symbol)

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
	edges := reasoner.influenceGraph.Incoming(outcome.Coordinate)

	identification := outcomeTransition.Status

	if outcomeTransition.Status == causal.IdentificationIdentified {
		// A present coordinate whose transition is not identified means the
		// future of the system is genuinely unknown; the evaluation is
		// unavailable rather than a silent persistence carry-forward.
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
buildMarketState materializes the temporal market state for one symbol: the
latest value of each schema market coordinate at or before at, plus the
retained timestamped trajectory the as-of transition evaluation reads.
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

		history := reasoner.store.History(coordinate)

		for _, observation := range history {
			if observation.At.After(at) {
				continue
			}

			state.History[coordinate] = append(state.History[coordinate], mcts.MarketSample{
				At:    observation.At,
				Value: observation.Raw,
			})
		}

		if entries := state.History[coordinate]; len(entries) > 0 {
			state.Current[coordinate] = entries[len(entries)-1].Value
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

	sort.Slice(coordinates, func(left int, right int) bool {
		return coordinates[left].ID() < coordinates[right].ID()
	})

	return coordinates
}

/*
modelFor returns the per-symbol causal model, instantiating it lazily from
the schema template.
*/
func (reasoner *Reasoner) modelFor(symbol string) *causal.CausalModel {
	model, found := reasoner.models[symbol]

	if found {
		return model
	}

	schema := reasoner.schemaTemplate.ForSymbol(symbol)
	model = causal.NewCausalModel(schema, reasoner.store, reasoner.influenceGraph, "causal-linear-v1")
	reasoner.models[symbol] = model

	return model
}

/*
Store exposes the observational coordinate store (used by the conformance
suite to verify information preservation and that simulation never becomes
observation).
*/
func (reasoner *Reasoner) Store() *relation.ObservationStore {
	if reasoner == nil {
		return nil
	}

	return reasoner.store
}

/*
Graph exposes the Influence Graph (used by the conformance suite).
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
