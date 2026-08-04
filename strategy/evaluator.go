package strategy

import (
	"math"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

/*
candidate is one symbol's tradeable view of the current tick, assembled from
the thesis evidence and the live book at the moment it is judged.

It is deliberately not a stored type. Nothing carries a candidate across ticks
or publishes it, so it holds only what an evaluation reads and nothing that
would have to be kept calibrated or expired.
*/
type candidate struct {
	Symbol         string
	At             time.Time
	ExpectedReturn *decimal.Decimal
	ReferencePrice *decimal.Decimal
	ExpectedFees   *decimal.Decimal
	ExpectedSpread *decimal.Decimal
	ExpectedImpact *decimal.Decimal
	Uncertainty    float64
	Confidence     float64
	Epoch          uint64
}

/*
ExecutableReturn subtracts every modeled execution friction from the expected
market return.
*/
func (row candidate) ExecutableReturn() *decimal.Decimal {
	return row.ExpectedReturn.
		Sub(row.ExpectedFees).
		Sub(row.ExpectedSpread).
		Sub(row.ExpectedImpact)
}

/*
FractionOf expresses one of the candidate's currency amounts as a fraction of
its reference price, which is the scale every threshold and comparison in the
decision path is stated on.

A missing reference price yields zero rather than an error: the candidate could
not have been priced without one, so this is unreachable for a candidate that
was actually built, and a zero fraction is the reading that claims nothing.
*/
func (row candidate) FractionOf(amount *decimal.Decimal) float64 {
	if amount == nil || row.ReferencePrice == nil || row.ReferencePrice.Sign() <= 0 {
		return 0
	}

	return amount.Div(row.ReferencePrice).Float64()
}

/*
RoundTripFraction is what entering and exiting this candidate costs, as a
fraction of its reference price.

Fees are already priced for both crossings when the candidate is built, and a
taker pays the full spread over a round trip, so both enter whole.
*/
func (row candidate) RoundTripFraction() float64 {
	return row.FractionOf(row.ExpectedFees) +
		row.FractionOf(row.ExpectedSpread) +
		row.FractionOf(row.ExpectedImpact)
}

/*
ExecutableFraction is the executable net return as a fraction of the reference
price, which is the unit every utility in the decision path is stated in.

Utility travels from here to the arbiter, where it is compared against an
incumbent's ReturnPct and the fee rate it would cost to unwind. Those are both
dimensionless fractions, so a utility carried in quote currency makes the
comparison decide by the symbol's price: a dollar of edge on a hundred dollar
symbol dwarfs any percentage an incumbent can show, and every challenger wins.
*/
func (row candidate) ExecutableFraction() float64 {
	return row.FractionOf(row.ExecutableReturn())
}

/*
UncertaintyFraction prices the forecast's uncertainty in the same units as the
return it is weighed against.

Uncertainty is the resonance stage's surprise: a dimensionless prediction error
norm of order one. The executable return is a fraction of the reference price,
which for a per-tick forecast is orders of magnitude smaller. Subtracting the one
from the other directly compares a norm to a return, and the norm wins on scale
alone regardless of what either says about the trade, so every candidate is
vetoed and no position is ever opened.

Discounting the return by its own uncertainty keeps the comparison in return
fractions. A forecast the network is confident in is charged little of its edge;
one it is unsure of is charged most of it; and a forecast of nothing is still
worth nothing, because the charge is proportional to the edge being claimed
rather than a flat subtraction from it.
*/
/*
UncertaintyFraction prices the forecast's uncertainty in the same units as the
return it is weighed against.
*/
func (row candidate) UncertaintyFraction() float64 {
	if math.IsNaN(row.Uncertainty) || math.IsInf(row.Uncertainty, 0) || row.Uncertainty <= 0 {
		return 0
	}

	return row.Uncertainty
}

type Evaluator struct {
	mctsEngine *mcts.CausalMCTS
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
}

func NewEvaluator(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
) Evaluator {
	engine := NewCausalEngineAdapter()
	// Configure MCTS: Exploration C = 1.414, CausalAlpha = 0.5
	// Target = 3 (Reward), Treatment = 2 (Action), Controls = [0, 1] (Energy, Surprise)
	mctsSearch := mcts.NewCausalMCTS(
		engine,
		1.414, 0.5,
		12, 2, 3,
		[]int{0, 1}, []int{0, 1, 2},
		false, // Non-linear SCM fit
	)

	return Evaluator{
		mctsEngine: mctsSearch,
		desk:       desk,
		price:      price,
		balance:    balance,
	}
}

func (evaluator Evaluator) EvaluateOpportunities(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	if thesis.Cognition == nil {
		thesis.Cognition = &sync.Map{}
	}

	if thesis.Causal == nil {
		thesis.Causal = &sync.Map{}
	}

	if thesis.Graphs == nil {
		thesis.Graphs = &sync.Map{}
	}

	occupied := getOccupiedSymbols(thesis, evaluator.desk)

	for _, symbol := range thesis.Symbols() {
		if _, blocked := occupied[symbol]; blocked {
			continue
		}

		if isExiting(thesis, symbol) {
			continue
		}

		forecast, ok := evaluator.candidate(thesis, symbol)

		if !ok {
			// Record the skip rather than dropping the symbol silently. A
			// symbol that cannot be priced is a fact about this tick, and
			// leaving no trace of it makes an unpriceable market look
			// identical to one that was never considered.
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionNothing,
				Symbol:           symbol,
				At:               thesis.At,
				Alternatives:     map[string]float64{"enter": 0, "nothing": 0},
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Cause:            "no_forecast",
				Reason:           "no priced forecast available for symbol this tick",
			})

			continue
		}

		// 1. Extract Relational Graph & Causal Rows
		supports, contradicts, hasGraph := inspectGraph(thesis, symbol)
		causalRows := getCausalHistoryRows(thesis, symbol)
		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, symbol)
		cognition := getCognition(thesis, symbol)

		// Compare the two sides as shares of the evidence rather than as raw
		// totals. Edge weights carry whatever units their source node used,
		// so a single contradiction drawn from an unbounded causal score
		// would otherwise outweigh every supporting edge no matter how much
		// support there is.
		if hasGraph && contradicts > 0 &&
			contradicts/(supports+contradicts) > graphContradictionShare {

			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "graph_contradiction",
				"relational graph contradicts trade hypothesis",
			))

			continue
		}

		// 2. Build Root State for MCTS, on fractions of the reference price.
		//
		// The trajectory weighs a forecast against a friction cost and a
		// reversal threshold, so both have to be on the forecast's own scale.
		// Passing currency amounts made the rollout's judgement depend on how
		// expensive the symbol happened to be.
		rootState := StrategyState{
			Symbol:        symbol,
			Energy:        forecast.Uncertainty,
			Surprise:      forecast.FractionOf(forecast.ExpectedImpact),
			Treatment:     forecast.FractionOf(forecast.ExpectedReturn),
			RoundTripCost: forecast.RoundTripFraction(),
			Reward:        0.0,
			Step:          0,
			MaxSteps:      5, // 5-step forward trajectory search
			IsHolding:     false,
		}

		// 3. Run Causal MCTS Search over Trajectory Tree
		var recommendedAction float64
		var mctsErr error

		// The search indexes every row by column, so a row narrower than the
		// state vector is an out-of-range panic rather than a poor
		// recommendation. Row count alone does not establish that: history
		// rows arrive from the causal stage unvalidated and a short or empty
		// row among them passes a count check and then crashes the planner
		// mid-tick.
		searchable := usableCausalRows(causalRows) >= 12

		if searchable {
			recommendedAction, mctsErr = evaluator.mctsEngine.Search(rootState, 50, causalRows)
		}

		// 4. Executable Net Return, weighed by Bayesian precision and corroborating heads.
		utility := unifiedUtility(
			forecast.ExecutableFraction(),
			causalFactor(doExpectation, uplift, noise, causalReady),
			cognitionFactor(cognition),
			graphFactor(supports, contradicts, hasGraph),
			forecast.Uncertainty,
		)

		if utility <= 0 {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, utility, "non_positive_utility",
				"executable utility does not clear trading costs",
			))
			continue
		}

		// If MCTS recommends ActionEnter (1.0) and Net Utility > 0
		/*
			The fallback is gated on the same condition the search was, so the
			two can never disagree about whether a recommendation exists. Gating
			it on the raw row count instead would let a tick with twelve
			unusable rows skip the search and also skip the fallback, deciding
			nothing at all.
		*/
		if (mctsErr == nil && recommendedAction == ActionEnter) || !searchable {
			opportunity := highVelocityOpportunity(thesis, symbol)

			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:            types.ActionEnter,
				Symbol:            symbol,
				At:                forecast.At,
				Utility:           utility,
				Opportunity:       opportunity,
				AllocationHaircut: 0.1,
				AllocationClass:   "normal",
				Alternatives: map[string]float64{
					"enter":   utility,
					"nothing": 0,
				},
				ExpectedFees:      forecast.ExpectedFees,
				ExpectedSpread:    forecast.ExpectedSpread,
				ExpectedReturn:    forecast.ExpectedReturn,
				ExpectedImpact:    forecast.ExpectedImpact,
				AdverseSelection:  adverseSelection(thesis, forecast),
				Uncertainty:       forecast.Uncertainty,
				Confidence:        forecast.Confidence,
				OpportunityMargin: utility,
				CognitiveLead:     cognition.LookaheadScore,
				BasinConfidence:   cognition.Confidence,
				ReferencePrice:    forecast.ReferencePrice.Copy(),
				ValidThroughEpoch: forecast.Epoch,
				ForecastSource:    string(types.SourceResonance),
				ForecastEpoch:     forecast.Epoch,
				Cause:             "causal_mcts_entry",
				Reason:            "causal MCTS search recommended entry trajectory",
			})
			continue
		}

		thesis.Decisions = append(thesis.Decisions, evaluator.reject(
			forecast, utility, "mcts_rejected",
			"causal MCTS trajectory search did not select entry action",
		))
	}
}

