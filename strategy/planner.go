package strategy

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

// StrategyWireBranchCount matches the two ranked branch slots rendered for
// each live candidate decision. Exported so the workspace observer (boot) can
// project the decision round into the dashboard frame.
const StrategyWireBranchCount = 2

/*
Planner is the live strategy decision stage. It consumes observational
Measurements through the Reasoner (coordinate store → Relation → Influence
Graph → Causal model), runs economic MCTS per symbol, and executes the
selected actions through allocation and the broker desk.

The legacy semantic graph (ChannelGraphs), admission policy, opportunity
scores, and predictive-readiness veto play no role in this path. There is
exactly one authoritative live decision path.
*/
type Planner struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	status     types.Status
	recorder   *audit.Recorder
	stager     *audit.Stager
	allocation *Allocation
	desk       *broker.Desk
	reasoner   *Reasoner
	pending    sync.Map
	lastPass   atomic.Int64
	updating   atomic.Bool
	thesis     *types.Thesis

	ObserveModule func(string, time.Duration)
	ObserveHop    func(string, string, time.Duration)
	executeEntry  func(types.Decision) error
	// marketProvider supplies the execution-market inputs for economic
	// evaluation; nil means the broker desk is the provider.
	marketProvider func(string) marketInputs
	// tradingGate reports whether the trading stage may engage. The live
	// default checks the desk's execution prerequisites (instrument/fee
	// surface loaded and live quotes present); tests override it.
	tradingGate func() bool
	// engaged latches the trading decision stage on once the prerequisites
	// are observed filled. Until then the stage stays dormant while the
	// observational layers keep filling underneath.
	engaged atomic.Bool
	// resident holds the evaluated candidate frontier: exactly one evaluated
	// candidate per symbol, replaced when that symbol receives a newer
	// CausalState or when the cheap execution/portfolio signature it was
	// evaluated under stops matching current truth. Passes arbitrate among
	// these already-evaluated candidates; they never rescan the universe.
	resident sync.Map
	// frontier is the resident candidate set kept in deterministic symbol
	// order, maintained incrementally on each evaluation so an arbitration
	// pass never re-sorts the whole universe.
	frontierMu sync.RWMutex
	frontier   []frontierEntry
	// portfolioGen advances when a genuinely global portfolio fact (account
	// cash) changes materially, so global invalidation is an explicit
	// generation rather than a per-ticker poll over every candidate.
	portfolioGen atomic.Uint64
	// evaluate runs the expensive causal/MCTS evaluation for one symbol.
	// The live default is decisionFromCausalState; tests inject a counting
	// wrapper to prove unrelated passes do not rerun evaluations.
	evaluate func(*CausalState, *system.Config, marketInputs) *types.Decision
}

/*
frontierEntry is one symbol's ordered position in the resident entry frontier,
alongside the economic quantities it is ranked by. The frontier is ordered by
incremental economic advantage over waiting (descending), then by search visits
(descending), then by symbol for deterministic replay — never merely lexically.
*/
type frontierEntry struct {
	symbol    string
	advantage float64
	visits    float64
}

/*
residentCandidate is one symbol's evaluated frontier record, together with the
exact state coordinate and the cheap execution/portfolio signature it was
evaluated under. The signature decides cheap current arbitration invalidation:
when the inputs, held quantity, or cash changed since evaluation, the stored
economics are no longer truthful and the candidate is re-evaluated. It is a
state-identity check, never an arbitrary freshness TTL.
*/
type residentCandidate struct {
	state      *CausalState
	stateKey   string
	inputs     marketInputs
	heldQty    float64
	entryPrice float64
	cash       float64
	portGen    uint64
	decision   *types.Decision
}

/*
NewPlanner builds the causal decision stage. It subscribes to
ChannelMeasurements (feeding the reasoner) and ChannelCausalState (the typed
handoff into search), and runs the portfolio pass on the tick clock.
*/
func NewPlanner(
	ctx context.Context,
	thesis *types.Thesis,
	recorder *audit.Recorder,
	desk *broker.Desk,
	store *relation.ObservationStore,
	influenceGraph *graph.InfluenceGraph,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	config := system.Cfg.Snapshot()

	epoch := uint64(1)
	measurementStep := time.Second
	schemaTemplate := DefaultCausalSchema(epoch, measurementStep)

	if config != nil && config.Planner != nil {
		if config.Planner.MeasurementStep > 0 {
			measurementStep = config.Planner.MeasurementStep
			schemaTemplate = DefaultCausalSchema(epoch, measurementStep)
		}
	}

	// The reasoner is a pure consumer of the graph stage's authoritative store
	// and influence graph: the caller passes the same graph.Solver's Store()
	// and Graph() it constructed, so the reasoner reads one fitted state
	// rather than a second, parallel estimator.
	reasoner, reasonerErr := NewReasoner(epoch, store, influenceGraph, schemaTemplate)

	if reasonerErr != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: reasoner construction failed",
			reasonerErr,
		))
		cancel()
		return nil
	}

	planner := &Planner{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		recorder:   recorder,
		stager:     audit.NewStager(recorder),
		allocation: NewAllocation(ctx, desk),
		desk:       desk,
		thesis:     thesis,
		reasoner:   reasoner,
	}

	planner.reasoner.SetOnState(planner.publishCausalState)
	planner.tradingGate = planner.prerequisitesReady

	return planner
}

func (planner *Planner) Name() string { return "planner" }

func (planner *Planner) Error() error { return planner.err }

