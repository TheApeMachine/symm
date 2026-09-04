package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Search policy. Every constant here is declared strategy policy in the units the
economic reward is measured in; none is derived from market evidence.
*/
const (
	// searchIterations is the rollout budget per planning round. Pearl's
	// counterfactual backpropagation updates every sibling branch on each
	// rollout, so this buys far more branch statistics than the same count
	// of one-branch-per-rollout iterations would.
	searchIterations = 24
	// searchExploration is the UCB exploration constant, in reward units.
	searchExploration = 0.5
	// searchUncertainty scales the reward standard error in selection.
	searchUncertainty = 0.25
	// searchHorizon is the rollout depth in ticker steps.
	searchHorizon = 5
	// causalMinimumRows is the observational support floor below which no
	// causal query runs and the search stays purely observational.
	causalMinimumRows = 8
	// causalExpectationWeight scales the interventional selection bias.
	causalExpectationWeight = 1.0
	// causalMaxCounterfactualMass caps virtual visits per branch so
	// counterfactual evidence informs but never outweighs real rollouts.
	causalMaxCounterfactualMass = 12
	// causalRejectionMargin is the dominance margin, in reward units, past
	// which a branch is withdrawn from selection because a live sibling's
	// identified interventional expectation decisively exceeds it. Zero means
	// strict dominance: any action another action causally beats is dropped,
	// while unidentified branches and the leader itself are always spared.
	causalRejectionMargin = 0.0
	// tickerCadence is the event-time spacing one rollout step represents.
	tickerCadence = time.Second
)

/*
Planner is the sole live entry authority. Desk-owned Stoploss instances retain
exclusive exit authority.

The univariate RLS directional predictor that previously decided entries is
gone: it duplicated Resonance's forecast through one scalar contextual score,
and gated entries on expectedLogReturn > breakEvenLogReturn, which is exactly
the scalar price prediction this architecture rejects.

Entry authority now belongs to the causal MCTS search. Until its
ActionEstimator and MarketModel are wired in, the Planner admits no entries: it
still consumes every ticker and emits a well-formed round so the strategy
surface and its maturity logic keep receiving frames, and it names the absent
decision authority explicitly rather than substituting a fabricated one.
*/
type Planner struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	err                   error
	status                *runtime.Status
	desk                  *broker.Desk
	allocation            *Allocation
	warRoom               *advisor.WarRoom
	search                *mcts.Search
	maxAllocationFraction float64
	observations          map[string][][]float64
	lastSequences         map[string]uint64
	lastTimestamps        map[string]time.Time
}

func NewPlanner(
	ctx context.Context,
	desk *broker.Desk,
) (*Planner, error) {
	if desk == nil {
		return nil, fmt.Errorf("planner: desk required")
	}

	config, err := system.Cfg.PlannerPolicy()

	if err != nil {
		return nil, err
	}

	if config.MaxAllocationFraction <= 0 || config.MaxAllocationFraction > 1 {
		return nil, fmt.Errorf("planner: allocation fraction must be in (0, 1]")
	}

	ctx, cancel := context.WithCancel(ctx)

	search := mcts.NewSearch(
		searchIterations, searchExploration, searchUncertainty, 42,
	)
	search.Causal = mcts.DefaultCausalEngine{Linear: true}
	search.CausalPolicy = mcts.EconomicCausalPolicy(
		causalMinimumRows,
		causalExpectationWeight,
		causalMaxCounterfactualMass,
		true,
	).WithRejectionFloor(causalRejectionMargin)

	return &Planner{
		ctx:                   ctx,
		cancel:                cancel,
		status:                runtime.NewStatus().Transition(runtime.READY),
		desk:                  desk,
		allocation:            NewAllocation(ctx, desk),
		warRoom:               advisor.NewWarRoom(),
		search:                search,
		maxAllocationFraction: config.MaxAllocationFraction,
		observations:          make(map[string][][]float64),
		lastSequences:         make(map[string]uint64),
		lastTimestamps:        make(map[string]time.Time),
	}, nil
}

