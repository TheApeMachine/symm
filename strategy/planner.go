package strategy

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
	uiStates   *runtime.Channel[*CausalState]
	strategy   *runtime.Channel[types.StrategyRound]
	tickWork   *runtime.Subscription[kraken.TickerData]
	stateWork  *runtime.Subscription[*CausalState]
	pending    sync.Map
	lastPass   int64
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
	bus *runtime.Workspace,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	config := system.Cfg.Snapshot()

	epoch := uint64(1)
	historyCapacity := 512
	relationInterval := time.Second
	measurementStep := time.Second
	relationMaxLag := 30 * time.Second
	schemaTemplate := DefaultCausalSchema(epoch, measurementStep)
	plans := RelationPlansFromSchema(schemaTemplate, epoch, relationMaxLag)

	if config != nil && config.Planner != nil {
		relationInterval = config.Planner.RelationInterval

		if relationInterval <= 0 {
			relationInterval = time.Second
		}

		if config.Planner.MeasurementStep > 0 {
			measurementStep = config.Planner.MeasurementStep
			schemaTemplate = DefaultCausalSchema(epoch, measurementStep)
		}

		if config.Planner.RelationMaxLag > 0 {
			relationMaxLag = config.Planner.RelationMaxLag
		}

		// The candidate Relation space is always generated from the schema's
		// authorized edges, so the two cannot drift apart.
		plans = RelationPlansFromSchema(schemaTemplate, epoch, relationMaxLag)
	}

	reasoner, reasonerErr := NewReasoner(epoch, historyCapacity, plans, schemaTemplate, relationInterval)

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
	planner.strategy = runtime.ChannelOf[types.StrategyRound](
		bus, types.ChannelDecisions,
		func(round types.StrategyRound) string { return "" },
	)
	planner.uiStates = runtime.ChannelOf[*CausalState](
		bus, types.ChannelCausalState,
		func(state *CausalState) string { return state.Symbol },
	)
	planner.stateWork = runtime.ChannelOf[*CausalState](
		bus, types.ChannelCausalState,
		func(state *CausalState) string { return state.Symbol },
	).Subscribe(planner.Name(), planner.Step)
	planner.tickWork = runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return "" },
	).Subscribe(planner.Name(), planner.StepTick)

	measurementWork := runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	).Subscribe(planner.Name()+"-reasoner", planner.StepMeasurement)

	planner.reasoner.SetOnState(planner.publishCausalState)
	planner.tradingGate = planner.prerequisitesReady
	_ = measurementWork

	return planner
}

func (planner *Planner) Name() string { return "planner" }

func (planner *Planner) Error() error { return planner.err }

func (planner *Planner) Stager() *audit.Stager {
	return planner.stager
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
StepMeasurement feeds one Measurement into the reasoner. The reasoner appends
observations, refreshes planned Relations, and publishes the resulting
CausalState on ChannelCausalState.
*/
func (planner *Planner) StepMeasurement(measurement *nmtypes.Measurement) error {
	if planner == nil || planner.reasoner == nil || measurement == nil {
		return nil
	}

	planner.reasoner.Ingest(measurement)
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

	if planner.uiStates != nil {
		planner.uiStates.Publish(state)
	}
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

// StepTick runs one portfolio pass at most once per engine tick.
func (planner *Planner) StepTick(ticker kraken.TickerData) error {
	if planner.lastPass == planner.thesis.Tick {
		return nil
	}

	planner.lastPass = planner.thesis.Tick

	return planner.Update(planner.thesis)
}

/*
Update runs one economic MCTS round over every symbol with a fresh causal
state, then allocates and executes the selected actions. Semantic readiness,
admission policy, and predictive readiness do not gate participation: every
symbol with observational state, an explicit schema, and a feasible action
may be considered.
*/
func (planner *Planner) Update(thesis *types.Thesis) error {
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
		return fmt.Errorf("planner: planner configuration required")
	}

	states := planner.drainPending()
	decisions := make([]*types.Decision, 0, len(states))

	plannerStarted := time.Now()
	defer func() {
		if planner.ObserveModule != nil {
			planner.ObserveModule("planner", time.Since(plannerStarted))
		}
	}()

	for _, state := range states {
		if state == nil || isExcludedSymbol(state.Symbol) {
			continue
		}

		// A symbol with no executable quote (or no cash/fee surface yet) is
		// not priced: no economic decision is possible, so none is
		// attempted. During startup this keeps the planner from churning on
		// symbols the market data has not reached.
		inputs := planner.marketInputsFor(state.Symbol)

		if !inputs.available {
			continue
		}

		decision := planner.decisionFromCausalState(state, config, inputs)
		decisions = append(decisions, decision)
	}

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

		planner.publishStrategy(thesis, false, "accumulating", decisions)
		return nil
	}

	if planner.allocation != nil {
		allocationStarted := time.Now()

		if err := planner.allocation.Calculate(decisions); err != nil {
			return err
		}

		if planner.ObserveModule != nil {
			planner.ObserveModule("allocation", time.Since(allocationStarted))
		}
	}

	if err := planner.executeDecisions(decisions); err != nil {
		return err
	}

	for _, decision := range decisions {
		planner.stager.Stage(decision, 10*time.Minute)
	}

	planner.publishStrategy(thesis, true, "decisions", decisions)
	return nil
}

