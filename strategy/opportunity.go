package strategy

import (
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Opportunity turns logic outputs into friction-aware enter decisions.
*/
type Opportunity struct {
	price       *broker.Price
	balance     *broker.Balance
	maxFraction *decimal.Decimal
}

/*
NewOpportunity wires the surfaces Measure needs to score forecasts.
*/
func NewOpportunity(price *broker.Price, balance *broker.Balance) *Opportunity {
	return &Opportunity{
		price:   price,
		balance: balance,
		maxFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.allocation.max_fraction"),
		),
	}
}

/*
StampFriction writes fees and impact onto every forecast so Continuity can
score occupied lots before Measure skips them for fresh enters.
*/
func (opportunity *Opportunity) StampFriction(thesis *types.Thesis) {
	for index := range thesis.Forecasts {
		opportunity.friction(&thesis.Forecasts[index])
	}
}

/*
friction stamps fees and touch-consumption impact onto one forecast. Impact is
the spread scaled by max_fraction of unreserved cash against BuyCapacity.
Unknown cash prices the full-touch bound because sizing is capped at the touch.
*/
func (opportunity *Opportunity) friction(forecast *types.Forecasts) {
	fraction, err := opportunity.price.Fraction(forecast.Symbol)

	if err != nil {
		errnie.Error(err)
		return
	}

	forecast.ExpectedFees = fraction.Float64()
	forecast.ExpectedImpact = forecast.ExpectedSpread *
		opportunity.utilization(forecast)
	forecast.FrictionReady = true
}

/*
utilization is the share of the visible ask touch a fresh enter would consume,
clamped to the full touch. It mirrors Arbiter.feasible: max_fraction of
unreserved cash, against the forecast's BuyCapacity.
*/
func (opportunity *Opportunity) utilization(forecast *types.Forecasts) float64 {
	if forecast.BuyCapacity == nil || forecast.BuyCapacity.Sign() <= 0 {
		return 1
	}

	cash, err := opportunity.balance.AssetAvailable(
		viper.GetString("market.quote_currency"),
	)

	if err != nil || cash == nil || cash.Sign() <= 0 {
		return 1
	}

	feasible := decimal.ExactMul(cash, opportunity.maxFraction)

	if feasible.Cmp(forecast.BuyCapacity) >= 0 {
		return 1
	}

	scale := max(
		int64(decimal.DefaultScale),
		feasible.GetScale(),
		forecast.BuyCapacity.GetScale(),
	)

	return feasible.SetScale(scale).
		Div(forecast.BuyCapacity.SetScale(scale)).
		Float64()
}