func highVelocityOpportunity(thesis *types.Thesis, symbol string) bool {
	if thesis == nil || len(thesis.Categories) == 0 {
		return false
	}

	categories := thesis.Categories[symbol]
	highVelocity := false

	for _, category := range categories {
		switch category.Type {
		case types.CategoryExhaustion,
			types.CategoryFadedExhaustion,
			types.CategoryThermalExhaustion,
			types.CategoryMechanicalCollapse,
			types.CategoryActiveReversal,
			types.CategorySpoofTrap,
			types.CategoryToxicBluff,
			types.CategoryBookThinning:
			return false
		case types.CategoryVerticalIgnition,
			types.CategoryFrenzy,
			types.CategoryAggressiveDrive,
			types.CategoryLiquidityShock,
			types.CategoryLoadedImbalance:
			highVelocity = true
		}
	}

	return highVelocity
}

func getCausalHistoryRows(thesis *types.Thesis, symbol string) [][]float64 {
	if thesis == nil || thesis.Causal == nil {
		return nil
	}
	val, ok := thesis.Causal.Load(symbol)
	if !ok || val == nil {
		return nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil
	}

	if rowsRaw, ok := m["historyRows"].([][]float64); ok {
		return rowsRaw
	}

	return nil
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}

/*
causalRowWidth is how many columns a causal history row must carry to be usable
by the trajectory search.

The search reads a treatment column out of every row it is given, and the state
vector the rows are built against is [energy, surprise, treatment, reward]. A row
with fewer columns than that cannot answer the question the search asks of it.
*/
const causalRowWidth = 4

