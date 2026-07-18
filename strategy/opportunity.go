package strategy

import (
	"context"
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Opportunity turns logic outputs into enter decisions.
Fee rates come from Price.Fraction; flatten-now lot PnL stays on WithFriction.
*/
type Opportunity struct {
	ctx      context.Context
	cancel   context.CancelFunc
	price    *broker.Price
	recorder *audit.Recorder
	uiHub    chan<- []byte
}

/*
NewOpportunity wires the surfaces Measure needs to score forecasts.
*/
func NewOpportunity(
	ctx context.Context,
	cancel context.CancelFunc,
	price *broker.Price,
	recorder *audit.Recorder,
	uiHub chan<- []byte,
) *Opportunity {
	return &Opportunity{
		ctx:      ctx,
		cancel:   cancel,
		price:    price,
		recorder: recorder,
		uiHub:    uiHub,
	}
}

/*
Measure stamps fee friction, scores executable utility, and appends enter when
utility clears doing nothing and cognition clears forecast noise. Cognitive and
forecast rejects are recorded as explicit nothing decisions for audit.
*/
func (opportunity *Opportunity) Measure(thesis *types.Thesis) {
	if err := opportunity.validate(map[string]any{
		"thesis": thesis,
	}); err != nil {
		return
	}

	blocked := occupied(thesis)

	for index := range thesis.Forecasts {
		forecast := &thesis.Forecasts[index]

		if _, skip := blocked[forecast.Symbol]; skip {
			continue
		}

		fraction, err := opportunity.price.Fraction(forecast.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		forecast.ExpectedFees = fraction.Float64()
		forecast.FrictionReady = true

		if !forecast.Eligible() {
			continue
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

		risk := 1 - cognition.Confidence

		if risk < 0 {
			risk = 0
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:            types.ActionEnter,
			Symbol:            forecast.Symbol,
			At:                forecast.At,
			Utility:           utility,
			Risk:              risk,
			AllocationClass:   allocation,
			Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
			ExpectedFees:      decimal.NewFromFloat64(2 * forecast.ExpectedFees),
			ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread),
			ReferencePrice:    decimal.NewFromFloat64(forecast.ReferencePrice),
			ValidThroughEpoch: forecast.ExpiresEpoch,
			ForecastSource:    forecast.Source,
			ForecastEpoch:     forecast.SourceEpoch,
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
		ReferencePrice:    decimal.NewFromFloat64(forecast.ReferencePrice),
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		Cause:             cause,
		Reason:            reason,
	}
}

func (opportunity *Opportunity) validate(mandatory map[string]any) error {
	check := map[string]any{"price": opportunity.price}
	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}

/*
occupied lists symbols that already hold inventory or pending entry/exit intent
so Measure cannot propose a fresh enter against a live lot.
*/
func occupied(thesis *types.Thesis) map[string]struct{} {
	blocked := map[string]struct{}{}

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
