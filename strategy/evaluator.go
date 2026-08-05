package strategy

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

type Evaluator struct {
	mctsEngine *mcts.CausalMCTS
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	/*
		passage estimates which boundary an open lot reaches first, and episodes
		holds the lots still on their way to one. Both are reference types on a
		value receiver, so every copy of the Evaluator shares the one model that
		is learning.
	*/
	passage  *types.PassageModel
	episodes map[string]*passageEpisode
	// recorder receives one durable record per finished lot, which is the
	// corpus the calibrated replacements for RiskMultiples and the bucket model
	// will eventually be fitted from.
	recorder *audit.Recorder
}

func NewEvaluator(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	recorder *audit.Recorder,
) Evaluator {
	engine := NewCausalEngineAdapter()
	mctsSearch := mcts.NewCausalMCTS(
		engine,
		1.414, 0.5,
		mctsMinimumCausalRows, 2, 3,
		[]int{0, 1}, []int{0, 1, 2},
		false,
	)

	return Evaluator{
		mctsEngine: mctsSearch,
		desk:       desk,
		price:      price,
		balance:    balance,
		passage:    types.NewPassageModel(),
		episodes:   map[string]*passageEpisode{},
		recorder:   recorder,
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
		usableRows := usableCausalRows(causalRows)
		holdDiscount, discountReady := exhaustionHoldDiscount(thesis, symbol)
		spectralRadius, propagationReady := hawkesSpectralRadius(thesis, symbol)
		firstStep, firstStepReady := forecast.Forecast.Step(0)

		if !firstStepReady {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "forecast_curve_unavailable",
				"confidence-supported forecast has no first step",
				nil,
			))

			continue
		}

		rootState := StrategyState{
			Symbol:               symbol,
			Energy:               forecast.Uncertainty,
			Surprise:             forecast.FractionOf(forecast.ExpectedImpact),
			Treatment:            firstStep,
			Forecast:             forecast.Forecast,
			RoundTripCost:        forecast.RoundTripFraction(),
			HoldDiscount:         holdDiscount,
			HawkesSpectralRadius: spectralRadius,
			Reward:               0.0,
			Step:                 0,
			IsHolding:            false,
		}

		causalWeight := causalFactor(doExpectation, uplift, noise, causalReady)
		cognitionWeight := cognitionFactor(cognition)
		graphWeight := graphFactor(supports, contradicts, hasGraph)
		trace := &types.DecisionTrace{
			GraphSupports:    supports,
			GraphContradicts: contradicts,
			Utility: types.DecisionUtilityTrace{
				ExecutableFraction: forecast.ExecutableFraction(),
				UncertaintyWeight:  uncertaintyWeight(forecast.Uncertainty),
				CausalFactor:       causalWeight,
				CognitionFactor:    cognitionWeight,
				GraphFactor:        graphWeight,
			},
			MCTS: rootState.Trace(
				usableRows,
				evaluator.mctsEngine.MinRows,
				mctsSearchIterations,
			),
		}

		if trace.MCTS.Searchable && !discountReady {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "hold_discount_unavailable",
				"valid long-side exhaustion decay is required for trajectory search",
				trace,
			))

			continue
		}

		if trace.MCTS.Searchable && !propagationReady {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "hawkes_propagation_unavailable",
				"a valid fitted Hawkes spectral radius is required for trajectory search",
				trace,
			))

			continue
		}

		if hasGraph && contradicts > 0 &&
			contradicts/(supports+contradicts) > graphContradictionShare {

			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "graph_contradiction",
				"relational graph contradicts trade hypothesis",
				trace,
			))

			continue
		}

		var recommendedAction float64
		var mctsErr error

		searchable := trace.MCTS.Searchable

		if searchable {
			trace.MCTS.Attempted = true
			recommendedAction, mctsErr = evaluator.mctsEngine.Search(
				rootState,
				mctsSearchIterations,
				causalRows,
			)
			if mctsErr != nil {
				trace.MCTS.Error = mctsErr.Error()
			}

			if mctsErr == nil {
				trace.MCTS.RecommendedAction = strategyAction(recommendedAction)
			}
		}

		utility := unifiedUtility(
			trace.Utility.ExecutableFraction,
			trace.Utility.CausalFactor,
			trace.Utility.CognitionFactor,
			trace.Utility.GraphFactor,
			forecast.Uncertainty,
		)

		if utility <= 0 {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, utility, "non_positive_utility",
				"executable utility does not clear trading costs",
				trace,
			))

			continue
		}

		if (mctsErr == nil && recommendedAction == ActionEnter) || !searchable {
			adverse := adverseSelection(thesis, forecast)
			haircut, haircutReason, haircutReady := allocationHaircut(
				thesis, forecast, adverse,
			)

			if !haircutReady {
				thesis.Decisions = append(thesis.Decisions, evaluator.reject(
					forecast, utility, "allocation_evidence_unavailable",
					"valid toxicity and executable-liquidity evidence is required for sizing",
					trace,
				))

				continue
			}

			opportunity := highVelocityOpportunity(thesis, symbol)
			cause := "causal_mcts_entry"
			reason := "causal MCTS search recommended entry trajectory"

			if !searchable {
				cause = "utility_entry"
				reason = "positive executable utility accepted before causal history reached the MCTS minimum"
			}

			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:                  types.ActionEnter,
				Symbol:                  symbol,
				At:                      forecast.At,
				Utility:                 utility,
				Opportunity:             opportunity,
				AllocationHaircut:       haircut,
				AllocationHaircutReason: haircutReason,
				AllocationClass:         "normal",
				Alternatives: map[string]float64{
					"enter":   utility,
					"nothing": 0,
				},
				ExpectedFees:      forecast.ExpectedFees,
				ExpectedSpread:    forecast.ExpectedSpread,
				ExpectedReturn:    forecast.ExpectedReturn,
				ExpectedImpact:    forecast.ExpectedImpact,
				AdverseSelection:  adverse,
				Uncertainty:       forecast.Uncertainty,
				Confidence:        forecast.Confidence,
				OpportunityMargin: utility,
				CognitiveLead:     cognition.LookaheadScore,
				BasinConfidence:   cognition.Confidence,
				ReferencePrice:    forecast.ReferencePrice,
				ValidThroughEpoch: forecast.Epoch,
				ForecastSource:    string(types.SourceResonance),
				ForecastEpoch:     forecast.Epoch,
				Cause:             cause,
				Reason:            reason,
				Trace:             trace,
			})

			continue
		}

		thesis.Decisions = append(thesis.Decisions, evaluator.reject(
			forecast, utility, "mcts_rejected",
			"causal MCTS trajectory search did not select entry action",
			trace,
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

		/*
			The stop's geometry is fed even for lots this pass will not judge.
			Toxicity and crossing cost are observations about the book, not
			conclusions about the trade, and a position that is already exiting
			or has no forecast this tick still has a regulator deciding whether
			the price it is seeing is one it could have sold into.
		*/
		desk.ApplyEvidence(evaluator.stopEvidence(thesis, position.Holding.Symbol))

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

		exitUtility := -exitCostFraction

		/*
			The first-passage scenario remains on the decision as a diagnostic,
			but it has no execution authority. Open inventory is liquidated only
			by its broker-owned stop, so a forecast revision cannot pre-empt the
			price geometry the position was sized and admitted under.
		*/
		scenario, holdEV, scored := evaluator.scorePassage(
			thesis, position, forecast, exitCostFraction,
		)

		alternatives := map[string]float64{
			"hold": holdUtility,
			"exit": exitUtility,
		}

		if scored {
			alternatives["passage_hold_ev"] = holdEV
			alternatives["passage_profit_first"] = scenario.ProfitFirst
			alternatives["passage_loss_first"] = scenario.LossFirst
			alternatives["passage_timeout"] = scenario.Timeout
			alternatives["passage_support"] = scenario.Support
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:            types.ActionHold,
			Symbol:            forecast.Symbol,
			At:                forecast.At,
			Utility:           holdUtility,
			Alternatives:      alternatives,
			ProposedNotional:  decimal.NewFromInt64(0),
			ProposedQuantity:  decimal.NewFromInt64(0),
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
			ReferencePrice:    forecast.ReferencePrice,
			ValidThroughEpoch: forecast.Epoch,
			ForecastSource:    string(types.SourceResonance),
			ForecastEpoch:     forecast.Epoch,
			Cause:             "continuation",
			Reason:            "open position is governed exclusively by its stoploss",
		})
	}

	/*
		Learning happens last, from the lots that finished. A trade only teaches
		anything once it is over, and running this after the pass above means a
		lot that closed on this very tick is retired with the last state it was
		actually observed in rather than one tick stale.
	*/
	evaluator.retireEpisodes(desk)
}