/*
Reasoner exposes the causal-reasoning stage the planner owns, so the strategy
workload's Step can feed it the graph stage's GraphUpdate off the envelope in
stream order — the same signal → logic → strategy hand-off the rest of the
pipeline uses. The reasoner re-fits its transition models from the shared store
the graph stage already advanced, then republishes the per-symbol CausalState
the planner's search consumes.
*/
func (planner *Planner) Reasoner() *Reasoner {
	return planner.reasoner
}

func (planner *Planner) Stager() *audit.Stager {
	return planner.stager
}

func (planner *Planner) SetMarketProvider(provider func(string) marketInputs) {
	planner.marketProvider = provider
}

func (planner *Planner) SetTradingGate(gate func() bool) {
	planner.tradingGate = gate
}

func (planner *Planner) SetEntryExecutor(executor func(types.Decision) error) {
	planner.executeEntry = executor
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) HasCapacity() bool {
	if planner == nil || planner.desk == nil {
		return true
	}

	return planner.desk.OpenSlots(false) > 0
}

func (planner *Planner) Holding(symbol string) bool {
	if planner == nil || planner.desk == nil || symbol == "" {
		return false
	}

	return planner.desk.Holding(symbol) > 0
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

/*
publishCausalState is the reasoner's state callback: it publishes the typed
CausalState handoff and retains the freshest state per symbol for the next
search round.
*/
func (planner *Planner) publishCausalState(state *CausalState) {
	if planner == nil || state == nil || state.Symbol == "" {
		return
	}

	planner.pending.Store(state.Symbol, state)
}

/*
Step retains the freshest causal state for one symbol.
*/
func (planner *Planner) Step(state *CausalState) error {
	if state == nil || state.Symbol == "" {
		return nil
	}

	planner.pending.Store(state.Symbol, state)

	return nil
}

// StepTick runs one portfolio pass at most once per engine tick and returns the decision round.
func (planner *Planner) StepTick(ticker kraken.TickerData) *types.StrategyRound {
	tick := atomic.LoadInt64(&planner.thesis.Tick)
	last := planner.lastPass.Load()

	if last >= tick {
		return nil
	}

	if !planner.lastPass.CompareAndSwap(last, tick) {
		return nil
	}

	if !planner.updating.CompareAndSwap(false, true) {
		return nil
	}
	defer planner.updating.Store(false)

	return planner.Update(planner.thesis)
}

/*
Update runs one economic MCTS round over every symbol with a fresh causal
state, then allocates and executes the selected actions. Semantic readiness,
admission policy, and predictive readiness do not gate participation: every
symbol with observational state, an explicit schema, and a feasible action
may be considered.
*/
func (planner *Planner) Update(thesis *types.Thesis) *types.StrategyRound {
	// The trading stage stays dormant until boot has filled its execution
	// prerequisites (instrument/fee surface loaded and live market data
	// present). Measurements keep flowing into the reasoner meanwhile, so by
	// the time trading engages the observational history and causal models
	// have warmed up underneath; no decision round runs before that.
	if !planner.engaged.Load() {
		gate := planner.tradingGate

		if gate == nil || !gate() {
			return nil
		}

		planner.engaged.Store(true)
		errnie.Info("planner: trading engaged after execution prerequisites filled")
	}

	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: planner configuration required",
			nil,
		))
		return nil
	}

	decisions := planner.residentDecisions(config)

	plannerStarted := time.Now()
	defer func() {
		if planner.ObserveModule != nil {
			planner.ObserveModule("planner", time.Since(plannerStarted))
		}
	}()

	if len(decisions) == 0 {
		return nil
	}

	actionable := false

	for _, decision := range decisions {
		if decision.Action != types.ActionNothing || decision.Reason != "" {
			actionable = true
			break
		}
	}

	if !actionable {
		for _, decision := range decisions {
			planner.stager.Stage(decision, 10*time.Minute)
		}

		return &types.StrategyRound{
			Evaluated: true,
			Outcome:   "inaction",
			Decisions: decisions,
		}
	}

	// Materialise working shells only for the decisions allocation or
	// execution will actually mutate; the resident evaluated records for
	// every other candidate stay shared and immutable.
	working := workingDecisions(decisions)

	if planner.allocation != nil {
		allocationStarted := time.Now()

		if err := planner.allocation.Calculate(working); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"planner: allocation calculation failed",
				err,
			))
			return nil
		}

		if planner.ObserveModule != nil {
			planner.ObserveModule("allocation", time.Since(allocationStarted))
		}
	}

	if err := planner.executeDecisions(working); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"planner: decision execution failed",
			err,
		))
		return nil
	}

	for _, decision := range working {
		planner.stager.Stage(decision, 10*time.Minute)
	}

	return &types.StrategyRound{
		Evaluated: true,
		Outcome:   "decisions",
		Decisions: working,
	}
}

/*
workingDecisions materialises mutable shells for exactly the decisions
allocation or execution will act on (entry and exit). Every other evaluated
candidate keeps its shared resident reference: a Wait or ActionNothing decision
is read-only on this path and is never copied merely to discover who wins.
*/
func workingDecisions(decisions []*types.Decision) []*types.Decision {
	working := make([]*types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision == nil {
			continue
		}

		if decision.Action == types.ActionEnter || decision.Action == types.ActionExit {
			working = append(working, cloneDecision(decision))
			continue
		}

		working = append(working, decision)
	}

	return working
}

