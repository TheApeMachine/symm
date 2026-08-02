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
			/*
				Record the skip rather than dropping the symbol silently. A
				symbol that cannot be priced is a fact about this tick, and
				leaving no trace of it makes an unpriceable market look
				identical to one that was never considered.
			*/
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

		/*
			Compare the two sides as shares of the evidence rather than as raw
			totals. Edge weights carry whatever units their source node used,
			so a single contradiction drawn from an unbounded causal score
			would otherwise outweigh every supporting edge no matter how much
			support there is.
		*/
		if hasGraph && contradicts > 0 &&
			contradicts/(supports+contradicts) > graphContradictionShare {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "graph_contradiction",
				"relational graph contradicts trade hypothesis",
			))
			continue
		}

		// 2. Build Root State for MCTS
		rootState := StrategyState{
			Symbol:    symbol,
			Energy:    forecast.Uncertainty,
			Surprise:  forecast.ExpectedImpact.Float64(),
			Treatment: forecast.ExpectedReturn.Float64(),
			Reward:    0.0,
			Step:      0,
			MaxSteps:  5, // 5-step forward trajectory search
			IsHolding: false,
		}

		// 3. Run Causal MCTS Search over Trajectory Tree
		var recommendedAction float64
		var mctsErr error

		if len(causalRows) >= 12 {
			recommendedAction, mctsErr = evaluator.mctsEngine.Search(rootState, 50, causalRows)
		}

		// 4. Executable Net Return
		execReturn := forecast.ExecutableReturn()
		utility := execReturn.Float64() - forecast.Uncertainty

		/*
			The causal and cognitive heads corroborate a forecast; they do not
			replace it. Each scores in its own unbounded units, so a raw score
			added to a utility measured in currency would decide every trade by
			itself. Scaling a bounded opinion by the return being judged keeps
			the heads able to strengthen, weaken, or reverse that return while
			leaving a forecast of nothing worth nothing.
		*/
		conviction := math.Abs(execReturn.Float64())

		if causalReady {
			utility += conviction * squash(
				doExpectation+uplift-noise,
				math.Abs(doExpectation)+math.Abs(uplift)+math.Abs(noise),
			)
		}

		if cognition.Ready {
			utility += conviction * squash(
				cognition.LookaheadScore*cognition.Confidence,
				math.Abs(cognition.LookaheadScore),
			)
		}

		// If MCTS recommends ActionEnter (1.0) and Net Utility > 0
		if (mctsErr == nil && recommendedAction == ActionEnter) || (len(causalRows) < 12 && utility > 0) {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:            types.ActionEnter,
				Symbol:            symbol,
				At:                forecast.At,
				Utility:           utility,
				Opportunity:       recommendedAction == ActionEnter,
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

	// Reconstruct history rows if stored
	if rowsRaw, ok := m["historyRows"].([][]float64); ok {
		return rowsRaw
	}

	// Single row fallback
	row := []float64{
		getFloat(m, "energy"),
		getFloat(m, "surprise"),
		getFloat(m, "intervention"),
		getFloat(m, "doExpectation"),
	}
	return [][]float64{row}
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
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

		holdUtility := forecast.ExpectedReturn.Float64() - forecast.Uncertainty

		/*
			Only the exit remains to be paid on a position already held, so it
			carries one crossing of the spread and one fee rather than the
			round trip an entry is charged for.
		*/
		exitCost := forecast.ReferencePrice.Mul(fee).Add(
			forecast.ExpectedSpread.Div(decimal.NewFromInt64(2)),
		).Add(forecast.ExpectedImpact)

		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, forecast.Symbol)
		cognition := getCognition(thesis, forecast.Symbol)

		conviction := math.Abs(forecast.ExpectedReturn.Float64())

		if causalReady {
			holdUtility += conviction * squash(
				doExpectation+uplift-noise,
				math.Abs(doExpectation)+math.Abs(uplift)+math.Abs(noise),
			)
		}

		if cognition.Ready {
			holdUtility += conviction * squash(
				cognition.LookaheadScore*cognition.Confidence,
				math.Abs(cognition.LookaheadScore),
			)
		}

		// Apply Graph Contradiction Penalty to Hold Utility
		_, contradicts, hasGraph := inspectGraph(thesis, forecast.Symbol)

		if hasGraph && contradicts > 0 {
			holdUtility -= (contradicts * 0.1)
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:  types.ActionHold,
			Symbol:  forecast.Symbol,
			At:      forecast.At,
			Utility: holdUtility,
			Alternatives: map[string]float64{
				"hold": holdUtility,
				"exit": -exitCost.Float64(),
			},
			ProposedNotional:  decimal.NewFromInt64(0),
			ProposedQuantity:  decimal.NewFromInt64(0),
			ExpectedReturn:    forecast.ExpectedReturn,
			ExpectedFees:      fee,
			ExpectedSpread:    forecast.ExpectedSpread.Div(decimal.NewFromInt64(2)),
			ExpectedImpact:    forecast.ExpectedImpact,
			AdverseSelection:  adverseSelection(thesis, forecast),
			Uncertainty:       forecast.Uncertainty,
			Confidence:        forecast.Confidence,
			OpportunityMargin: holdUtility + exitCost.Float64(),
			CognitiveLead:     cognition.LookaheadScore,
			BasinConfidence:   cognition.Confidence,
			ReferencePrice:    forecast.ReferencePrice.Copy(),
			ValidThroughEpoch: forecast.Epoch,
			ForecastSource:    string(types.SourceResonance),
			ForecastEpoch:     forecast.Epoch,
			Cause:             "continuation",
			Reason:            "continuation holds active position",
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

	curveRaw, found := thesis.Resonance.Load("forwardCurve")

	if !found {
		return candidate{}, false
	}

	curve, ok := curveRaw.([]float64)

	if !ok || len(curve) == 0 {
		return candidate{}, false
	}

	expectedReturn := curve[0]

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

	confidence, _ := loadResonanceFloat(thesis, "confidence")
	surprise, _ := loadResonanceFloat(thesis, "surprise")

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
		Epoch: uint64(thesis.Tick) + horizon(thesis, len(curve)),
	}, true
}