/*
Measure scores executable utility and appends enter when utility clears doing
nothing and cognition clears forecast noise. Cognitive and forecast rejects are
recorded as explicit nothing decisions for audit. Occupied symbols are skipped
for entry; Continuity already scored them after StampFriction. An exited symbol
must first reclaim its exact exit mark so adverse continuation cannot churn.
*/
func (opportunity *Opportunity) Measure(thesis *types.Thesis) {
	blocked := opportunity.occupied(thesis)

	for index := range thesis.Forecasts {
		forecast := &thesis.Forecasts[index]
		stopped := false
		var stopMark *decimal.Decimal

		if value, found := thesis.Holdings.Load(forecast.Symbol); found {
			holding := value.(*types.Holding)

			if holding.Stoploss != nil {
				holding.Stoploss.RLock()
				stopped = holding.Status == types.CLOSED &&
					(holding.Stoploss.Action == "stop" ||
						holding.Stoploss.Action == "take_profit")

				if stopped && holding.StopMark != nil {
					stopMark = holding.StopMark.Copy()
				}

				holding.Stoploss.RUnlock()
			}
		}

		if _, skip := blocked[forecast.Symbol]; skip {
			continue
		}

		if !forecast.FrictionReady {
			opportunity.friction(forecast)
		}

		if !forecast.Eligible() {
			continue
		}

		if stopped {
			ticker, err := opportunity.price.Get(forecast.Symbol)
			reclaimMark := (*decimal.Decimal)(nil)

			if err == nil && ticker != nil {
				reclaimMark = ticker.Last

				if ticker.Bid != nil && ticker.Ask != nil &&
					ticker.Bid.Sign() > 0 && ticker.Ask.Sign() > 0 {
					reclaimMark = opportunity.price.Div(
						ticker.Bid.Add(ticker.Ask),
						decimal.NewFromInt64(2),
					)
				}
			}

			if stopMark == nil || reclaimMark == nil {
				thesis.Decisions = append(thesis.Decisions, opportunity.reject(
					*forecast, 0, "post_exit_observation",
					"exited lot has no exact reclaim boundary",
				))
				continue
			}

			if reclaimMark.Cmp(stopMark) <= 0 {
				rejected := opportunity.reject(
					*forecast, 0, "post_exit_observation",
					"market has not reclaimed the exited lot boundary",
				)
				rejected.Alternatives["reclaim_price"] = stopMark.Float64()
				thesis.Decisions = append(thesis.Decisions, rejected)
				continue
			}
		}

		cogVal, ok := thesis.Cognition.Load(forecast.Symbol)

		if !ok {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_not_ready",
				"cognitive memory is not ready for this evidence sequence",
			))
			continue
		}

		cognition, ok := cogVal.(types.Cognition)

		if !ok || !cognition.Ready {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_not_ready",
				"cognitive memory is not ready for this evidence sequence",
			))
			continue
		}

		if cognition.Ambiguous {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_ambiguity",
				"cognitive memory is ambiguous for this evidence sequence",
			))
			continue
		}

		if cognition.Winner != "buy" {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_opposition",
				"cognitive memory does not support a buy entry",
			))
			continue
		}

		if cognition.Confidence <= 0 {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_no_confidence",
				"cognitive buy support has no confidence",
			))
			continue
		}

		if forecast.Confidence <= 0 {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "forecast_no_confidence",
				"forecast confidence is not positive",
			))
			continue
		}

		evidence, vetoed := opportunity.stance(thesis, forecast.Symbol)

		if vetoed {
			rejected := opportunity.reject(
				*forecast, 0, "evidence_opposition",
				"active evidence does not clear a long entry: "+
					strings.Join(evidence.Opposes, ", "),
			)
			rejected.Alternatives["evidence_favors"] = float64(len(evidence.Favors))
			rejected.Alternatives["evidence_opposes"] = float64(len(evidence.Opposes))
			rejected.Alternatives["evidence_vetoes"] = float64(len(evidence.Vetoes))
			thesis.Decisions = append(thesis.Decisions, rejected)
			continue
		}

		reading := measureOpportunity(*forecast, cognition, thesis)
		utility := reading.Margin - 2*forecast.ExpectedFees - forecast.ExpectedSpread -
			forecast.ExpectedImpact - forecast.ExpectedAdverseSelection

		if utility <= 0 {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, utility, "infeasible",
				"expected executable utility does not exceed doing nothing",
			))
			continue
		}

		if !reading.CognitiveClears(*forecast) {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, utility, "cognitive_weak",
				"cognitive confidence does not clear forecast noise share",
			))
			continue
		}

		allocation := "normal"

		if reading.Reserved() {
			allocation = "reserved"
		}

		haircut := max(0, 1-cognition.Confidence)

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:            types.ActionEnter,
			Symbol:            forecast.Symbol,
			At:                forecast.At,
			Utility:           utility,
			AllocationHaircut: haircut,
			AllocationClass:   allocation,
			Alternatives: map[string]float64{
				"enter":            utility,
				"nothing":          0,
				"evidence_favors":  float64(len(evidence.Favors)),
				"evidence_opposes": float64(len(evidence.Opposes)),
				"evidence_vetoes":  float64(len(evidence.Vetoes)),
			},
			ExpectedFees:      decimal.NewFromFloat64(2 * forecast.ExpectedFees),
			ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread),
			ExpectedReturn:    decimal.NewFromFloat64(forecast.ExpectedReturn),
			ExpectedImpact:    decimal.NewFromFloat64(forecast.ExpectedImpact),
			AdverseSelection:  forecast.ExpectedAdverseSelection,
			Uncertainty:       forecast.Uncertainty,
			ReferencePrice:    forecast.ReferencePrice.Copy(),
			ValidThroughEpoch: forecast.ExpiresEpoch,
			ForecastSource:    forecast.Source,
			ForecastModel:     forecast.ModelVersion,
			ForecastEpoch:     forecast.SourceEpoch,
			CalibrationCount:  forecast.CalibrationSamples,
			Confidence:        cognition.Confidence,
			OpportunityMargin: reading.Margin,
			CognitiveLead:     reading.Lead,
			BasinConfidence:   reading.Basin,
			Cause:             "entry",
			Reason:            "executable utility exceeds doing nothing",
		})
	}
}