/*
Court is the credibility ledger this Planner deliberates against.

The Arenas report resolved predictions into it, so the advisor whose calls keep
failing grows quieter at the table rather than being silenced outright
(MCTS.md §6.2). It is the same ledger the deliberation reads, deliberately: an
accountability record kept apart from the council it is supposed to weight
would judge advisors without ever changing what they are worth.
*/
func (planner *Planner) Court() *advisor.WarRoom { return planner.warRoom }

/*
Step consumes one envelope once and emits one decision round per ticker.
*/
func (planner *Planner) Step(envelope *types.Envelope) *types.Envelope {
	if planner.err != nil {
		if planner.cancel != nil {
			planner.cancel()
		}

		return nil
	}

	if envelope == nil {
		planner.halt(errnie.Error(errnie.Err(
			errnie.NotFound,
			"planner: envelope was nil",
			nil,
		)))

		return nil
	}

	if len(envelope.Perspectives) > 0 {
		planner.warRoom.Admit(envelope.Perspectives, envelopeSymbol(envelope))
	}

	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	if err := planner.validateTicker(envelope); err != nil {
		planner.halt(errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "planner: validate ticker", err,
		)))

		return nil
	}

	planner.recordObservation(envelope)

	if planner.desk != nil && planner.desk.Holding(envelope.TickerData.Symbol) > 0 {
		holdingRound := &types.StrategyRound{
			Symbol:    envelope.TickerData.Symbol,
			Evaluated: true,
			Outcome:   "managed",
			Decisions: []*types.Decision{
				{
					Action:           types.ActionNothing,
					Symbol:           envelope.TickerData.Symbol,
					At:               envelope.TickerData.Timestamp,
					PredictiveStatus: "position-held-desk-managed",
					Reason:           "planner: position is open and desk-managed by stoploss",
				},
			},
		}
		envelope.StrategyRound = holdingRound

		return envelope
	}

	round := planner.plan(envelope)
	envelope.StrategyRound = round

	decision := round.Decisions[0]

	if decision.Action == types.ActionEnter {
		if err := planner.execute(decision, round); err != nil {
			planner.halt(errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "planner: execute decision", err,
			)))

			return nil
		}
	}

	return envelope
}