/*
usableCausalRows counts how many history rows are wide enough for the search to
index safely.

History rows reach the planner from the causal stage without a width contract:
they are whatever that stage last stored, and a stage that produced a partial or
empty reading stores a partial or empty row. Counting rows alone therefore
establishes nothing about whether they can be indexed, and the search panics on
the first short one rather than declining to recommend.

Counting the usable ones instead means a tick whose history is malformed simply
fails the evidence threshold and falls through to the utility path, which is the
same outcome as having too little history — which is what a malformed row
actually represents.
*/
func usableCausalRows(rows [][]float64) int {
	usable := 0

	for _, row := range rows {
		if len(row) >= causalRowWidth {
			usable++
		}
	}

	return usable
}

// ManageContinuation scores continuation (Hold) for active open holdings.
func (evaluator Evaluator) ManageContinuation(
	thesis *types.Thesis,
	desk *broker.Desk,
	price *broker.Price,
) {
	if thesis == nil || desk == nil || price == nil {
		return
	}

	for position := range desk.Positions() {
		if position.Status != types.OPEN {
			continue
		}

		if isExiting(thesis, position.Holding.Symbol) {
			continue
		}

		forecast, found := evaluator.candidate(thesis, position.Holding.Symbol)

		if !found {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionHold,
				Symbol:           position.Holding.Symbol,
				Cause:            "continuation",
				Reason:           "awaiting eligible forecast for continuation scoring",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			continue
		}

		fee, err := evaluator.price.Fee(position.Holding.Symbol)

		if err != nil {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionHold,
				Symbol:           position.Holding.Symbol,
				Cause:            "continuation",
				Reason:           "fee rate unavailable for continuation scoring",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			errnie.Error(errnie.Err(
				errnie.NotFound,
				"continuation: fee rate unavailable for "+position.Holding.Symbol,
				err,
			))

			continue
		}

		// Only the exit remains to be paid on a position already held, so it
		// carries one crossing of the spread and one fee rather than the
		// round trip an entry is charged for.
		//
		// Stated as a fraction of the reference price, because the utility it
		// gates is one: the fee already arrives as a rate, and the two frictions
		// beside it are amounts that have to be divided down to join it.
		exitCostFraction := fee.Float64() +
			forecast.FractionOf(forecast.ExpectedSpread)/2 +
			forecast.FractionOf(forecast.ExpectedImpact)

		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, forecast.Symbol)
		cognition := getCognition(thesis, forecast.Symbol)
		supports, contradicts, hasGraph := inspectGraph(thesis, forecast.Symbol)

		// Scored by the same decision function an entry is, so a position is
		// held for the reasons it would be opened for. The charge here is
		// against the gross return rather than the executable one, because the
		// exit cost this utility is compared to is subtracted separately below.
		holdUtility := unifiedUtility(
			forecast.FractionOf(forecast.ExpectedReturn),
			causalFactor(doExpectation, uplift, noise, causalReady),
			cognitionFactor(cognition),
			graphFactor(supports, contradicts, hasGraph),
			forecast.Uncertainty,
		)

		/*
			Holding is only worth it while continuing beats closing. Once the
			thesis behind a position has decayed past the cost of getting out,
			the position is closed on its own merits rather than being left to
			wait for a better candidate to displace it.
		*/
		action := types.ActionHold
		cause := "continuation"
		reason := "continuation holds active position"
		quantity := decimal.NewFromInt64(0)

		if holdUtility < -exitCostFraction {
			action = types.ActionExit
			cause = "continuation_decayed"
			reason = "holding no longer covers the cost of exiting"
			quantity = position.Holding.Qty.Copy()
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:  action,
			Symbol:  forecast.Symbol,
			At:      forecast.At,
			Utility: holdUtility,
			Alternatives: map[string]float64{
				"hold": holdUtility,
				"exit": -exitCostFraction,
			},
			ProposedNotional:  decimal.NewFromInt64(0),
			ProposedQuantity:  quantity,
			ExpectedReturn:    forecast.ExpectedReturn,
			ExpectedFees:      fee,
			ExpectedSpread:    forecast.ExpectedSpread.Div(decimal.NewFromInt64(2)),
			ExpectedImpact:    forecast.ExpectedImpact,
			AdverseSelection:  adverseSelection(thesis, forecast),
			Uncertainty:       forecast.Uncertainty,
			Confidence:        forecast.Confidence,
			OpportunityMargin: holdUtility + exitCostFraction,
			CognitiveLead:     cognition.LookaheadScore,
			BasinConfidence:   cognition.Confidence,
			ReferencePrice:    forecast.ReferencePrice.Copy(),
			ValidThroughEpoch: forecast.Epoch,
			ForecastSource:    string(types.SourceResonance),
			ForecastEpoch:     forecast.Epoch,
			Cause:             cause,
			Reason:            reason,
		})
	}
}