/*
residentDecisions evaluates the dirty frontier once, then materialises the
already-maintained resident frontier in deterministic symbol order for
arbitration. Expensive causal/MCTS evaluation runs only when a symbol holds a
newer CausalState than the evaluated record or when the cheap
execution/portfolio signature changed; an unrelated ticker leaves every other
resident record untouched.

The returned decisions are direct references to the evaluated records: they are
immutable here. Allocation and execution materialise their own working shells
for the few actionable candidates, never the whole universe.
*/
func (planner *Planner) residentDecisions(
	config *system.Config,
) []*types.Decision {
	// Evaluate the arriving states once, in causal arrival order. A dirty
	// symbol is replaced in place; no dirty slice or pending map materialised.
	planner.pending.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		state, ok := value.(*CausalState)

		if !ok || state == nil || isExcludedSymbol(symbol) {
			return true
		}

		stateKey := causalStateKey(state)

		if record, found := planner.resident.Load(symbol); found {
			candidate := record.(*residentCandidate)

			if candidate.stateKey == stateKey && planner.signatureCurrent(symbol, candidate) {
				return true
			}
		}

		planner.evaluateResident(symbol, state, config, planner.evaluate)

		return true
	})

	evaluate := planner.evaluate

	if evaluate == nil {
		evaluate = planner.decisionFromCausalState
	}

	// Second sweep: cheap signature invalidation over the maintained ordered
	// frontier. A candidate whose stored economics no longer match current
	// truth is re-evaluated in place rather than arbitrated with stale
	// economics; nothing is cloned or re-sorted.
	planner.frontierMu.RLock()
	frontier := append([]frontierEntry(nil), planner.frontier...)
	planner.frontierMu.RUnlock()

	decisions := make([]*types.Decision, 0, len(frontier))

	for _, entry := range frontier {
		loaded, found := planner.resident.Load(entry.symbol)

		if !found {
			continue
		}

		candidate, ok := loaded.(*residentCandidate)

		if !ok || candidate == nil || candidate.decision == nil {
			continue
		}

		if !planner.signatureCurrent(entry.symbol, candidate) {
			planner.evaluateResident(entry.symbol, candidate.state, config, evaluate)
			loaded, _ = planner.resident.Load(entry.symbol)
			candidate, _ = loaded.(*residentCandidate)

			if candidate == nil || candidate.decision == nil {
				continue
			}
		}

		decisions = append(decisions, candidate.decision)
	}

	return decisions
}

/*
evaluateResident runs the expensive evaluation for one symbol, replaces its
resident record, and maintains its position in the ordered frontier in place.
It reports through the missing-execution-market decision so an unpriced symbol
stays observable rather than being silently dropped.
*/
func (planner *Planner) evaluateResident(
	symbol string,
	state *CausalState,
	config *system.Config,
	evaluate func(*CausalState, *system.Config, marketInputs) *types.Decision,
) {
	if evaluate == nil {
		evaluate = planner.decisionFromCausalState
	}

	inputs := planner.marketInputsFor(symbol)
	heldQty, entryPrice := planner.heldPosition(symbol)
	cash := inputs.cash

	if !inputs.available && planner.desk != nil {
		if balance := planner.desk.Balance().Cash(); balance != nil {
			cash = balance.Float64()
		}
	}

	record := &residentCandidate{
		state:      state,
		stateKey:   causalStateKey(state),
		inputs:     inputs,
		heldQty:    heldQty,
		entryPrice: entryPrice,
		cash:       cash,
		portGen:    planner.portfolioGen.Load(),
		decision:   evaluate(state, config, inputs),
	}

	// Release the evaluated record only once: it replaces any older resident
	// record for the symbol and updates the ordered frontier in place.
	planner.resident.Store(symbol, record)
	planner.updateFrontier(symbol)
}

/*
updateFrontier inserts or replaces the symbol's slot in the economically
ordered entry frontier without re-sorting the whole set. Only candidates whose
evaluated decision is an entry with a defined enter advantage occupy the
frontier at all; Wait, observational, and held-position Exit records are
retained in resident (for audit/Hindsight) but never enter the scarce-capital
ranking. When a candidate flips Enter → Wait/Exit its slot is removed.
*/
func (planner *Planner) updateFrontier(symbol string) {
	planner.frontierMu.Lock()
	defer planner.frontierMu.Unlock()

	record, found := planner.resident.Load(symbol)

	entry, admitted := frontierEntryFor(record, found)

	// Remove any existing slot for this symbol; it is reinserted below at its
	// fresh economic position. This is the only in-place mutation of the
	// maintained ordering — nothing else scans or re-sorts the frontier.
	kept := planner.frontier[:0]

	for _, existing := range planner.frontier {
		if existing.symbol == symbol {
			continue
		}

		kept = append(kept, existing)
	}

	planner.frontier = kept

	if !admitted {
		return
	}

	position, present := searchFrontierEconomic(planner.frontier, entry)

	if present {
		planner.frontier[position] = entry
		return
	}

	planner.frontier = append(planner.frontier, frontierEntry{})
	copy(planner.frontier[position+1:], planner.frontier[position:])
	planner.frontier[position] = entry
}

/*
frontierEntryFor derives the economic frontier slot for one resident record.
An ActionEnter candidate carrying a defined enter advantage ranks by that
advantage first; a Wait, observational, or held-position Exit record carries no
enter advantage and therefore sorts after every entry candidate, symbol-ordered
for replay determinism. It is retained in the enumeration (for audit/Hindsight
and exit execution) but never competes for scarce entry capital.
*/
func frontierEntryFor(record any, found bool) (frontierEntry, bool) {
	if !found || record == nil {
		return frontierEntry{}, false
	}

	candidate, ok := record.(*residentCandidate)

	if !ok || candidate == nil || candidate.decision == nil {
		return frontierEntry{}, false
	}

	alternatives := candidate.decision.Alternatives

	advantage, hasAdvantage := alternatives["economic:enter_advantage"]

	if !hasAdvantage {
		advantage = math.Inf(-1)
	}

	visits := alternatives["economic:visits"]

	return frontierEntry{
		symbol:    candidate.decision.Symbol,
		advantage: advantage,
		visits:    visits,
	}, true
}