/*
plan runs one full decision round: the War Room deliberates over the active
advisor perspectives, the causal MCTS search evaluates the feasible actions
under the resulting market model, and the selected action becomes the decision.

Every stage can decline. A round that cannot deliberate, cannot model the
market, or cannot identify an estimable action returns an admission naming the
exact reason, never a fabricated entry.
*/
func (planner *Planner) plan(envelope *types.Envelope) *types.StrategyRound {
	ticker := envelope.TickerData

	decision := types.NewDecision(types.ActionNothing, ticker.Symbol)
	decision.At = ticker.Timestamp
	decision.Direction = 1
	decision.ForecastSource = "resonance-forecast"
	decision.ForecastModel = "causal-mcts-pearl-v1"
	decision.ForecastHorizon = 0

	if envelope.Resonance != nil && envelope.Resonance.SupportedHorizon > 0 {
		decision.ForecastHorizon = int(envelope.Resonance.SupportedHorizon)
	} else if envelope.Resonance != nil && envelope.Resonance.Forecast != nil && envelope.Resonance.Forecast.Horizon > 0 {
		decision.ForecastHorizon = envelope.Resonance.Forecast.Horizon
	}

	decision.AllocationClass = "none"
	decision.Cause = "advisor-deliberated market context"

	round := &types.StrategyRound{
		Symbol:    ticker.Symbol,
		Evaluated: true,
		Outcome:   "admission",
		Decisions: []*types.Decision{decision},
	}

	consensus := planner.warRoom.Deliberate(
		envelope.Perspectives, ticker.Symbol, ticker.Timestamp,
	)
	decision.Confidence = consensus.Confidence
	decision.Alternatives = consensusAlternatives(consensus)

	if consensus.Participants == 0 {
		decision.PredictiveStatus = "awaiting-advisor-consensus"
		decision.Reason = "planner: no advisor has classified this symbol yet"

		return round
	}

	// 1. Resonance Is Required (Discrepancy 1)
	if envelope.Resonance == nil {
		decision.PredictiveStatus = "resonance-missing"
		decision.Reason = "planner: resonance artifact is required but missing from envelope"

		return round
	}

	if envelope.Resonance.Symbol != "" && envelope.Resonance.Symbol != ticker.Symbol {
		decision.PredictiveStatus = "resonance-symbol-mismatch"
		decision.Reason = fmt.Sprintf("planner: resonance symbol %s does not match ticker %s", envelope.Resonance.Symbol, ticker.Symbol)

		return round
	}

	if !envelope.Resonance.Calibrated {
		decision.PredictiveStatus = "resonance-uncalibrated"
		decision.Reason = "planner: resonance artifact is uncalibrated"

		return round
	}

	if envelope.Resonance.SupportedHorizon <= 0 {
		decision.PredictiveStatus = "resonance-horizon-invalid"
		decision.Reason = "planner: resonance supported horizon must be positive"

		return round
	}

	if envelope.Resonance.Forecast == nil {
		decision.PredictiveStatus = "resonance-forecast-missing"
		decision.Reason = "planner: resonance forecast is missing"

		return round
	}

	if envelope.Resonance.Forecast.Held || envelope.Resonance.Forecast.Call == 0 {
		decision.PredictiveStatus = "resonance-call-held"
		decision.Reason = "planner: resonance forecast call is held or neutral"

		return round
	}

	// Magnitude comes from the desk's own realized passage economics: how far
	// this symbol actually travels, measured, rather than asserted by a
	// precursor archetype.
	magnitude := 0.0

	if planner.desk != nil {
		pe := planner.desk.PassageEconomics(ticker.Symbol)

		if pe.FavorableExcursion.Mid > 0 {
			riskDistanceFraction := 0.01
			ask := ticker.Ask.Float64()

			if ask > 0 {
				spread := (ask - ticker.Bid.Float64()) / ask

				if spread > 0.001 {
					riskDistanceFraction = spread * 3.0
				}
			}

			magnitude = pe.FavorableExcursion.Mid * riskDistanceFraction
		}

		if magnitude == 0 {
			magnitude = planner.desk.PassageMovementMagnitude(ticker.Symbol)
		}
	}

	model, ready := newResonanceMarketModel(envelope.Resonance, tickerCadence, magnitude)
	transitionSource := "resonance-forecast"

	if !ready {
		decision.PredictiveStatus = "support-insufficient"
		decision.Reason = "planner: resonance could not model the transition"

		return round
	}

	decision.ForecastSource = transitionSource

	state, built := planner.rootState(envelope, model)

	if !built {
		decision.PredictiveStatus = "feasibility-infeasible"
		decision.Reason = "planner: cannot price the portfolio to open a search"

		return round
	}

	// Deterministic RNG seed (Discrepancy 27)
	seed := envelope.Tick

	if seed == 0 {
		seed = int64(envelope.Stream.Sequence)
	}

	if seed == 0 {
		seed = ticker.Timestamp.UnixNano()
	}

	for _, char := range ticker.Symbol {
		seed = seed*31 + int64(char)
	}

	if planner.search != nil {
		planner.search.SetSeed(seed)
	}

	var liquidationShare float64

	if envelope.Derivatives != nil && envelope.Derivatives.Metrics != nil {
		if metric, found := envelope.Derivatives.Metrics["liquidation_share"]; found {
			liquidationShare = metric.Raw
		}
	}

	opportunity := SynthesizeOpportunity(OpportunityInput{
		Symbol:           ticker.Symbol,
		Consensus:        consensus,
		Resonance:        envelope.Resonance,
		Cognition:        envelope.Cognition,
		LiquidationShare: liquidationShare,
		Desk:             planner.desk,
		At:               ticker.Timestamp,
	})

	if opportunity != nil {
		decision.OpportunityType = string(opportunity.Archetype)
		decision.OpportunityPhase = string(opportunity.Phase)
	}

	result := planner.search.Run(state, &consensusEstimator{
		consensus:   consensus,
		opportunity: opportunity,
	})

	planner.recordSearch(decision, result, consensus)
	decision.Trace = buildTrace(consensus, result, transitionSource)

	if result.DecisionUnavailable {
		decision.PredictiveStatus = "support-insufficient"
		decision.Reason = "planner: no feasible action had an estimable causal outcome (" +
			result.IdentificationStatus.String() + ")"

		return round
	}

	decision.PredictiveReady = true

	if result.SelectedAction != mcts.Enter {
		decision.PredictiveStatus = "unattractive"
		decision.Reason = "planner: causal search selected " +
			result.SelectedAction.String() + " over entering"

		return round
	}

	if opportunity == nil {
		decision.PredictiveStatus = "unattractive"
		decision.Reason = "planner: no qualified opportunity precursor to enter"

		return round
	}

	decision.PredictiveStatus = "causal-search-resolved"
	decision.Action = types.ActionEnter
	decision.Reason = "planner: causal search selected entry on the armed precursor"
	round.Outcome = "entry"

	return round
}

