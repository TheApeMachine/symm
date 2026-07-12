package types

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
validateMetric enforces the dimensional contract carried by each metric name.
Logic can only compare evidence safely when subject, unit, and directional role
mean the same thing regardless of which signal implementation emitted it.
*/
func (measurement Measurement) validateMetric() error {
	valid := false

	switch measurement.Metric {
	case MetricEventCount:
		valid = measurement.matches(
			SubjectTradeArrivals, UnitCount, SideNone, SideBuy, SideSell,
		)
	case MetricArrivalRate:
		valid = measurement.matches(
			SubjectTradeArrivals, UnitEventsPerSecond, SideBuy, SideSell,
		)
	case MetricConditionalIntensity, MetricBaselineIntensity:
		valid = measurement.matches(
			SubjectHawkesProcess, UnitEventsPerSecond, SideBuy, SideSell,
		)
	case MetricExcitationAmplitude:
		valid = measurement.matches(
			SubjectHawkesKernel,
			UnitEventsPerSecond,
			SideBuyToBuy,
			SideSellToBuy,
			SideBuyToSell,
			SideSellToSell,
		)
	case MetricDecayRate:
		valid = measurement.matches(
			SubjectHawkesKernel, UnitInverseSecond, SideNone,
		)
	case MetricKernelMemory:
		valid = measurement.matches(
			SubjectHawkesKernel, UnitSecond, SideNone,
		)
	case MetricSpectralRadius:
		valid = measurement.matches(
			SubjectHawkesProcess, UnitDimensionless, SideNone,
		)
	case MetricHawkesPoissonDelta, MetricCrossSelfDelta:
		valid = measurement.matches(SubjectHawkesFit, UnitNat, SideNone)
	case MetricImmediateOffspring, MetricTotalDescendants:
		valid = measurement.matches(
			SubjectHawkesProcess, UnitDimensionless, SideBuy, SideSell,
		)
	}

	if !valid || !measurement.metricValueValid() {
		return errnie.Err(
			errnie.Validation,
			"measurement: metric identity or value domain is invalid",
			nil,
		)
	}

	return nil
}

func (measurement Measurement) metricValueValid() bool {
	switch measurement.Metric {
	case MetricEventCount:
		return measurement.Raw >= 0 && measurement.Raw == math.Trunc(measurement.Raw)
	case MetricDecayRate, MetricKernelMemory:
		return measurement.Raw > 0
	case MetricArrivalRate,
		MetricConditionalIntensity,
		MetricBaselineIntensity,
		MetricExcitationAmplitude,
		MetricSpectralRadius,
		MetricImmediateOffspring,
		MetricTotalDescendants:
		return measurement.Raw >= 0
	case MetricHawkesPoissonDelta, MetricCrossSelfDelta:
		return true
	}

	return false
}

func (measurement Measurement) matches(
	subject SubjectType,
	unit MeasurementUnit,
	sides ...MeasurementSide,
) bool {
	if measurement.Subject != subject || measurement.Unit != unit {
		return false
	}

	for _, side := range sides {
		if measurement.Side == side {
			return true
		}
	}

	return false
}