/*
candidate assembles the per-symbol view an evaluation needs from the evidence
already on the thesis: the resonance forecast for expected return, the live book
for reference price and friction.

Prediction and friction are read at the same instant they are judged, so a
candidate cannot outlive the market that produced it. ok is false whenever any
part is missing, because a partially known candidate is not tradeable.
*/
func (evaluator Evaluator) candidate(
	thesis *types.Thesis,
	symbol string,
) (candidate, bool) {
	if thesis == nil || thesis.Resonance == nil {
		return candidate{}, false
	}

	row, ok := resonanceRow(thesis, symbol)

	if !ok {
		return candidate{}, false
	}

	curve, ok := row["forwardCurve"].([]float64)

	if !ok || len(curve) == 0 {
		return candidate{}, false
	}

	/*
		The edge a position earns is what the forecast accumulates over the
		horizon it is held for, not what it earns on the single tick after
		entry.

		Friction is a round trip: it is paid once on the way in and once on the
		way out, whatever happens in between. Weighing that whole cost against
		one tick of forecast understates every trade by the length of the
		horizon, and on a market whose spread exceeds a single tick's move it
		rejects every candidate regardless of how strong or how sustained the
		move is. Accumulating the curve puts both sides of the comparison over
		the same holding period.

		Each step is discounted by the share of the latent state still surviving
		at that step, which is what the resonance stage publishes alongside the
		curve. Later steps of a rollout are the temporal recursion relaxing
		toward the origin rather than a statement about the market, so counting
		them undiscounted would credit the position with a forecast that was
		never made.
	*/
	expectedReturn := accumulatedReturn(curve, retention(row, len(curve)))
	multiplier := momentumMultiplier(thesis, symbol)

	if multiplier > 1.0 {
		expectedReturn *= multiplier
	}

	if math.IsNaN(expectedReturn) || math.IsInf(expectedReturn, 0) {
		return candidate{}, false
	}

	tick := evaluator.price.Tick(symbol)

	if tick == nil || tick.Bid == nil || tick.Ask == nil {
		return candidate{}, false
	}

	bid, ask := tick.Bid.Float64(), tick.Ask.Float64()

	if bid <= 0 || ask < bid {
		return candidate{}, false
	}

	fee, err := evaluator.price.Fee(symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"friction: fee rate unavailable for "+symbol,
			err,
		))

		return candidate{}, false
	}

	reference := decimal.NewFromFloat64((bid + ask) / 2)

	/*
		A taker crosses half the quoted spread on the way in, and the same
		again on the way out, so a round trip pays the full spread.
	*/
	spread := decimal.NewFromFloat64(ask - bid)

	confidence, _ := loadResonanceFloat(row, "confidence")
	surprise, _ := loadResonanceFloat(row, "surprise")

	/*
		Every friction below is an absolute amount per unit traded, so the fee
		rate has to be priced against the reference before it can be added to
		the others. A taker pays it entering and again exiting.
	*/
	fees := reference.Mul(fee).Mul(decimal.NewFromInt64(2))

	return candidate{
		Symbol:         symbol,
		At:             thesis.At,
		ExpectedReturn: reference.Mul(decimal.NewFromFloat64(expectedReturn)),
		ReferencePrice: reference,
		ExpectedFees:   fees,
		ExpectedSpread: spread,

		/*
			Impact is the share of the touch a fill consumes, which the
			allocator sizes against; until it has sized the order the best
			available estimate is the spread it must cross.
		*/
		ExpectedImpact: spread.Mul(decimal.NewFromFloat64(0.1)),
		Uncertainty:    surprise,
		Confidence:     math.Max(0, math.Min(1, confidence)),

		/*
			The forecast is good for as many ticks ahead as it predicts, so a
			decision drawn from it expires at the end of that horizon. Counting
			from the current tick keeps the epoch in the future even on the
			first tick the desk ever evaluates.
		*/
		Epoch: uint64(thesis.Tick) + horizon(row, len(curve)),
	}, true
}