/*
searchFrontierEconomic finds the insertion position for an entry by binary
search over the economic ordering, reporting whether an equal entry is present.
*/
func searchFrontierEconomic(entries []frontierEntry, entry frontierEntry) (int, bool) {
	low, high := 0, len(entries)

	for low < high {
		mid := (low + high) / 2

		if frontierOrderedBefore(entries[mid], entry) {
			low = mid + 1
		} else {
			high = mid
		}
	}

	if low < len(entries) && frontierEqual(entries[low], entry) {
		return low, true
	}

	return low, false
}

/*
frontierOrderedBefore reports whether left precedes right in the economic
frontier order: advantage descending, then visits descending, then symbol
ascending for replay determinism.
*/
func frontierOrderedBefore(left, right frontierEntry) bool {
	if left.advantage != right.advantage {
		return left.advantage > right.advantage
	}

	if left.visits != right.visits {
		return left.visits > right.visits
	}

	return left.symbol < right.symbol
}

func frontierEqual(left, right frontierEntry) bool {
	return left.advantage == right.advantage &&
		left.visits == right.visits &&
		left.symbol == right.symbol
}

/*
signatureCurrent reports whether the cheap execution/portfolio facts a
candidate was evaluated under still match current truth and whether the
explicit portfolio generation is unchanged. Any difference makes the stored
economics stale. This is identity comparison, not a freshness window: a
candidate stays valid across arbitrarily many unrelated passes.
*/
func (planner *Planner) signatureCurrent(symbol string, candidate *residentCandidate) bool {
	if candidate == nil {
		return false
	}

	if candidate.portGen != planner.portfolioGen.Load() {
		return false
	}

	inputs := planner.marketInputsFor(symbol)
	heldQty, entryPrice := planner.heldPosition(symbol)
	cash := inputs.cash

	if !inputs.available && planner.desk != nil {
		if balance := planner.desk.Balance().Cash(); balance != nil {
			cash = balance.Float64()
		}
	}

	return candidate.inputs == inputs &&
		candidate.heldQty == heldQty &&
		candidate.entryPrice == entryPrice &&
		candidate.cash == cash
}

/*
causalStateKey identifies a CausalState by its causal coordinate: epoch,
schema version, model version, publication instant, and identification status.
A newer state (any differing field) replaces the resident record on the next
pass.
*/
func causalStateKey(state *CausalState) string {
	if state == nil {
		return ""
	}

	return strconv.FormatUint(state.Epoch, 10) +
		":" + strconv.FormatUint(state.SchemaVersion, 10) +
		":" + state.ModelVersion +
		":" + strconv.FormatInt(state.At.UnixNano(), 10) +
		":" + state.Identification.String()
}

/*
cloneDecision produces a fresh decision shell sharing the immutable provenance
of the evaluated record. Allocation mutates the clone (action, reason,
stoploss, allocation class), so the resident record itself stays reusable.
*/
func cloneDecision(source *types.Decision) *types.Decision {
	if source == nil {
		return nil
	}

	clone := *source

	if source.Alternatives != nil {
		clone.Alternatives = make(map[string]float64, len(source.Alternatives))

		for key, value := range source.Alternatives {
			clone.Alternatives[key] = value
		}
	}

	return &clone
}

/*
hasSufficientObservations verifies the cheap preconditions that make a causal
evaluation meaningful: a present market state, at least twenty retained
observations for the symbol's primary coordinates, identified transitions, and
available execution-market inputs. When any precondition fails the reason string
explains exactly which one.
*/
func (planner *Planner) hasSufficientObservations(symbol string, state *CausalState) (bool, string) {
	if state == nil || state.MarketState.Current == nil {
		return false, "planner: causal market state unavailable"
	}

	if planner.reasoner == nil || planner.reasoner.Store() == nil {
		return false, "planner: observation store unavailable"
	}

	store := planner.reasoner.Store()

	primaryObservations := 0

	for coordinate := range state.MarketState.Current {
		primaryObservations = max(primaryObservations, store.Count(coordinate))
	}

	if primaryObservations < 20 {
		return false, "planner: insufficient historical observations for symbol"
	}

	if state.Identification != causal.IdentificationIdentified || state.Transition == nil {
		return false, "planner: causal transition not identified"
	}

	inputs := planner.marketInputsFor(symbol)

	if !inputs.available || !(inputs.mark > 0) {
		return false, "planner: broker market inputs unavailable (cash, mark, or fee)"
	}

	return true, ""
}