/*
stance reads the symbol's composed evidence graph for the category phenomena
bearing on a long entry. Established deception, liquidity vacuum, collapse, or
active reversal veto directly; other opposing context vetoes only when it
outnumbers favoring phenomena. Missing structure remains neutral.
*/
func (opportunity *Opportunity) stance(
	thesis *types.Thesis,
	symbol string,
) (types.EntryEvidence, bool) {
	value, found := thesis.Graphs.Load(symbol)

	if !found {
		return types.EntryEvidence{}, false
	}

	evidenceGraph, ok := value.(*types.Graph)

	if !ok || evidenceGraph == nil {
		return types.EntryEvidence{}, false
	}

	evidence := evidenceGraph.LongEntryEvidence()
	return evidence,
		len(evidence.Vetoes) > 0 || len(evidence.Opposes) > len(evidence.Favors)
}

/*
reject records a nothing decision that still exposes enter utility when scored.
*/
func (opportunity *Opportunity) reject(
	forecast types.Forecasts,
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
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		ForecastModel:     forecast.ModelVersion,
		ForecastEpoch:     forecast.SourceEpoch,
		CalibrationCount:  forecast.CalibrationSamples,
		ExpectedReturn:    decimal.NewFromFloat64(forecast.ExpectedReturn),
		ExpectedFees:      decimal.NewFromFloat64(2 * forecast.ExpectedFees),
		ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread),
		ExpectedImpact:    decimal.NewFromFloat64(forecast.ExpectedImpact),
		AdverseSelection:  forecast.ExpectedAdverseSelection,
		Uncertainty:       forecast.Uncertainty,
		Cause:             cause,
		Reason:            reason,
	}
}

/*
occupied lists wallet-open symbols, Thesis-created pending lots, and in-flight
lifecycle so Measure cannot propose a fresh enter against a live lot.
*/
func (opportunity *Opportunity) occupied(thesis *types.Thesis) map[string]struct{} {
	blocked := map[string]struct{}{}

	for holding := range opportunity.balance.Holdings() {
		if holding.Status == types.CLOSED {
			continue
		}

		blocked[holding.Symbol] = struct{}{}
	}

	thesis.Holdings.Range(func(_, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil {
			return true
		}

		if holding.Stoploss != nil {
			holding.Stoploss.RLock()
			defer holding.Stoploss.RUnlock()
		}

		if holding.Status != types.CLOSED {
			blocked[holding.Symbol] = struct{}{}
		}

		return true
	})

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		switch value {
		case types.LifecycleEntrySelected, types.LifecycleEntrySubmitted,
			types.LifecycleExitSelected, types.LifecycleExitSubmitted,
			types.LifecyclePartiallyEntered, types.LifecycleManaging:
			blocked[symbol] = struct{}{}
		}
		return true
	})
	return blocked
}