/*
stopEvidence is what the strategy knows about an open lot that the broker
cannot see for itself.

The broker owns the book: what the touch holds and what selling into it would
realise. What it has no way to derive is whether the touch is honest. A bid
that keeps stepping up while the quantity behind it is being pulled produces a
rising mark that nothing could actually have been sold into, and a regulator
reading only prices would ratchet its floor up to meet a peak that never
existed.

The spread and impact travel together because the impact estimate is a fraction
of the spread it was measured against.
*/
func (evaluator Evaluator) stopEvidence(
	thesis *types.Thesis,
	symbol string,
) types.StopEvidence {
	evidence := types.StopEvidence{Symbol: symbol}

	if evaluator.price == nil {
		return evidence
	}

	tick := evaluator.price.Tick(symbol)

	if tick == nil || tick.Bid == nil || tick.Ask == nil {
		return evidence
	}

	if tick.Ask.Cmp(tick.Bid) > 0 {
		evidence.Spread = tick.Ask.SetScale(8).Sub(tick.Bid)
		evidence.Impact = estimateImpact(thesis, symbol, evidence.Spread)
	}

	evidence.HollowPressure, evidence.HollowReady = hollowPressure(thesis, symbol)
	evidence.RegimeExit = regimeExit(thesis, symbol)
	evidence.ObservedAt = time.Now().UTC()
	evidence.Present = true

	return evidence
}