/*
decisionFromCausalState runs the economic MCTS for one symbol and maps the
result to a Decision. If the causal evaluation is unavailable the decision
represents that explicitly: it is ActionNothing with a reason, never a
fabricated Wait win and never a hidden semantic gate.
*/
func (planner *Planner) decisionFromCausalState(
	state *CausalState,
	config *system.Config,
	inputs marketInputs,
) *types.Decision {
	if state == nil {
		decision := types.NewDecision(types.ActionNothing, "")
		decision.Reason = "planner: no causal state for symbol"
		decision.ValuationAttempted = false
		decision.ValuationAvailable = false
		decision.UtilityAvailable = false
		return decision
	}

	decision := types.NewDecision(types.ActionNothing, state.Symbol)
	decision.At = state.At
	decision.CausalIdentification = state.Identification.String()

	alternatives := make(map[string]float64)
	decision.Alternatives = alternatives

	alternatives["causal:epoch"] = float64(state.Epoch)
	alternatives["causal:schema_version"] = float64(state.SchemaVersion)
	alternatives["causal:identification"] = float64(state.Identification)

	if state.BlockingCoordinate != nil {
		decision.CausalBlockingCoordinate = state.BlockingCoordinate.ID()

		if state.BlockingTransition != nil {
			alternatives["causal:blocking_rank"] = float64(state.BlockingTransition.Rank)
			alternatives["causal:blocking_observations"] = float64(state.BlockingTransition.ObservationCount)
			alternatives["causal:blocking_aligned"] = float64(state.BlockingTransition.AlignedCount)
			alternatives["causal:blocking_parameters"] = float64(state.BlockingTransition.ParameterCount)
		}
	}

	if state.Identification != causal.IdentificationIdentified ||
		state.Transition == nil {
		decision.ValuationAttempted = true
		decision.ValuationAvailable = false
		decision.ValuationStatus = state.Identification.String()
		decision.UtilityAvailable = false

		decision.Reason = "planner: causal evaluation unavailable: " + state.Identification.String()

		if state.BlockingCoordinate != nil {
			decision.Reason += " on " + state.BlockingCoordinate.ID()
		}

		if state.BlockingReason != "" {
			decision.Reason += ": " + state.BlockingReason
		}

		return decision
	}

	// Observation sufficiency gates the search before any MCTS rollout: an
	// identified model built on fewer than twenty retained observations for
	// its primary coordinates, or without available execution-market inputs,
	// produces no causal evaluation.
	if ok, reason := planner.hasSufficientObservations(state.Symbol, state); !ok {
		decision.ValuationAttempted = false
		decision.ValuationAvailable = false
		decision.ValuationStatus = "insufficient_observations"
		decision.UtilityAvailable = false
		decision.Reason = reason
		return decision
	}

	position, _ := planner.heldPosition(state.Symbol)

	if !inputs.available || !(inputs.mark > 0) {
		decision.ValuationAttempted = false
		decision.ValuationAvailable = false
		decision.ValuationStatus = "missing_execution_market"
		decision.UtilityAvailable = false
		decision.Reason = "planner: broker market inputs unavailable (cash, mark, or fee)"
		return decision
	}

	cash := inputs.cash
	mark := inputs.mark

	markDec := decimal.NewFromFloat64(mark)
	decision.Mark = markDec

	feeRate := inputs.feeRate
	spreadFraction := inputs.spreadFraction

	// UnitQuantity is the sized base quantity one new position unit
	// represents (allocation policy). The held position (if any) is the
	// actual base quantity, so Exit and Scale economics use real holdings,
	// not a lot count.
	unitQuantity := (cash * config.Planner.MaxAllocationFraction) / mark

	if unitQuantity <= 0 {
		decision.ValuationAttempted = false
		decision.ValuationAvailable = false
		decision.ValuationStatus = "no_allocatable_capital"
		decision.UtilityAvailable = false
		decision.Reason = "planner: no allocatable capital for economic evaluation"
		return decision
	}

	horizon := config.Planner.SearchHorizon

	if horizon < 1 {
		horizon = 5
	}

	// Scale is not executable by the broker yet, so the live exposure policy
	// allows exactly one sized unit: Scale is never offered by the search
	// until broker execution exists. The library retains Scale as an action.
	maxPosition := unitQuantity

	economicState := mcts.NewEconomicState(
		mcts.PortfolioState{Cash: cash, Position: position, MarkPrice: mark},
		state.MarketState,
		&causalMarketModel{state: state},
		mcts.CostModel{
			FeeRate:          feeRate,
			SpreadFraction:   spreadFraction,
			SlippageFraction: config.Planner.SlippageFraction,
		},
		unitQuantity,
		maxPosition,
		horizon,
	)

	search := mcts.NewSearch(
		config.Planner.MCTSIterations,
		config.Planner.ExplorationConstant,
		config.Planner.UncertaintyWeight,
		causalSeed(state),
	)
	mctsStarted := time.Now()
	result := search.Run(economicState, &causalActionEstimator{state: state})

	if planner.ObserveModule != nil {
		planner.ObserveModule("mcts", time.Since(mctsStarted))
	}

	decision.Cause = "causal-mcts"
	decision.ValuationAttempted = true
	decision.ValuationStatus = result.IdentificationStatus.String()

	if result.DecisionUnavailable {
		decision.ValuationAvailable = false
		decision.UtilityAvailable = false
		decision.Reason = "planner: no feasible action has an estimable economic objective (" +
			result.IdentificationStatus.String() + ")"
		recordEconomic(decision, result, state)
		decision.Trace = economicTrace(state, result)
		return decision
	}

	decision.ValuationAvailable = true
	decision.UtilityAvailable = true
	decision.Utility = result.ExpectedEconomicOutcome
	recordEconomic(decision, result, state)

	// Incremental economic advantage: Enter mean less Wait mean, read from the
	// explored root branches. Zero is the natural indifference point — a
	// candidate whose marginal value over waiting is not strictly positive is
	// not a valuable opportunity, regardless of how it ranks against other
	// candidates in the round. Entries only: exits are governed by their own
	// authority and are never gated by an enter advantage they cannot have.
	_, enterFound, _, waitFound, enterAdvantage := recordEntryEconomic(
		decision,
		result,
	)

	// Exit economics are symmetrical to entry economics: for a held position,
	// the decision MCTS actually explores is Exit versus Wait, so the branch
	// means for those two explorations are recorded explicitly. An unexplored
	// branch stays absent — never zero-filled.
	recordExitEconomic(decision, result, position)

	if result.SelectedAction == mcts.Enter {
		// The economic search already charges round-trip friction (entry and
		// terminal exit fees and spread) inside the rollout reward, so an
		// entry must now clear a minimum profit hurdle on top of those costs
		// before it may execute. The hurdle scales with the sized entry
		// notional (allocation policy), never with absolute cash, and any
		// noise in the causal coefficients that leaves the advantage below
		// this margin is treated as no opportunity rather than an entry.
		minAdvantage := unitQuantity * mark * config.Planner.MinAdvantageFraction

		if !enterFound || !waitFound || enterAdvantage < minAdvantage {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: enter advantage does not clear minimum economic hurdle"
			decision.Trace = economicTrace(state, result)
			return decision
		}

		// A strongly negative signed CVD fraction or heavy bid-side withdrawal
		// toxic flow contradicts a long entry regardless of how the price
		// transition scores it: the recorded precursor telemetry must not
		// oppose the trade the search selected.
		if cvd, found := alternatives["precursor:cvd_signed_fraction"]; found && cvd < -1.5 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: entry rejected due to opposing precursor flow"
			decision.Trace = economicTrace(state, result)
			return decision
		}

		if withdraw, found := alternatives["precursor:toxicity_withdrawal_bid"]; found && withdraw > 1.5 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: entry rejected due to opposing precursor flow"
			decision.Trace = economicTrace(state, result)
			return decision
		}
	}

	switch result.SelectedAction {
	case mcts.Enter:
		if position > 0 {
			// A held position never receives another entry: the exposure
			// policy admits exactly one sized unit, so a held symbol holds.
			decision.Action = types.ActionNothing
			break
		}

		decision.Action = types.ActionEnter
	case mcts.Exit:
		if position <= 0 {
			decision.Action = types.ActionNothing
			break
		}

		decision.Action = types.ActionExit
		// The lifecycle/audit provenance must make unambiguous that this
		// exit is the Planner's MCTS decision, never a StopLoss trigger.
		decision.Cause = "planner_mcts"
		decision.Reason = "planner_mcts: exit selected over wait"
	case mcts.Scale:
		// Defensive: Scale cannot fire while the exposure cap is one unit,
		// but if it ever does it must not be reported as an executable
		// action the broker would silently ignore.
		decision.Reason = "planner: scale is not executable by the broker"
	case mcts.Wait:
		// A genuine Wait choice: expected net wealth change was lower than
		// alternatives. It carries no reason and is not published as a gate.
		decision.Reason = ""
	default:
		decision.Reason = "planner: unknown selected action"
	}

	decision.Trace = economicTrace(state, result)
	return decision
}