/*
rootState builds the search's opening state from live portfolio reality: cash
on hand, the position actually held, and the executable mark.
*/
func (planner *Planner) rootState(
	envelope *types.Envelope,
	model mcts.MarketModel,
) (*mcts.EconomicState, bool) {
	ticker := envelope.TickerData

	if planner.desk == nil || planner.desk.Balance() == nil || planner.desk.Price() == nil {
		return nil, false
	}

	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return nil, false
	}

	ask := ticker.Ask.Float64()
	bid := ticker.Bid.Float64()

	if ask <= 0 || bid <= 0 || ask < bid {
		return nil, false
	}

	feeRate, err := planner.feeRate(ticker.Symbol)

	if err != nil {
		return nil, false
	}

	budget := cash.Float64() * planner.maxAllocationFraction
	unit := budget / ask

	if unit <= 0 {
		return nil, false
	}

	crossingFraction := (ask - bid) / ask

	baseCosts := mcts.CostModel{
		FeeRate:        feeRate,
		SpreadFraction: crossingFraction,
	}

	costs := planner.entryCosts(ticker.Symbol, unit, baseCosts)

	state := mcts.NewEconomicState(
		mcts.PortfolioState{
			Cash:      cash.Float64(),
			Position:  0,
			MarkPrice: ask,
		},
		mcts.MarketState{At: ticker.Timestamp},
		model,
		costs,
		unit,
		unit,
		searchHorizon,
	)

	if history := planner.observations[ticker.Symbol]; len(history) > 0 {
		state.WithHistory(history)
	}

	return state, true
}

func (planner *Planner) recordObservation(envelope *types.Envelope) {
	if envelope == nil || envelope.TickerData.Symbol == "" || envelope.TickerData.Bid == nil {
		return
	}

	if planner.observations == nil {
		planner.observations = make(map[string][][]float64)
	}

	currentExposure := 0.0

	if planner.desk != nil {
		currentExposure = float64(planner.desk.Holding(envelope.TickerData.Symbol))
	}

	row := []float64{
		envelope.TickerData.Bid.Float64(),
		0,
		currentExposure,
		0,
	}

	history := planner.observations[envelope.TickerData.Symbol]

	if len(history) >= 64 {
		history = history[1:]
	}

	planner.observations[envelope.TickerData.Symbol] = append(history, row)
}

/*
entryCosts calculates visible order book depth impact (market impact) and spread
fraction for the requested position size, incorporating execution friction into
search economics.
*/
func (planner *Planner) entryCosts(
	symbol string,
	unit float64,
	baseCosts mcts.CostModel,
) mcts.CostModel {
	if planner.desk == nil || planner.desk.Price() == nil {
		return baseCosts
	}

	unitDecimal := decimal.NewFromFloat64(unit)
	executable, err := planner.desk.Price().ExecutableQuantity(symbol, unitDecimal)

	if err != nil || executable == nil || executable.Sign() <= 0 {
		return baseCosts
	}

	cost, err := planner.desk.Price().EntryCost(symbol, executable)

	if err != nil || cost == nil || cost.EntryPrice == nil || cost.EntryPrice.Sign() <= 0 {
		return baseCosts
	}

	entryPriceFloat := cost.EntryPrice.Float64()

	if cost.Impact != nil && entryPriceFloat > 0 {
		impactFrac := cost.Impact.Float64() / entryPriceFloat

		if impactFrac > 0 {
			baseCosts.SlippageFraction = impactFrac
		}
	}

	if cost.Spread != nil && entryPriceFloat > 0 {
		spreadFrac := cost.Spread.Float64() / entryPriceFloat

		if spreadFrac > 0 {
			baseCosts.SpreadFraction = spreadFrac
		}
	}

	return baseCosts
}