func (evaluator Evaluator) candidate(
	thesis *types.Thesis,
	symbol string,
) (candidate, bool) {
	if thesis == nil || thesis.Resonance == nil || evaluator.price == nil {
		return candidate{}, false
	}

	reading, ok := resonanceReading(thesis, symbol)

	if !ok || reading.Forecast == nil ||
		reading.ForecastValidity.State != types.ValidityValid {
		return candidate{}, false
	}

	if err := reading.Forecast.Validate(); err != nil {
		return candidate{}, false
	}

	_, ok = reading.Forecast.Step(0)

	if !ok {
		return candidate{}, false
	}

	expectedReturn := reading.Forecast.ExpectedReturn

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

	fees := reference.SetScale(8).Mul(fee.SetScale(8)).Mul(decimal.NewFromInt64(2))

	return candidate{
		Symbol:         symbol,
		At:             thesis.At,
		ExpectedReturn: reference.SetScale(8).Mul(decimal.NewFromFloat64(expectedReturn)),
		ReferencePrice: reference,
		ExpectedFees:   fees,
		ExpectedSpread: spread,
		ExpectedImpact: impact,
		Forecast:       reading.Forecast,
		Uncertainty:    reading.Surprise,
		Confidence:     reading.Forecast.Confidence,
		Epoch: uint64(thesis.Tick) +
			uint64(reading.Forecast.SupportedHorizon),
	}, true
}

func (evaluator Evaluator) reject(
	forecast candidate,
	utility float64,
	cause, reason string,
	trace *types.DecisionTrace,
) types.Decision {
	return types.Decision{
		Action:            types.ActionNothing,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
		ReferencePrice:    forecast.ReferencePrice,
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
		Trace:             trace,
	}
}