/*
prerequisitesReady reports whether the trading stage's execution
prerequisites are filled: the instrument and fee surface is loaded and at
least one tradable symbol has a live quote. This is an operational
readiness check (the system cannot price or execute without them), not an
evidence gate.
*/
func (planner *Planner) prerequisitesReady() bool {
	if planner == nil || planner.desk == nil {
		return false
	}

	if planner.desk.Instrument().Status() != types.READY {
		return false
	}

	for _, symbol := range planner.desk.Instrument().Symbols() {
		if planner.desk.Price().Tick(symbol) != nil {
			return true
		}
	}

	return false
}

/*
heldPosition returns the actual held base quantity and entry price of one
symbol from the desk, or zero when flat. Position economics in the causal
search use real holdings, never a lot count.
*/
func (planner *Planner) heldPosition(symbol string) (float64, float64) {
	if planner == nil || planner.desk == nil {
		return 0, 0
	}

	for position := range planner.desk.Positions() {
		if position == nil || position.Decision.Symbol != symbol {
			continue
		}

		if position.Holding == nil || position.Holding.Qty == nil || position.Holding.Qty.Sign() <= 0 {
			continue
		}

		quantity := position.Holding.Qty.Float64()
		entryPrice := 0.0

		if position.Holding.EntryPrice != nil {
			entryPrice = position.Holding.EntryPrice.Float64()
		}

		return quantity, entryPrice
	}

	return 0, 0
}

/*
marketInputs are the current execution-market values the economic evaluation
requires. available reports whether every required value was actually
obtained; fabricated fallbacks are never substituted.
*/
type marketInputs struct {
	cash           float64
	mark           float64
	feeRate        float64
	spreadFraction float64
	available      bool
}

/*
marketInputsFor returns the current execution-market inputs for one symbol.
The default provider reads the broker desk and reports unavailable when any
required value (cash, mark, or fee) is missing; tests may inject a
deterministic provider.
*/
func (planner *Planner) marketInputsFor(symbol string) marketInputs {
	if planner == nil {
		return marketInputs{}
	}

	if planner.marketProvider != nil {
		return planner.marketProvider(symbol)
	}

	return planner.deskMarketInputs(symbol)
}

func (planner *Planner) deskMarketInputs(symbol string) marketInputs {
	if planner == nil || planner.desk == nil {
		return marketInputs{}
	}

	// Probe availability with the non-logging accessors: a missing tick or
	// fee is an unavailable state, not an error to be logged every pass.
	balance := planner.desk.Balance().Cash()
	tick := planner.desk.Price().Tick(symbol)
	fee := planner.desk.Price().FeeIfAvailable(symbol)

	if balance == nil || tick == nil || tick.Ask == nil || fee == nil || fee.Fee == nil {
		return marketInputs{}
	}

	inputs := marketInputs{
		cash:    balance.Float64(),
		feeRate: fee.Fee.Float64() / 100,
	}

	// The ask mark for valuation in MCTS, with fee handled directly by CostModel.
	inputs.mark = tick.Ask.Float64()

	if tick.Bid != nil && tick.Bid.Float64() > 0 && tick.Ask.Float64() > tick.Bid.Float64() {
		bid := tick.Bid.Float64()
		ask := tick.Ask.Float64()
		inputs.spreadFraction = (ask - bid) / ((ask + bid) / 2)
	}

	inputs.available = true

	return inputs
}