/*
feeRate returns the venue taker fee fraction for one symbol, or an error when the
schedule is unavailable. Unknown execution facts are never defaulted to optimistic zeros.
*/
func (planner *Planner) feeRate(symbol string) (float64, error) {
	if planner.desk == nil || planner.desk.Price() == nil {
		return 0, fmt.Errorf("planner: price service required for fee")
	}

	fee := planner.desk.Price().Fee(symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return 0, fmt.Errorf("planner: valid fee required for %s", symbol)
	}

	return fee.Fee.Float64() / 100.0, nil
}

/*
consensusAlternatives projects the deliberated move distribution onto the
decision surface so a round can be read back and argued with.
*/
func consensusAlternatives(consensus *advisor.DeliberationOutcome) map[string]float64 {
	alternatives := make(map[string]float64, len(consensus.Probabilities)+3)

	for move, probability := range consensus.Probabilities {
		alternatives["move:"+move.String()] = probability
	}

	alternatives["consensus:dominant"] = float64(consensus.DominantMove)
	alternatives["consensus:participants"] = float64(consensus.Participants)
	alternatives["consensus:vetoes"] = float64(len(consensus.Vetoes))
	alternatives["consensus:synergies"] = float64(len(consensus.Synergies))

	return alternatives
}

/*
recordSearch attaches the search's economic and causal provenance to the
decision, so a trade can be audited for how much of it was real rollout
evidence and how much was counterfactual.
*/
func (planner *Planner) recordSearch(
	decision *types.Decision,
	result *mcts.SearchResult,
	consensus *advisor.DeliberationOutcome,
) {
	decision.CalibrationCount = uint64(result.Visits)
	decision.Alternatives["search:expected_outcome"] = result.ExpectedEconomicOutcome
	decision.Alternatives["search:outcome_uncertainty"] = result.OutcomeUncertainty
	decision.Alternatives["search:visits"] = float64(result.Visits)

	if result.Trace == nil {
		return
	}

	for _, branch := range result.Trace.Branches {
		name := "branch:" + branch.Action.String()
		decision.Alternatives[name+":visits"] = float64(branch.Visits)
		decision.Alternatives[name+":blended"] = branch.BlendedValue
		decision.Alternatives[name+":mean"] = branch.MeanReward
		decision.Alternatives[name+":counterfactual_mass"] = branch.CounterfactualMass

		if branch.CausalExpectationDefined {
			decision.Alternatives[name+":do_expectation"] = branch.CausalExpectation
		}
	}
}

func (planner *Planner) validateTicker(envelope *types.Envelope) error {
	ticker := envelope.TickerData

	if ticker.Symbol == "" || ticker.Bid == nil || ticker.Ask == nil ||
		ticker.Bid.Sign() <= 0 || ticker.Ask.Sign() <= 0 {
		return fmt.Errorf("ticker symbol and positive bid/ask required")
	}

	if ticker.Timestamp.IsZero() {
		return fmt.Errorf("ticker event time required")
	}

	if envelope.Resonance != nil && envelope.Resonance.Symbol != "" && envelope.Resonance.Symbol != ticker.Symbol {
		return fmt.Errorf("symbol mismatch between ticker (%s) and resonance (%s)", ticker.Symbol, envelope.Resonance.Symbol)
	}

	if planner.lastSequences != nil {
		lastSeq := planner.lastSequences[ticker.Symbol]
		seq := envelope.Stream.Sequence

		if seq == 0 && envelope.Tick > 0 {
			seq = uint64(envelope.Tick)
		}

		if seq > 0 && lastSeq > 0 && seq < lastSeq {
			return fmt.Errorf("envelope sequence regression for %s: %d < %d", ticker.Symbol, seq, lastSeq)
		}

		if seq > 0 {
			planner.lastSequences[ticker.Symbol] = seq
		}
	}

	if planner.lastTimestamps != nil {
		lastTime := planner.lastTimestamps[ticker.Symbol]

		if !lastTime.IsZero() && ticker.Timestamp.Before(lastTime) {
			return fmt.Errorf("envelope timestamp regression for %s: %v < %v", ticker.Symbol, ticker.Timestamp, lastTime)
		}

		planner.lastTimestamps[ticker.Symbol] = ticker.Timestamp
	}

	return nil
}