/*
horizon is how many ticks ahead the resonance forecast claims to see, falling
back to the length of the curve it produced when the solver has not published
an active horizon.
*/
func horizon(row map[string]any, curveLength int) uint64 {
	if raw, found := row["activeHorizon"]; found {
		if active, ok := raw.(int); ok && active > 0 {
			return uint64(active)
		}
	}

	if curveLength > 0 {
		return uint64(curveLength)
	}

	return 1
}

/*
retention is the share of the forecast's latent state still surviving at each
rollout step, as published alongside the curve.

A curve with no envelope is treated as fully retained. The stage publishes both
together, so this is the shape of an older reading rather than a live one, and
the curve is the more conservative thing to trust in that case: the horizon it
was drawn for already bounds how far it runs.
*/
func retention(row map[string]any, curveLength int) []float64 {
	if raw, found := row["forwardRetention"]; found {
		if surviving, ok := raw.([]float64); ok && len(surviving) == curveLength {
			return surviving
		}
	}

	surviving := make([]float64, curveLength)

	for index := range surviving {
		surviving[index] = 1
	}

	return surviving
}

/*
accumulatedReturn is what the forecast earns over the whole horizon, with each
step weighted by how much of the latent state still survives to make it.

Steps are summed rather than compounded. These are log returns, which add over
time by construction, and at the magnitudes a per-tick forecast carries the
difference is immaterial anyway.
*/
func accumulatedReturn(curve []float64, surviving []float64) float64 {
	total := 0.0

	/*
		Weights are relative to the first step, which is where the forecast
		begins. One application of the temporal recursion already removes most
		of the latent magnitude, so absolute retention would discount even the
		first step to a fraction of itself and understate the whole horizon.
	*/
	reference := 0.0

	if len(surviving) > 0 {
		reference = surviving[0]
	}

	for index, step := range curve {
		if math.IsNaN(step) || math.IsInf(step, 0) {
			continue
		}

		weight := 1.0

		if index < len(surviving) && reference > 0 {
			weight = math.Min(1, surviving[index]/reference)
		}

		if weight <= 0 {
			/*
				The state has fully relaxed by this step, so nothing beyond it
				is a forecast and accumulating further would credit the position
				with the decay envelope.
			*/
			break
		}

		total += step * weight
	}

	return total
}

/*
graphContradictionShare is the fraction of a symbol's weighted relational
evidence that must oppose the trade before the hypothesis is abandoned. A
simple majority means the graph is saying more against the trade than for it.
*/
const graphContradictionShare = 0.5

/*
unifiedUtility synthesizes forecast edge, Bayesian precision, and corroboration into one decision metric.

The Bayesian confidence weight (1 / (1 + uncertainty)) dynamically dampens uncertain forecasts
without ever inverting a positive edge into a negative value. Corroboration scales the return
being claimed, so a forecast of nothing stays worth nothing however confident the heads are.
*/
func unifiedUtility(
	executableFraction, causal, cognition, graph, uncertainty float64,
) float64 {
	if math.IsNaN(executableFraction) || math.IsInf(executableFraction, 0) || executableFraction == 0 {
		return 0
	}

	confidenceWeight := 1.0

	if !math.IsNaN(uncertainty) && !math.IsInf(uncertainty, 0) && uncertainty > 0 {
		confidenceWeight = 1.0 / (1.0 + uncertainty)
	}

	utility := executableFraction * confidenceWeight * causal * cognition * graph

	if math.IsNaN(utility) || math.IsInf(utility, 0) {
		return 0
	}

	return utility
}