/*
drainPending collects and clears the retained causal states.
*/
func (planner *Planner) drainPending() []*CausalState {
	states := make([]*CausalState, 0)

	planner.pending.Range(func(key, value any) bool {
		if state, ok := value.(*CausalState); ok && state != nil {
			states = append(states, state)
		}

		planner.pending.Delete(key)
		return true
	})

	return states
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
		return decision
	}

	decision := types.NewDecision(types.ActionNothing, state.Symbol)
	decision.At = state.At
	alternatives := make(map[string]float64)
	decision.Alternatives = alternatives

	alternatives["causal:epoch"] = float64(state.Epoch)
	alternatives["causal:schema_version"] = float64(state.SchemaVersion)
	alternatives["causal:identification"] = float64(state.Identification)

	if state.Identification != causal.IdentificationIdentified ||
		state.Transition == nil {
		decision.Reason = "planner: causal evaluation unavailable: " + state.Identification.String()
		return decision
	}

	position, _ := planner.heldPosition(state.Symbol)

	if !inputs.available || !(inputs.mark > 0) {
		decision.Reason = "planner: broker market inputs unavailable (cash, mark, or fee)"
		return decision
	}

	cash := inputs.cash
	mark := inputs.mark
	feeRate := inputs.feeRate
	spreadFraction := inputs.spreadFraction

	// UnitQuantity is the sized base quantity one new position unit
	// represents (allocation policy). The held position (if any) is the
	// actual base quantity, so Exit and Scale economics use real holdings,
	// not a lot count.
	unitQuantity := (cash * config.Planner.MaxAllocationFraction) / mark

	if unitQuantity <= 0 {
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
	result := search.Run(economicState, &causalActionEstimator{state: state})

	decision.Cause = "causal-mcts"

	if result.DecisionUnavailable {
		decision.Reason = "planner: no feasible action has an estimable economic objective (" +
			result.IdentificationStatus.String() + ")"
		recordEconomic(decision, result, state)
		decision.Trace = economicTrace(state, result)
		return decision
	}

	recordEconomic(decision, result, state)

	switch result.SelectedAction {
	case mcts.Enter:
		decision.Action = types.ActionEnter
	case mcts.Exit:
		decision.Action = types.ActionExit
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

	// The fee-inclusive buy mark, matching broker.Price.Mark(BUY):
	// ask * (1 + feeRate).
	inputs.mark = tick.Ask.Float64() * (1 + inputs.feeRate)

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
	alternatives := decision.Alternatives
	alternatives["economic:expected_outcome"] = result.ExpectedEconomicOutcome
	alternatives["economic:outcome_uncertainty"] = result.OutcomeUncertainty
	alternatives["economic:visits"] = float64(result.Visits)

	if state != nil && state.Transition != nil {
		alternatives["causal:effective_support"] = state.Transition.EffectiveSupport
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
publishStrategy emits the planning decision round as domain data on
ChannelDecisions. The workspace observer (boot) projects it into the dashboard
StrategyFrame; the planner never publishes UI directly.
*/
func (planner *Planner) publishStrategy(
	thesis *types.Thesis,
	evaluated bool,
	outcome string,
	decisions []*types.Decision,
) {
	if planner.strategy != nil {
		planner.strategy.Publish(types.StrategyRound{
			Evaluated: evaluated,
			Outcome:   outcome,
			Decisions: decisions,
		})
	}
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

	if planner.desk == nil {
		return nil
	}

	for _, decision := range exits {
		if err := planner.desk.Execute(*decision); err != nil {
			decision.Reason = "planner: exit is no longer executable: " + err.Error()

			if !errnie.IsNotAcceptable(err) {
				return fmt.Errorf("planner: execute exit %s: %w", decision.Symbol, err)
			}
		}
	}

	for _, decision := range winners {
		if decision.Action != types.ActionEnter {
			continue
		}

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

			continue
		}
	}

	return nil
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
