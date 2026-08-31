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
)

/*
CausalState is the typed handoff from the reasoner to strategic search: the
current observational market state, the fitted causal transition models for
every schema market variable (the time-sliced system the rollout evolves),
identification status, and the relevant Influence Graph edges. It is
published on ChannelCausalState.
*/
type CausalState struct {
	Symbol             string
	At                 time.Time
	Epoch              uint64
	SchemaVersion      uint64
	ModelVersion       string
	Identification     causal.IdentificationStatus
	BlockingCoordinate *relation.Coordinate
	BlockingStatus     causal.IdentificationStatus
	BlockingReason     string
	BlockingTransition *causal.TransitionModel
	// MarketState is the temporal market state: current values plus the
	// timestamped trajectory the as-of transition evaluation reads.
	MarketState     mcts.MarketState
	OutcomeVariable causal.VariableID
	// Transition is the outcome variable's fitted transition (convenience).
	Transition *causal.TransitionModel
	// Transitions is the fitted transition of every schema market variable;
	// the causal rollout evolves this whole system.
	Transitions map[relation.Coordinate]*causal.TransitionModel
	// ActiveClosure is the query-local dependency closure of the outcome.
	ActiveClosure []relation.Coordinate
	// StepLag is the causal time step of one rollout transition.
	StepLag        time.Duration
	InfluenceEdges []*graph.InfluenceEdge
}

type reasonerSymbolState struct {
	mu    sync.Mutex
	model *causal.CausalModel
}

/*
Reasoner is the live causal-reasoning stage. It does NOT estimate Relations:
that single responsibility belongs to the graph stage, which owns the shared
ObservationStore and InfluenceGraph (graph.SharedObservationStore /
graph.SharedInfluenceGraph) and refreshes them on its own cadence. The reasoner
is a pure consumer — it reads those shared objects and derives the per-symbol
CausalModel and CausalState on top of the already-fitted influence graph. The
dataflow is:

	ChannelRelations (graph.GraphUpdate)
	    → read shared ObservationStore + InfluenceGraph
	    → per-symbol Causal Model
	    → CausalState

Feature selection is query-local; every valid Measurement coordinate stays in
the shared store. Simulated states never enter the store.
*/
type Reasoner struct {
	epoch          uint64
	store          *relation.ObservationStore
	influenceGraph *graph.InfluenceGraph
	schemaTemplate *causal.CausalSchema
	symbolStates   sync.Map
	onState        atomic.Pointer[func(*CausalState)]
}

