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
		searchIterations, searchExploration, searchUncertainty, time.Now().UnixNano(),
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
	}, nil
}

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

	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	if err := planner.validateTicker(envelope); err != nil {
		planner.halt(errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "planner: validate ticker", err,
		)))

		return nil
	}

	if planner.desk.Holding(envelope.TickerData.Symbol) > 0 {
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
	opportunity := planner.opportunity(envelope)

	decision := types.NewDecision(types.ActionNothing, ticker.Symbol)
	decision.At = ticker.Timestamp
	decision.Direction = 1
	decision.ForecastSource = "war-room-deliberation"
	decision.ForecastModel = "causal-mcts-pearl-v1"
	decision.ForecastHorizon = searchHorizon
	decision.AllocationClass = "none"
	decision.Cause = "opportunity-conditioned market context"
	decision.Opportunity = opportunity.Archetype != ""
	decision.OpportunityType = string(opportunity.Archetype)
	decision.OpportunityPhase = string(opportunity.Phase)

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
		// Advisors reach the council only once one of their falsifiable
		// predictions survives a full round on the volume-bar clock, so an
		// empty council on a thin symbol is expected early rather than
		// broken. Naming that explicitly keeps it from reading as a fault.
		decision.PredictiveStatus = "awaiting-advisor-consensus"
		decision.Reason = "planner: no advisor prediction has survived a round for this symbol yet"

		return round
	}

	// The precursor rule: position while armed, never once ignition prints.
	admissible := entryAdmissible(opportunity)

	if !admissible {
		decision.PredictiveStatus = "precursor-not-armed"
		decision.Reason = "planner: entry requires an armed long precursor, phase is " +
			string(opportunity.Phase)
	}

	// Resonance is preferred when it has calibrated: it is a measured forecast
	// rather than a regime prior. When it has not — a cold start, a newly
	// active symbol, or a held call — the council's own distribution supplies
	// the transition model instead of aborting. Refusing to plan there would
	// blind the system precisely when a pump is most likely and least modeled.
	model, ready := newResonanceMarketModel(envelope.Resonance, tickerCadence)
	transitionSource := "resonance-forecast"

	if !ready {
		model, ready = newConsensusMarketModel(consensus, tickerCadence)
		transitionSource = "war-room-consensus"
	}

	if !ready {
		decision.PredictiveStatus = "no-transition-model"
		decision.Reason = "planner: neither resonance nor consensus could model the transition"

		return round
	}

	decision.ForecastSource = transitionSource

	state, built := planner.rootState(envelope, model)

	if !built {
		decision.PredictiveStatus = "portfolio-state-unavailable"
		decision.Reason = "planner: cannot price the portfolio to open a search"

		return round
	}

	result := planner.search.Run(state, &opportunityEstimator{
		consensus:       consensus,
		opportunity:     opportunity,
		entryAdmissible: admissible,
	})

	planner.recordSearch(decision, result, consensus)
	decision.Trace = buildTrace(consensus, result, transitionSource)

	if result.DecisionUnavailable {
		decision.PredictiveStatus = "decision-unavailable"
		decision.Reason = "planner: no feasible action had an estimable causal outcome (" +
			result.IdentificationStatus.String() + ")"

		return round
	}

	decision.PredictiveReady = true
	decision.PredictiveStatus = "causal-search-resolved"

	if result.SelectedAction != mcts.Enter {
		decision.Reason = "planner: causal search selected " +
			result.SelectedAction.String() + " over entering"

		return round
	}

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

	if planner.desk == nil || planner.desk.Balance() == nil {
		return nil, false
	}

	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return nil, false
	}

	mark := ticker.Bid.Float64()

	if mark <= 0 {
		return nil, false
	}

	budget := cash.Float64() * planner.maxAllocationFraction
	unit := budget / mark

	if unit <= 0 {
		return nil, false
	}

	costs := mcts.CostModel{FeeRate: planner.feeRate(ticker.Symbol)}

	if spread := ticker.Ask.Float64() - mark; spread > 0 {
		costs.SpreadFraction = spread / mark / 2
	}

	return mcts.NewEconomicState(
		mcts.PortfolioState{
			Cash: cash.Float64(),
			// Flat by construction: Step returns early when this symbol is
			// already held, so the search always opens from no exposure.
			// Desk.Holding counts open positions, not base quantity, and
			// PortfolioState.Position is explicitly a base quantity — the
			// two must never be conflated.
			Position:  0,
			MarkPrice: mark,
		},
		mcts.MarketState{At: ticker.Timestamp},
		model,
		costs,
		unit,
		unit,
		searchHorizon,
	), true
}

/*
feeRate returns the venue taker fee fraction for one symbol, or zero when the
schedule is unavailable. A missing fee is never treated as a discount: the
caller's break-even pricing carries the authoritative figure.
*/
func (planner *Planner) feeRate(symbol string) float64 {
	if planner.desk == nil || planner.desk.Price() == nil {
		return 0
	}

	fee := planner.desk.Price().Fee(symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return 0
	}

	return fee.Fee.Float64() / 100.0
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

/*
opportunity returns the tracked candidate for the envelope's symbol, or a zero
candidate when none is active.
*/
func (planner *Planner) opportunity(envelope *types.Envelope) types.OpportunityCandidate {
	symbol := envelope.TickerData.Symbol

	for _, candidate := range envelope.Opportunities {
		if candidate != nil && candidate.Symbol == symbol {
			return *candidate
		}
	}

	return types.OpportunityCandidate{}
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
