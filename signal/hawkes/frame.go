package hawkes

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) frame(symbolName string, outcome excitation.Outcome) *types.Measurement {
	branching := outcome.Fit.Params().BranchingMatrix()
	var buyToBuy *float64
	var sellToBuy *float64
	var buyToSell *float64
	var sellToSell *float64
	var spectralRadius *float64

	if outcome.Readiness.HawkesFit {
		buyToBuy = &branching[0][0]
		sellToBuy = &branching[0][1]
		buyToSell = &branching[1][0]
		sellToSell = &branching[1][1]
		spectralRadius = &outcome.Fit.SpectralRadius
	}

	measurement := &types.Measurement{
		ID:           uuid.NewString(),
		Source:       types.SourceHawkes,
		Symbol:       symbolName,
		At:           outcome.At,
		ObservedFrom: outcome.ObservedFrom,
		Horizon:      outcome.Horizon,
		Maturity:     outcome.Maturity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricEventCount, types.SideNone): {
				Raw:  float64(outcome.EventCount),
				Unit: types.UnitCount,
			},
			types.MetricKey(types.MetricEventCount, types.SideBuy): {
				Raw:  float64(outcome.BuyEventCount),
				Unit: types.UnitCount,
			},
			types.MetricKey(types.MetricEventCount, types.SideSell): {
				Raw:  float64(outcome.SellEventCount),
				Unit: types.UnitCount,
			},
			types.MetricKey(types.MetricArrivalRate, types.SideBuy): {
				Raw:  outcome.BuyArrivalRate,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricArrivalRate, types.SideSell): {
				Raw:  outcome.SellArrivalRate,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
				Raw:  outcome.Fit.IntensityX,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricConditionalIntensity, types.SideSell): {
				Raw:  outcome.Fit.IntensityY,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricBaselineIntensity, types.SideBuy): {
				Raw:  outcome.Fit.MuX,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricBaselineIntensity, types.SideSell): {
				Raw:  outcome.Fit.MuY,
				Unit: types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy): {
				Raw:        outcome.Fit.AlphaXX,
				Normalized: buyToBuy,
				Unit:       types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToBuy): {
				Raw:        outcome.Fit.AlphaXY,
				Normalized: sellToBuy,
				Unit:       types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToSell): {
				Raw:        outcome.Fit.AlphaYX,
				Normalized: buyToSell,
				Unit:       types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell): {
				Raw:        outcome.Fit.AlphaYY,
				Normalized: sellToSell,
				Unit:       types.UnitEventsPerSecond,
			},
			types.MetricKey(types.MetricDecayRate, types.SideNone): {
				Raw:  outcome.Fit.Beta,
				Unit: types.UnitInverseSecond,
			},
			types.MetricKey(types.MetricKernelMemory, types.SideNone): {
				Raw:  outcome.Fit.Runway().Seconds(),
				Unit: types.UnitSecond,
			},
			types.MetricKey(types.MetricSpectralRadius, types.SideNone): {
				Raw:        outcome.Fit.SpectralRadius,
				Normalized: spectralRadius,
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricHawkesPoissonDelta, types.SideNone): {
				Raw:  outcome.HawkesPoissonLogLikelihoodDelta,
				Unit: types.UnitNat,
			},
			types.MetricKey(types.MetricCrossSelfDelta, types.SideNone): {
				Raw:  outcome.CrossSelfLogLikelihoodDelta,
				Unit: types.UnitNat,
			},
			types.MetricKey(types.MetricImmediateOffspring, types.SideBuy): {
				Raw:  outcome.ImmediateBuyOffspring,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricImmediateOffspring, types.SideSell): {
				Raw:  outcome.ImmediateSellOffspring,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricTotalDescendants, types.SideBuy): {
				Raw:  outcome.TotalBuyDescendants,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricTotalDescendants, types.SideSell): {
				Raw:  outcome.TotalSellDescendants,
				Unit: types.UnitDimensionless,
			},
		},
	}
	separation, separationReady := types.MeasurementHypothesisSeparation(
		types.SourceHawkes,
		measurement.Metrics,
	)
	snrSample := types.MetricSample{
		Raw:  separation,
		Unit: types.UnitDimensionless,
	}

	if separationReady {
		snrSample.Normalized = &separation
	}

	measurement.PutMetric(types.MetricHypothesisSeparation, types.SideNone, snrSample)
	signal.remember(measurement)

	return measurement
}

func countOutcome(trade kraken.TradeData, input excitation.Input) excitation.Outcome {
	at := input.Horizon
	observedFrom := input.ObservedFrom
	buyCount := 0
	sellCount := 0

	if at.IsZero() {
		at = trade.Timestamp
	}

	if observedFrom.IsZero() {
		observedFrom = at
	}

	if input.Symbol != "" {
		buyCount, sellCount = arrivalCounts(input, at, observedFrom)
	}

	if buyCount+sellCount == 0 {
		if trade.Side == "buy" {
			buyCount = 1
		}

		if trade.Side == "sell" {
			sellCount = 1
		}
	}

	span := at.Sub(observedFrom)
	buyRate := 0.0
	sellRate := 0.0

	if span > 0 {
		buyRate = float64(buyCount) / span.Seconds()
		sellRate = float64(sellCount) / span.Seconds()
	}

	return excitation.Outcome{
		ObservedFrom:    observedFrom,
		At:              at,
		Horizon:         span,
		EventCount:      buyCount + sellCount,
		BuyEventCount:   buyCount,
		SellEventCount:  sellCount,
		BuyArrivalRate:  buyRate,
		SellArrivalRate: sellRate,
		Maturity:        0,
		Readiness: excitation.Readiness{
			Observation: true,
		},
	}
}

func arrivalCounts(
	input excitation.Input,
	at time.Time,
	observedFrom time.Time,
) (int, int) {
	buyCount, sellCount := input.Stream.ObservationCounts(at)

	if !at.Equal(observedFrom) {
		return buyCount, sellCount
	}

	for _, eventTime := range input.Stream.BuyTimes() {
		if eventTime.Equal(at) {
			buyCount++
		}
	}

	for _, eventTime := range input.Stream.SellTimes() {
		if eventTime.Equal(at) {
			sellCount++
		}
	}

	return buyCount, sellCount
}