/*
causalFactor reads the causal head as corroboration of the forecast, on (0, 2).

The three terms are one statement about whether intervening moves the target:
the interventional expectation and the uplift argue for it, and the SCM's own
reconstruction noise argues against trusting either. tanh bounds the sum, so a
head that has not yet found a stable fit cannot swing a trade on scale alone,
and an unmeasured head returns exactly 1 — the forecast, unchanged.
*/
func causalFactor(doExpectation, uplift, noise float64, ready bool) float64 {
	if !ready {
		return 1
	}

	score := doExpectation + uplift - noise

	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 1
	}

	return 1 + math.Tanh(score)
}

/*
cognitionFactor scales the forecast by attractor basin confidence and branch entropy, on (0, 1].

Rather than multiplying by raw multi-step joint transition probabilities (which compound exponentially),
cognition represents regime stability and prior consistency.
*/
func cognitionFactor(cognition types.Cognition) float64 {
	if !cognition.Ready || cognition.LookaheadPaths <= 0 {
		return 1
	}

	if cognition.Confidence > 0 && !math.IsNaN(cognition.Confidence) && !math.IsInf(cognition.Confidence, 0) {
		return math.Max(0.1, math.Min(1.0, cognition.Confidence))
	}

	return 1
}

/*
graphFactor reads the relational graph as consistency with everything else the
market is saying, on [0, 1].

Both sides are weighted shares of the same evidence rather than raw totals,
because edge weights carry whatever units their source node used and one
contradiction drawn from an unbounded causal score would otherwise outweigh
every supporting edge in the graph.
*/
func graphFactor(supports, contradicts float64, hasGraph bool) float64 {
	total := supports + contradicts

	if !hasGraph || contradicts <= 0 || total <= 0 {
		return 1
	}

	return 1 - math.Min(1, contradicts/total)
}

/*
adverseSelection prices the risk of being filled by someone better informed.

The causal head estimates how much of the flow is informed, and being on the
wrong side of informed flow costs the spread, so the expected loss is that
share of the spread. Absent a causal reading the charge is zero rather than a
guess, which keeps an unmeasured cost from silently vetoing trades.
*/
func adverseSelection(thesis *types.Thesis, row candidate) *decimal.Decimal {
	informed := 0.0

	if thesis != nil && thesis.Causal != nil {
		if raw, found := thesis.Causal.Load(row.Symbol); found {
			if metrics, ok := raw.(map[string]any); ok {
				informed = getFloat(metrics, "informedFlow")
			}
		}
	}

	if math.IsNaN(informed) || math.IsInf(informed, 0) || informed <= 0 {
		return decimal.NewFromInt64(0)
	}

	return row.ExpectedSpread.Mul(
		decimal.NewFromFloat64(math.Min(1, informed)),
	)
}

/*
loadResonanceFloat reads one of the resonance solver's flat scalar keys.
*/
func loadResonanceFloat(row map[string]any, key string) (float64, bool) {
	raw, found := row[key]

	if !found {
		return 0, false
	}

	value, ok := raw.(float64)

	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	return value, true
}

func resonanceRow(thesis *types.Thesis, symbol string) (map[string]any, bool) {
	if thesis == nil || thesis.Resonance == nil || symbol == "" {
		return nil, false
	}

	raw, found := thesis.Resonance.Load(symbol)

	if !found {
		return nil, false
	}

	row, ok := raw.(map[string]any)

	if !ok {
		return nil, false
	}

	return row, true
}

func (e Evaluator) reject(
	forecast candidate,
	utility float64,
	cause, reason string,
) types.Decision {
	return types.Decision{
		Action:            types.ActionNothing,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
		ReferencePrice:    forecast.ReferencePrice.Copy(),
		ValidThroughEpoch: forecast.Epoch,
		ForecastSource:    string(types.SourceResonance),
		ForecastEpoch:     forecast.Epoch,
		ExpectedReturn:    forecast.ExpectedReturn,
		ExpectedFees:      forecast.ExpectedFees,
		ExpectedSpread:    forecast.ExpectedSpread,
		ExpectedImpact:    forecast.ExpectedImpact,
		Uncertainty:       forecast.Uncertainty,
		Confidence:        forecast.Confidence,
		OpportunityMargin: utility,
		Cause:             cause,
		Reason:            reason,
	}
}

// Helper Functions

