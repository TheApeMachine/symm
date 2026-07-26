package strategy

import (
	"context"
	"maps"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Opportunity turns logic outputs into friction-aware enter decisions.
*/
type Opportunity struct {
	ctx         context.Context
	cancel      context.CancelFunc
	price       *broker.Price
	balance     *broker.Balance
	recorder    *audit.Recorder
	uiHub       chan<- []byte
	maxFraction *decimal.Decimal
}

/*
NewOpportunity wires the surfaces Measure needs to score forecasts.
*/
func NewOpportunity(
	ctx context.Context,
	cancel context.CancelFunc,
	price *broker.Price,
	balance *broker.Balance,
	recorder *audit.Recorder,
	uiHub chan<- []byte,
) *Opportunity {
	return &Opportunity{
		ctx:      ctx,
		cancel:   cancel,
		price:    price,
		balance:  balance,
		recorder: recorder,
		uiHub:    uiHub,
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
	if err := opportunity.validate(map[string]any{"thesis": thesis}); err != nil {
		return
	}

	forecasts := append([]types.Forecasts(nil), thesis.Forecasts...)

	for index := range forecasts {
		opportunity.friction(&forecasts[index])
	}

	thesis.Forecasts = forecasts
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

	cash, err := opportunity.balance.AssetAvailable("USD")

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
for entry; Continuity already scored them after StampFriction.
*/
func (opportunity *Opportunity) Measure(thesis *types.Thesis) {
	if err := opportunity.validate(map[string]any{
		"thesis": thesis,
	}); err != nil {
		return
	}

	blocked := opportunity.occupied(thesis)
	forecasts := append([]types.Forecasts(nil), thesis.Forecasts...)

	for index := range forecasts {
		forecast := &forecasts[index]

		if _, skip := blocked[forecast.Symbol]; skip {
			continue
		}

		if !forecast.FrictionReady {
			opportunity.friction(forecast)
		}

		if !forecast.Eligible() {
			continue
		}

		cognition := types.Cognition{
			Ready:      true,
			Confidence: forecast.Confidence,
		}
		cognitionReady := false
		cogVal, found := thesis.Cognition.Load(forecast.Symbol)

		if found {
			live, ok := cogVal.(types.Cognition)

			if ok && live.Ready {
				cognition = live
				cognitionReady = true
			}
		}

		if cognitionReady && cognition.Ambiguous {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_ambiguity",
				"cognitive memory is ambiguous for this evidence sequence",
			))
			continue
		}

		if cognitionReady && isOpposingRegime(cognition.Winner) {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "cognitive_opposition",
				"cognitive memory does not support an entry: "+cognition.Winner,
			))
			continue
		}

		if cognitionReady && cognition.Confidence <= 0 {
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

		reading := measureOpportunity(*forecast, cognition, thesis)

		if reading.PhaseOpposes() {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "phase_opposition",
				"phase dial attractor conflicts with cognitive buy: "+reading.PhaseClass,
			))
			continue
		}

		trap := logic.TrapShare(thesis, forecast.Symbol)

		if trap.Dominates() {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "trap_dominant",
				"trap evidence dominates opportunity mass: "+trap.Family,
			))
			continue
		}

		categoryTrapShare, categoryTrapDominates := logic.CategoryTrap(thesis, forecast.Symbol)

		if categoryTrapDominates {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "category_trap_dominant",
				"category graph trap/contradict structure dominates opportunity: "+
					strconv.FormatFloat(categoryTrapShare, 'f', -1, 64),
			))
			continue
		}

		if reading.CausalReady && reading.CausalNoise > 0.6 && reading.CausalIntervention <= 0 {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, 0, "causal_confounded",
				"causal Do-calculus detects confounding noise dominating direct effect",
			))
			continue
		}

		// Enter pays the taker fee once; exit friction is scored when Continuity
		// or Rotate liquidates. ExecutableReturn already encodes that one-way cut.
		// Scaled by Pearl Causal Uplift (P(y|do(x)) - P(y)) to isolate direct effect.
		causalMult := 1.0

		if reading.CausalReady && reading.CausalUplift > 0 {
			causalMult += reading.CausalUplift
		}

		utility := (forecast.ExecutableReturn() * causalMult) - forecast.Uncertainty

		// Boost utility by the opportunity-leads share from the resident category
		// graph. When Leads edges from the current category into opportunity
		// categories outweigh those into exhaustion, the graph provides additional
		// predictive evidence that the move is real rather than a phantom lift.
		// Zero when no graph is available, preserving prior behavior.
		oppShare, _ := logic.CategoryOpportunityLead(thesis, forecast.Symbol)
		utility *= (1 + oppShare)

		if utility <= 0 {
			thesis.Decisions = append(thesis.Decisions, opportunity.reject(
				*forecast, utility, "infeasible",
				"expected executable utility does not exceed doing nothing",
			))
			continue
		}

		if cognitionReady && !reading.CognitiveClears(*forecast) {
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

		haircut := 1 - cognition.Confidence

		if haircut < 0 {
			haircut = 0
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:            types.ActionEnter,
			Symbol:            forecast.Symbol,
			At:                forecast.At,
			Utility:           utility,
			AllocationHaircut: haircut,
			AllocationClass:   allocation,
			Alternatives: map[string]float64{
				"enter":   utility,
				"nothing": 0,
			},
			ExpectedFees:      decimal.NewFromFloat64(forecast.ExpectedFees),
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
		ExpectedFees:      decimal.NewFromFloat64(forecast.ExpectedFees),
		ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread),
		ExpectedImpact:    decimal.NewFromFloat64(forecast.ExpectedImpact),
		AdverseSelection:  forecast.ExpectedAdverseSelection,
		Uncertainty:       forecast.Uncertainty,
		Cause:             cause,
		Reason:            reason,
	}
}

func (opportunity *Opportunity) validate(mandatory map[string]any) error {
	check := map[string]any{
		"price":   opportunity.price,
		"balance": opportunity.balance,
	}
	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}

/*
occupied lists wallet-open symbols, Thesis-created pending lots, and in-flight
lifecycle so Measure cannot propose a fresh enter against a live lot.
*/
func (opportunity *Opportunity) occupied(thesis *types.Thesis) map[string]struct{} {
	blocked := map[string]struct{}{}

	if opportunity.balance != nil {
		for _, holding := range opportunity.balance.Holdings() {
			if holding.Status == types.CLOSED {
				continue
			}

			blocked[holding.Symbol] = struct{}{}
		}
	}

	if thesis == nil {
		return blocked
	}

	thesis.Holdings.Range(func(_, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil || holding.Status == types.CLOSED {
			return true
		}

		blocked[holding.Symbol] = struct{}{}
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