/*
NewReasoner builds the reasoning stage. store and influenceGraph are the shared
authoritative instances owned by the graph stage; the reasoner reads them and
never writes a relation estimate. schemaTemplate is the symbol-agnostic
CausalSchema instantiated per symbol. A nil schemaTemplate, store, or graph is a
validation error: the reasoner cannot reason without the fitted influence data.
*/
func NewReasoner(
	epoch uint64,
	store *relation.ObservationStore,
	influenceGraph *graph.InfluenceGraph,
	schemaTemplate *causal.CausalSchema,
) (*Reasoner, error) {
	if schemaTemplate == nil {
		return nil, errStrategy("causal schema template is required")
	}

	if store == nil {
		return nil, errStrategy("shared observation store is required")
	}

	if influenceGraph == nil {
		return nil, errStrategy("shared influence graph is required")
	}

	return &Reasoner{
		epoch:          epoch,
		store:          store,
		influenceGraph: influenceGraph,
		schemaTemplate: schemaTemplate,
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

/*
Ingest appends one Measurement to the coordinate store and refreshes the
Relation estimates for its symbol using symbol-local locking.
*/
/*
OnGraphUpdate consumes one graph relation refresh for a symbol and republishes
its causal state over the freshly estimated influence graph. It never writes a
relation: it reads the shared store and graph that the graph stage just advanced
and derives the CausalState for the affected symbol. The symbol-local lock
serializes concurrent updates so a slow causal snapshot can never interleave a
later one for the same symbol.
*/
func (reasoner *Reasoner) OnGraphUpdate(symbol string, at time.Time) {
	if reasoner == nil || symbol == "" {
		return
	}

	symbolState := reasoner.getSymbolState(symbol)
	symbolState.mu.Lock()

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

	// A symbol that has not printed a trade yet has no cvd/midpoint_log_return
	// history, which would otherwise brick planner evaluation for the pair.
	// Fall back to the quote-driven liquidity midpoint outcome (tick-fed, so
	// present for every subscribed symbol) until the executed-flow outcome
	// exists. The fallback is the same variable identity the schema declared,
	// so the rollout market state and transition stay consistent.
	if outcomeTransition == nil || outcomeTransition.ObservationCount == 0 {
		if fallback := quoteSurfaceOutcome(schema); fallback.Coordinate != outcome.Coordinate {
			if fallbackTransition, found := transitions[fallback.Coordinate]; found &&
				fallbackTransition != nil && fallbackTransition.ObservationCount > 0 {
				outcome = fallback
				outcomeTransition = fallbackTransition
			}
		}
	}

	market := reasoner.buildMarketState(symbol, at, model)
	edges := reasoner.symbolInfluenceEdges(symbol)

	activeClosure, identification, blockingCoordinate, blockingStatus, blockingReason, blockingTransition := activeCausalClosure(
		outcome.Coordinate,
		transitions,
	)

	stepLag := time.Second

	if outcomeTransition != nil && outcomeTransition.SelfLag > 0 {
		stepLag = outcomeTransition.SelfLag
	}

	return &CausalState{
		Symbol:             symbol,
		At:                 at,
		Epoch:              reasoner.epoch,
		SchemaVersion:      schema.Version,
		ModelVersion:       model.ModelVersion(),
		Identification:     identification,
		BlockingCoordinate: blockingCoordinate,
		BlockingStatus:     blockingStatus,
		BlockingReason:     blockingReason,
		BlockingTransition: blockingTransition,
		MarketState:        market,
		OutcomeVariable:    outcome,
		Transition:         outcomeTransition,
		Transitions:        transitions,
		ActiveClosure:      activeClosure,
		StepLag:            stepLag,
		InfluenceEdges:     append([]*graph.InfluenceEdge(nil), edges...),
	}
}

/*
quoteSurfaceOutcome returns the quote-driven liquidity midpoint variable from
the bound schema, or the zero variable when the schema does not declare it.
It is the fallback outcome for a symbol whose executed-flow outcome has no
history yet.
*/
func quoteSurfaceOutcome(schema *causal.CausalSchema) causal.VariableID {
	if schema == nil {
		return causal.VariableID{}
	}

	for _, marketVariable := range schema.MarketVariables {
		coordinate := marketVariable.Variable.Coordinate

		if coordinate.Source == "liquidity" && coordinate.Metric == "midpoint" {
			return marketVariable.Variable
		}
	}

	return causal.VariableID{}
}

/*
activeCausalClosure finds the query-local dependency closure for the requested
outcome: starting at the outcome, it recursively walks only active fitted parents
(transitions with defined Relations). If any transition in this closure is
unidentified, it records the blocking coordinate and status. Unrelated coordinates
outside the closure never veto the query.
*/
func activeCausalClosure(
	outcome relation.Coordinate,
	transitions map[relation.Coordinate]*causal.TransitionModel,
) ([]relation.Coordinate, causal.IdentificationStatus, *relation.Coordinate, causal.IdentificationStatus, string, *causal.TransitionModel) {
	outcomeTransition := transitions[outcome]

	if outcomeTransition == nil || outcomeTransition.ObservationCount == 0 {
		// No trade (or quote) history has formed for this outcome yet: report
		// insufficient support instead of a hard undefined gate, so the pair
		// stays un-evaluated without being permanently bricked.
		blockingCoordinate := outcome
		return []relation.Coordinate{outcome}, causal.IdentificationInsufficientSupport, &blockingCoordinate, causal.IdentificationInsufficientSupport, "awaiting first trade/quote history", nil
	}

	if outcomeTransition.Status != causal.IdentificationIdentified {
		blockingCoordinate := outcome
		return []relation.Coordinate{outcome}, outcomeTransition.Status, &blockingCoordinate, outcomeTransition.Status, outcomeTransition.Reason, outcomeTransition
	}

	closure := make([]relation.Coordinate, 0, len(transitions))
	visited := make(map[relation.Coordinate]bool, len(transitions))

	visited[outcome] = true
	closure = append(closure, outcome)
	queue := []relation.Coordinate{outcome}

	var blockingCoordinate *relation.Coordinate
	var blockingTransition *causal.TransitionModel
	blockingStatus := causal.IdentificationIdentified
	blockingReason := ""

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		transition := transitions[current]

		if transition == nil && blockingCoordinate == nil {
			coordinateCopy := current
			blockingCoordinate = &coordinateCopy
			blockingStatus = causal.IdentificationUndefined
			blockingReason = "required transition in active closure missing"
			continue
		}

		if transition == nil {
			continue
		}

		if transition.Status != causal.IdentificationIdentified && blockingCoordinate == nil {
			coordinateCopy := current
			blockingCoordinate = &coordinateCopy
			blockingStatus = transition.Status
			blockingReason = transition.Reason
			blockingTransition = transition
			continue
		}

		if transition.Status != causal.IdentificationIdentified {
			continue
		}

		for _, parent := range transition.Parents {
			parentCoordinate := parent.Parent.Coordinate

			if !visited[parentCoordinate] {
				visited[parentCoordinate] = true
				closure = append(closure, parentCoordinate)
				queue = append(queue, parentCoordinate)
			}
		}
	}

	if blockingCoordinate != nil {
		return closure, blockingStatus, blockingCoordinate, blockingStatus, blockingReason, blockingTransition
	}

	return closure, causal.IdentificationIdentified, nil, causal.IdentificationIdentified, "", nil
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