/*
recordEconomic records the economic outcome provenance on the decision.
*/
func recordEconomic(decision *types.Decision, result *mcts.SearchResult, state *CausalState) {
	// Utility is the search's expected economic outcome for the selected
	// action: the same value the causal rollout actually optimized, surfaced
	// directly rather than reconstructed by the frontend.
	decision.Utility = result.ExpectedEconomicOutcome

	alternatives := decision.Alternatives
	alternatives["economic:expected_outcome"] = result.ExpectedEconomicOutcome
	alternatives["economic:outcome_uncertainty"] = result.OutcomeUncertainty
	alternatives["economic:visits"] = float64(result.Visits)

	if state != nil && state.Transition != nil {
		alternatives["causal:effective_support"] = state.Transition.EffectiveSupport
	}

	// Precursor telemetry: the structural conditions that may justify an
	// entry before the price actually moves. Missing values are absent keys,
	// never fabricated zeros.
	if radius, found := marketValue(state, "hawkes", "branching_spectral_radius", ""); found {
		alternatives["precursor:hawkes_supercritical"] = radius
	}

	if retreat, found := marketValue(state, "toxicity", "retreat_rate", "ask"); found {
		alternatives["precursor:ask_retreat_velocity"] = retreat
	}

	if basis, found := marketValue(state, "derivatives", "basis_zscore", ""); found {
		alternatives["precursor:basis_tension"] = basis
	}

	if cvdSigned, found := marketValue(state, "cvd", "signed_net_fraction_zscore", ""); found {
		alternatives["precursor:cvd_signed_fraction"] = cvdSigned
	}

	if cvdGross, found := marketValue(state, "cvd", "gross_notional_rate_zscore", ""); found {
		alternatives["precursor:cvd_gross_rate"] = cvdGross
	}

	if bookImb, found := marketValue(state, "depthflow", "book_imbalance", ""); found {
		alternatives["precursor:depthflow_book_imbalance"] = bookImb
	}

	if resGap, found := marketValue(state, "depthflow", "imbalance_resolution_gap", ""); found {
		alternatives["precursor:depthflow_resolution_gap"] = resGap
	}

	if spread, found := marketValue(state, "liquidity", "relative_spread", ""); found {
		alternatives["precursor:liquidity_spread"] = spread
	}

	if lagCorr, found := marketValue(state, "leadlag", "best_lag_correlation", ""); found {
		alternatives["precursor:leadlag_correlation"] = lagCorr
	}

	if consensus, found := marketValue(state, "sentiment", "directional_consensus", ""); found {
		alternatives["precursor:sentiment_consensus"] = consensus
	}

	if oiGrowth, found := marketValue(state, "derivatives", "open_interest_growth_zscore", ""); found {
		alternatives["precursor:derivatives_oi_growth"] = oiGrowth
	}

	if liqRate, found := marketValue(state, "derivatives", "liquidation_notional_rate", ""); found {
		alternatives["precursor:derivatives_liquidation_rate"] = liqRate
	}

	if withBid, found := marketValue(state, "toxicity", "withdrawal_fraction_zscore", "bid"); found {
		alternatives["precursor:toxicity_withdrawal_bid"] = withBid
	}

	if withAsk, found := marketValue(state, "toxicity", "withdrawal_fraction_zscore", "ask"); found {
		alternatives["precursor:toxicity_withdrawal_ask"] = withAsk
	}

	if fillBid, found := marketValue(state, "toxicity", "fill_fraction_zscore", "bid"); found {
		alternatives["precursor:toxicity_fill_bid"] = fillBid
	}

	if fillAsk, found := marketValue(state, "toxicity", "fill_fraction_zscore", "ask"); found {
		alternatives["precursor:toxicity_fill_ask"] = fillAsk
	}
}

/*
marketValue reads one market coordinate's current value from the causal state,
matched by structural selector (source, metric, side) rather than the full
symbol-bound identity, so the precursor telemetry stays decoupled from the
per-symbol coordinate rewrite.
*/
func marketValue(state *CausalState, source, metric, side string) (float64, bool) {
	if state == nil {
		return 0, false
	}

	for coordinate, value := range state.MarketState.Current {
		if coordinate.Source == source && coordinate.Metric == metric && coordinate.Side == side {
			return value, true
		}
	}

	return 0, false
}

/*
branchMean returns the mean economic reward observed for one root action
branch, read directly off the search provenance. It reports whether the
branch was actually explored, so callers never fabricate a zero for an
absent alternative.
*/
func branchMean(result *mcts.SearchResult, action mcts.Action) (float64, bool) {
	if result == nil || result.Trace == nil {
		return 0, false
	}

	for _, branch := range result.Trace.Branches {
		if branch.Action == action {
			return branch.MeanReward, true
		}
	}

	return 0, false
}

/*
recordEntryEconomic records the Entry-versus-Wait branch economics for the
decision search, symmetrical to recordExitEconomic. Each mean key is present
only when the branch was actually explored; the exploration flags make absence
explicit so a zero is never fabricated for an unexplored branch. The entry
advantage is defined only when both branches were explored and is returned for
the caller's action gate.
*/
func recordEntryEconomic(
	decision *types.Decision,
	result *mcts.SearchResult,
) (enterMean float64, enterFound bool, waitMean float64, waitFound bool, advantage float64) {
	if decision == nil || result == nil {
		return 0, false, 0, false, 0
	}

	alternatives := decision.Alternatives

	if alternatives == nil {
		alternatives = make(map[string]float64)
		decision.Alternatives = alternatives
	}

	enterMean, enterFound = branchMean(result, mcts.Enter)
	waitMean, waitFound = branchMean(result, mcts.Wait)

	if enterFound {
		alternatives["economic:enter_mean"] = enterMean
		alternatives["economic:enter_explored"] = 1
	} else {
		alternatives["economic:enter_explored"] = 0
	}

	if waitFound {
		alternatives["economic:wait_mean"] = waitMean
	}

	if enterFound && waitFound {
		advantage = enterMean - waitMean
		alternatives["economic:enter_advantage"] = advantage
	}

	return enterMean, enterFound, waitMean, waitFound, advantage
}