func (planner *Planner) breakEven(envelope *types.Envelope) (*float64, error) {
	if envelope == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: envelope required to price break-even",
			nil,
		))
	}

	ticker := envelope.TickerData
	symbol := ticker.Symbol

	if planner.desk == nil || planner.desk.Price() == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: desk price service required to price break-even",
			nil,
		))
	}

	fee := planner.desk.Price().Fee(symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: valid fee required to price break-even",
			nil,
		))
	}

	feeRate := fee.Fee.Float64() / 100.0
	exitFactor := 1.0 - feeRate

	if exitFactor <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: fee leaves no realizable proceeds",
			nil,
		))
	}

	ask := ticker.Ask.Float64()
	bid := ticker.Bid.Float64()

	if ask <= 0 || bid <= 0 || ask < bid {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: positive uncrossed quotes required to price break-even",
			nil,
		))
	}

	floorBreakEven := ask * (1.0 + feeRate) / exitFactor
	planner.desk.Price().Update(&ticker)

	if planner.desk.Balance() == nil {
		return &floorBreakEven, nil
	}

	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return &floorBreakEven, nil
	}

	budget := decimal.NewFromInt64(0).Add(cash).Mul(
		decimal.NewFromFloat64(planner.maxAllocationFraction),
	)
	quantity := planner.desk.Price().Quantity(symbol, budget)

	if quantity == nil || quantity.Sign() <= 0 {
		return &floorBreakEven, nil
	}

	executable, err := planner.desk.Price().ExecutableQuantity(symbol, quantity)

	if err != nil || executable == nil || executable.Sign() <= 0 {
		return &floorBreakEven, nil
	}

	cost, err := planner.desk.Price().EntryCost(symbol, executable)

	if err == nil && cost != nil && cost.BreakEven != nil {
		costValue := cost.BreakEven.Float64()

		if costValue > floorBreakEven {
			floorBreakEven = costValue
		}
	}

	return &floorBreakEven, nil
}

/*
execute sizes and dispatches one entry decision. It is the path the causal
search hands its admitted entries to.
*/
func (planner *Planner) execute(
	decision *types.Decision,
	round *types.StrategyRound,
) error {
	if err := planner.allocation.Calculate([]*types.Decision{decision}); err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: allocation failed: " + err.Error()
		round.Outcome = "allocation-failed"

		return err
	}

	if decision.Action != types.ActionEnter {
		round.Outcome = "admission"

		return nil
	}

	if err := planner.desk.Execute(*decision); err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: execution failed: " + err.Error()
		round.Outcome = "execution-failed"

		return err
	}

	round.Outcome = "entry"

	return nil
}

func (planner *Planner) Error() error { return planner.err }

func (planner *Planner) halt(err error) {
	if err == nil || planner.err != nil {
		return
	}

	planner.err = err

	if planner.status != nil {
		planner.status.Transition(runtime.FATAL)
	}

	if planner.cancel != nil {
		planner.cancel()
	}
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func envelopeSymbol(envelope *types.Envelope) string {
	if envelope == nil {
		return ""
	}

	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Symbol
	case types.EnvelopeTrade:
		return envelope.TradeData.Symbol
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Symbol
	case types.EnvelopeFuturesTicker:
		return envelope.FuturesTickerData.Symbol
	case types.EnvelopeFuturesTrade:
		return envelope.FuturesTradeData.Symbol
	default:
		return ""
	}
}
