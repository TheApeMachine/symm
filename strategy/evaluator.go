package strategy

import (
	"math"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

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
	mctsSearch := mcts.NewCausalMCTS(
		engine,
		1.414, 0.5,
		12, 2, 3,
		[]int{0, 1}, []int{0, 1, 2},
		false,
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

	if thesis.Lifecycle == nil {
		thesis.Lifecycle = &sync.Map{}
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

		supports, contradicts, hasGraph := inspectGraph(thesis, symbol)
		causalRows := getCausalHistoryRows(thesis, symbol)
		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, symbol)
		cognition := getCognition(thesis, symbol)

		if hasGraph && contradicts > 0 &&
			contradicts/(supports+contradicts) > graphContradictionShare {

			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "graph_contradiction",
				"relational graph contradicts trade hypothesis",
			))

			continue
		}

		rootState := StrategyState{
			Symbol:        symbol,
			Energy:        forecast.Uncertainty,
			Surprise:      forecast.FractionOf(forecast.ExpectedImpact),
			Treatment:     forecast.FractionOf(forecast.ExpectedReturn),
			RoundTripCost: forecast.RoundTripFraction(),
			Reward:        0.0,
			Step:          0,
			MaxSteps:      5,
			IsHolding:     false,
		}

		var recommendedAction float64
		var mctsErr error

		searchable := usableCausalRows(causalRows) >= 12

		if searchable {
			recommendedAction, mctsErr = evaluator.mctsEngine.Search(rootState, 50, causalRows)
		}

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

		fee, err := price.Fee(position.Holding.Symbol)

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

			continue
		}

		exitCostFraction := fee.Float64() +
			forecast.FractionOf(forecast.ExpectedSpread)/2 +
			forecast.FractionOf(forecast.ExpectedImpact)

		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, forecast.Symbol)
		cognition := getCognition(thesis, forecast.Symbol)
		supports, contradicts, hasGraph := inspectGraph(thesis, forecast.Symbol)

		holdUtility := unifiedUtility(
			forecast.FractionOf(forecast.ExpectedReturn),
			causalFactor(doExpectation, uplift, noise, causalReady),
			cognitionFactor(cognition),
			graphFactor(supports, contradicts, hasGraph),
			forecast.Uncertainty,
		)

		action := types.ActionHold
		cause := "continuation"
		reason := "continuation holds active position"
		quantity := decimal.NewFromInt64(0)

		hasStructuralReversal := false

		if thesis.Categories != nil {
			for _, category := range thesis.Categories[position.Holding.Symbol] {
				switch category.Type {
				case types.CategoryActiveReversal,
					types.CategoryMechanicalCollapse,
					types.CategoryToxicBluff,
					types.CategoryExhaustion:
					hasStructuralReversal = true
				}
			}
		}

		exitThreshold := exitCostFraction * 1.5

		if hasStructuralReversal {
			exitThreshold = exitCostFraction
		}

		if holdUtility < -exitThreshold {
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

func (evaluator Evaluator) candidate(
	thesis *types.Thesis,
	symbol string,
) (candidate, bool) {
	if thesis == nil || thesis.Resonance == nil || evaluator.price == nil {
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

	logReturn := accumulatedReturn(curve, retention(row, len(curve)))
	boost, survival := momentumTerms(thesis, symbol)

	if logReturn > 0 {
		logReturn *= boost
	}

	logReturn *= survival
	expectedReturn := math.Expm1(logReturn)

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
		return candidate{}, false
	}

	reference := decimal.NewFromFloat64((bid + ask) / 2)
	spread := decimal.NewFromFloat64(ask - bid)
	impact := estimateImpact(thesis, symbol, spread)

	confidence, _ := loadResonanceFloat(row, "confidence")
	surprise, _ := loadResonanceFloat(row, "surprise")
	fees := reference.SetScale(8).Mul(fee.SetScale(8)).Mul(decimal.NewFromInt64(2))

	return candidate{
		Symbol:         symbol,
		At:             thesis.At,
		ExpectedReturn: reference.SetScale(8).Mul(decimal.NewFromFloat64(expectedReturn)),
		ReferencePrice: reference,
		ExpectedFees:   fees,
		ExpectedSpread: spread,
		ExpectedImpact: impact,
		Uncertainty:    surprise,
		Confidence:     math.Max(0, math.Min(1, confidence)),
		Epoch:          uint64(thesis.Tick) + horizon(row, len(curve)),
	}, true
}

func (evaluator Evaluator) reject(
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