/*
horizon is how many ticks ahead the resonance forecast claims to see, falling
back to the length of the curve it produced when the solver has not published
an active horizon.
*/
func horizon(thesis *types.Thesis, curveLength int) uint64 {
	if raw, found := thesis.Resonance.Load("activeHorizon"); found {
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
graphContradictionShare is the fraction of a symbol's weighted relational
evidence that must oppose the trade before the hypothesis is abandoned. A
simple majority means the graph is saying more against the trade than for it.
*/
const graphContradictionShare = 0.5

/*
squash maps an unbounded score onto (-1, 1) so it can be read as a fractional
adjustment to an expected return.

The score is divided by its own magnitude before the curve is applied, because
the heads carry no fixed scale and any constant chosen here would be wrong the
moment they drift: too large and every opinion rounds to nothing, too small and
tanh saturates into a sign function that ignores how strongly the head actually
argued. Dividing by the terms that formed the score keeps the ratio between
them, which is the part that carries meaning.
*/
func squash(score, magnitude float64) float64 {
	if math.IsNaN(score) || math.IsNaN(magnitude) || magnitude <= 0 {
		return 0
	}

	return math.Tanh(score / magnitude)
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
func loadResonanceFloat(thesis *types.Thesis, key string) (float64, bool) {
	if thesis == nil || thesis.Resonance == nil {
		return 0, false
	}

	raw, found := thesis.Resonance.Load(key)

	if !found {
		return 0, false
	}

	value, ok := raw.(float64)

	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	return value, true
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
			case graph.RelationContradicts,
				graph.RelationStaleRelativeTo,
				graph.RelationIncomparableWith:
				contradicts += vote
			}
		}
	}

	return supports, contradicts, true
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