/*
recordExitEconomic records the Exit-versus-Wait branch economics for a held
position, symmetrical to the entry advantage. Each key is present only when the
branch was actually explored; the exploration flag makes absence explicit so a
zero is never fabricated for an unexplored branch.
*/
func recordExitEconomic(
	decision *types.Decision,
	result *mcts.SearchResult,
	position float64,
) {
	if decision == nil || result == nil || position <= 0 {
		return
	}

	alternatives := decision.Alternatives

	if alternatives == nil {
		alternatives = make(map[string]float64)
		decision.Alternatives = alternatives
	}

	if exitMean, found := branchMean(result, mcts.Exit); found {
		alternatives["economic:exit_mean"] = exitMean
		alternatives["economic:exit_explored"] = 1
	} else {
		alternatives["economic:exit_explored"] = 0
	}

	if waitMean, found := branchMean(result, mcts.Wait); found {
		alternatives["economic:wait_mean_exit"] = waitMean
	}

	exitMean, exitFound := branchMean(result, mcts.Exit)
	waitMean, waitFound := branchMean(result, mcts.Wait)

	if exitFound && waitFound {
		alternatives["economic:exit_advantage"] = exitMean - waitMean
	}
}

/*
economicTrace builds the observable decision trace: the MCTS branches with
their economic rewards plus the causal model provenance.
*/
func economicTrace(state *CausalState, result *mcts.SearchResult) *types.DecisionTrace {
	trace := &types.DecisionTrace{
		Hypothesis: "causal:" + state.Symbol + ":economic",
		MCTS: types.DecisionMCTSTrace{
			Iterations:        0,
			Branches:          make([]types.DecisionMCTSBranch, 0),
			RecommendedAction: result.SelectedAction.String(),
		},
	}

	if result.Trace != nil {
		trace.MCTS.Iterations = result.Trace.Iterations

		for _, branch := range result.Trace.Branches {
			trace.MCTS.Branches = append(trace.MCTS.Branches, types.DecisionMCTSBranch{
				Action:     branch.Action.String(),
				Visits:     branch.Visits,
				MeanReward: branch.MeanReward,
			})
		}
	}

	trace.MCTS.Tree = result.Tree

	return trace
}

/*
causalSeed fingerprints the causal state so the same replay state explores the
same rollout paths.
*/
func causalSeed(state *CausalState) int64 {
	if state == nil {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(state.Symbol))

	buffer := make([]byte, 0, 32)
	buffer = strconv.AppendInt(buffer, state.At.UnixNano(), 10)
	_, _ = hasher.Write(buffer)
	buffer = buffer[:0]
	buffer = strconv.AppendUint(buffer, state.Epoch, 10)
	_, _ = hasher.Write(buffer)

	return int64(hasher.Sum64())
}

/*
executeDecisions executes the selected actions through the desk. Exits run
first; entries follow. No semantic gate, admission policy, or predictive
readiness check runs here: the desk rejects only real execution constraints.
*/
func (planner *Planner) executeDecisions(
	decisions []*types.Decision,
) error {
	exits := make([]*types.Decision, 0, len(decisions))
	winners := make([]*types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		switch decision.Action {
		case types.ActionExit:
			exits = append(exits, decision)
		case types.ActionEnter:
			winners = append(winners, decision)
		}
	}

	var g errgroup.Group
	g.SetLimit(goruntime.GOMAXPROCS(0))

	if planner.desk == nil {
		return nil
	}

	for _, decision := range exits {
		decision := decision
		g.Go(func() error {
			if err := planner.desk.Execute(*decision); err != nil {
				decision.Reason = "planner: exit is no longer executable: " + err.Error()

				if !errnie.IsNotAcceptable(err) {
					return fmt.Errorf("planner: execute exit %s: %w", decision.Symbol, err)
				}
				return nil
			}

			planner.portfolioGen.Add(1)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	var g2 errgroup.Group
	g2.SetLimit(goruntime.GOMAXPROCS(0))
	for _, decision := range winners {
		if decision.Action != types.ActionEnter {
			continue
		}

		decision := decision
		g2.Go(func() error {
			var err error

			if planner.executeEntry != nil {
				err = planner.executeEntry(*decision)
			} else {
				err = planner.desk.Execute(*decision)
			}

			if err != nil {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: entry is no longer executable: " + err.Error()

				if !errnie.IsNotAcceptable(err) {
					return fmt.Errorf("planner: execute %s: %w", decision.Symbol, err)
				}
				return nil
			}

			planner.portfolioGen.Add(1)

			return nil
		})
	}

	return g2.Wait()
}

/*
isExcludedSymbol keeps fiat and stablecoin pairs out of the tradable universe.
It is an operational instrument filter, not an evidence gate.
*/
func isExcludedSymbol(symbol string) bool {
	base := symbol

	if index := strings.Index(symbol, "/"); index != -1 {
		base = symbol[:index]
	}

	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD",
		"USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG",
		"USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD",
		"CUSD", "USD0", "USDS", "RLUSD", "UST":
		return true
	default:
		return false
	}
}