func getCausalMetrics(thesis *types.Thesis, symbol string) (doExp, uplift, noise float64, ready bool) {
	if thesis == nil || thesis.Causal == nil {
		return 0, 0, 0, false
	}

	val, ok := thesis.Causal.Load(symbol)

	if !ok || val == nil {
		return 0, 0, 0, false
	}

	m, ok := val.(map[string]any)

	if !ok {
		return 0, 0, 0, false
	}

	getFloat := func(key string) float64 {
		if v, ok := m[key].(float64); ok {
			return v
		}
		return 0.0
	}

	doExp = getFloat("doExpectation")
	uplift = getFloat("uplift")
	noise = getFloat("noise")
	strength := getFloat("strength")
	confidence := getFloat("confidence")

	ready = (strength > 0 || confidence > 0) && (uplift != 0 || doExp != 0)
	return doExp, uplift, noise, ready
}

func getCognition(thesis *types.Thesis, symbol string) types.Cognition {
	val, ok := thesis.Cognition.Load(symbol)

	if !ok || val == nil {
		return types.Cognition{}
	}

	if cog, ok := val.(types.Cognition); ok {
		return cog
	}

	return types.Cognition{}
}

func inspectGraph(thesis *types.Thesis, symbol string) (supports, contradicts float64, hasGraph bool) {
	val, ok := thesis.Graphs.Load("market_graph")

	if !ok || val == nil {
		return 0, 0, false
	}

	g, ok := val.(*graph.Graph)

	if !ok || g == nil {
		return 0, 0, false
	}

	for _, edge := range g.Edges {
		fromNode, fromOk := g.Nodes[edge.From]
		toNode, toOk := g.Nodes[edge.To]

		if (fromOk && fromNode.Symbol == symbol) || (toOk && toNode.Symbol == symbol) {
			/*
				Weights arrive in whatever units their relation is drawn from:
				an agreement strength, an interventional level, an age in
				seconds. Counting how much evidence points each way means
				reading each edge as one bounded, non-negative vote, or a
				single stale timestamp measured in seconds would outvote every
				other relation in the graph.
			*/
			vote := math.Tanh(math.Abs(edge.Weight)) * math.Abs(edge.Confidence)

			switch edge.Relation {
			case graph.RelationSupports,
				graph.RelationConditions,
				graph.RelationLeads:
				supports += vote
			case graph.RelationContradicts:
				contradicts += vote
			case graph.RelationStaleRelativeTo,
				graph.RelationIncomparableWith:
				// Temporal dispersion / neutral edge: neither direct support nor active contradiction
			}
		}
	}

	return supports, contradicts, true
}

func getMetricValue(measurement *types.Measurement, metricKey string) float64 {
	if measurement == nil || measurement.Metrics == nil {
		return 0
	}

	sample, ok := measurement.Metrics[metricKey]

	if !ok {
		return 0
	}

	if sample.Normalized != nil && !math.IsNaN(*sample.Normalized) && !math.IsInf(*sample.Normalized, 0) {
		return *sample.Normalized
	}

	if !math.IsNaN(sample.Raw) && !math.IsInf(sample.Raw, 0) {
		return sample.Raw
	}

	return 0
}

func momentumMultiplier(thesis *types.Thesis, symbol string) float64 {
	if thesis == nil {
		return 1.0
	}

	hawkesFactor := 1.0
	pumpFactor := 1.0
	flowFactor := 1.0
	thinFactor := 1.0
	sentimentFactor := 1.0
	exhaustionPenalty := 0.0

	if thesis.Measurements != nil {
		thesis.Measurements.Range(func(key, value any) bool {
			rows, ok := value.([]*types.Measurement)

			if !ok {
				measurement, measurementOK := value.(*types.Measurement)

				if !measurementOK || measurement == nil {
					return true
				}

				rows = []*types.Measurement{measurement}
			}

			for _, measurement := range rows {
				if measurement == nil || measurement.Symbol != symbol {
					continue
				}

				switch measurement.Source {
				case types.SourcePumpDump:
					ignition := getMetricValue(measurement, string(types.MetricKey(types.MetricIgnition, types.SideNone)))
					trend := getMetricValue(measurement, string(types.MetricKey(types.MetricTrend, types.SideNone)))
					rvol := getMetricValue(measurement, string(types.MetricKey(types.MetricRVOL, types.SideNone)))
					compression := getMetricValue(measurement, string(types.MetricKey(types.MetricCompression, types.SideNone)))
					exhaustion := getMetricValue(measurement, string(types.MetricKey(types.MetricExhaustion, types.SideNone)))

					rvolTerm := 0.0

					if rvol > 1.0 {
						rvolTerm = math.Log(rvol)
					}

					pumpVelocity := 1.0 + ignition*(1.0+rvolTerm) + trend + compression

					if pumpVelocity > pumpFactor {
						pumpFactor = pumpVelocity
					}

					if exhaustion > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, exhaustion)
					}

				case types.SourceHawkes:
					descendants := getMetricValue(measurement, string(types.MetricKey(types.MetricTotalDescendants, types.SideBuy)))
					offspring := getMetricValue(measurement, string(types.MetricKey(types.MetricImmediateOffspring, types.SideBuy)))
					intensity := getMetricValue(measurement, string(types.MetricKey(types.MetricConditionalIntensity, types.SideBuy)))
					baseline := getMetricValue(measurement, string(types.MetricKey(types.MetricBaselineIntensity, types.SideBuy)))

					hawkesCascade := 1.0

					if descendants > 1.0 {
						hawkesCascade = descendants
					}

					if descendants <= 1.0 && offspring > 0 && offspring < 1.0 {
						hawkesCascade = 1.0 / (1.0 - offspring)
					}

					if baseline > 0 && intensity > baseline {
						hawkesCascade *= (intensity / baseline)
					}

					if hawkesCascade > hawkesFactor {
						hawkesFactor = hawkesCascade
					}

				case types.SourceCVD:
					drive := getMetricValue(measurement, string(types.MetricKey(types.MetricDrive, types.SideBuy)))
					absorption := getMetricValue(measurement, string(types.MetricKey(types.MetricAbsorption, types.SideNone)))

					flowFactor = math.Max(flowFactor, 1.0+drive)

					if absorption > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, absorption*0.5)
					}

				case types.SourceSentiment:
					surge := getMetricValue(measurement, string(types.MetricKey(types.MetricSurgeScore, types.SideNone)))
					lead := getMetricValue(measurement, string(types.MetricKey(types.MetricLeaderStrength, types.SideNone)))

					if surge > 0 || lead > 0 {
						sentimentFactor = math.Max(sentimentFactor, 1.0+surge+lead)
					}

				case types.SourceDepthFlow:
					thin := getMetricValue(measurement, string(types.MetricKey(types.MetricThinScore, types.SideNone)))
					spoof := getMetricValue(measurement, string(types.MetricKey(types.MetricSpoofScore, types.SideNone)))

					if thin > 0 {
						thinFactor = math.Max(thinFactor, 1.0+thin)
					}

					if spoof > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, spoof)
					}

				case types.SourceLiquidity:
					scarcity := getMetricValue(measurement, string(types.MetricKey(types.MetricScarcityScore, types.SideNone)))

					if scarcity > 0 {
						thinFactor = math.Max(thinFactor, 1.0+scarcity)
					}
				}
			}

			return true
		})
	}

	categoryFactor := 1.0

	if len(thesis.Categories) > 0 {
		for _, category := range thesis.Categories[symbol] {
			boost := category.Strength * category.Confidence * (1.0 - math.Min(1.0, category.Surprisal))

			switch category.Type {
			case types.CategoryVerticalIgnition,
				types.CategoryFrenzy,
				types.CategoryAggressiveDrive,
				types.CategoryLiquidityShock,
				types.CategoryLoadedImbalance,
				types.CategoryCoiledCompression,
				types.CategoryRiskOnSurge,
				types.CategoryDecoupledAlpha,
				types.CategoryLaminarResonance:
				if boost > 0 {
					categoryFactor += boost
				}

			case types.CategoryExhaustion,
				types.CategoryFadedExhaustion,
				types.CategoryThermalExhaustion,
				types.CategoryMechanicalCollapse,
				types.CategoryActiveReversal,
				types.CategorySpoofTrap,
				types.CategoryToxicBluff:
				penalty := category.Strength * category.Confidence
				exhaustionPenalty = math.Max(exhaustionPenalty, penalty)
			}
		}
	}

	survivalFactor := math.Max(0.0, 1.0-exhaustionPenalty)
	multiplier := hawkesFactor * pumpFactor * flowFactor * thinFactor * sentimentFactor * categoryFactor * survivalFactor

	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return 0.0
	}

	return multiplier
}

func getOccupiedSymbols(thesis *types.Thesis, desk *broker.Desk) map[string]struct{} {
	occupied := make(map[string]struct{})

	if thesis == nil || desk == nil {
		return occupied
	}

	for position := range desk.Positions() {
		if position.Status != types.CLOSED {
			occupied[position.Holding.Symbol] = struct{}{}
		}
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		if symbol, ok := key.(string); ok {
			if state, isStr := value.(string); isStr {
				switch state {
				case types.LifecycleEntrySelected,
					types.LifecycleEntrySubmitted,
					types.LifecyclePartiallyEntered,
					types.LifecycleManaging:
					occupied[symbol] = struct{}{}
				}
			}
		}

		return true
	})

	return occupied
}

func isExiting(thesis *types.Thesis, symbol string) bool {
	if val, ok := thesis.Lifecycle.Load(symbol); ok {
		if state, isStr := val.(string); isStr {
			return state == types.LifecycleExitSelected || state == types.LifecycleExitSubmitted
		}
	}

	return false
}
