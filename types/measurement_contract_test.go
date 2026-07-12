package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementValidateMetric(t *testing.T) {
	Convey("Given Hawkes metric identities with their declared dimensions", t, func() {
		contracts := []struct {
			metric  MetricType
			subject SubjectType
			unit    MeasurementUnit
			side    MeasurementSide
		}{
			{MetricEventCount, SubjectTradeArrivals, UnitCount, SideNone},
			{MetricArrivalRate, SubjectTradeArrivals, UnitEventsPerSecond, SideBuy},
			{MetricConditionalIntensity, SubjectHawkesProcess, UnitEventsPerSecond, SideSell},
			{MetricBaselineIntensity, SubjectHawkesProcess, UnitEventsPerSecond, SideBuy},
			{MetricExcitationAmplitude, SubjectHawkesKernel, UnitEventsPerSecond, SideBuyToSell},
			{MetricDecayRate, SubjectHawkesKernel, UnitInverseSecond, SideNone},
			{MetricKernelMemory, SubjectHawkesKernel, UnitSecond, SideNone},
			{MetricSpectralRadius, SubjectHawkesProcess, UnitDimensionless, SideNone},
			{MetricHawkesPoissonDelta, SubjectHawkesFit, UnitNat, SideNone},
			{MetricCrossSelfDelta, SubjectHawkesFit, UnitNat, SideNone},
			{MetricImmediateOffspring, SubjectHawkesProcess, UnitDimensionless, SideBuy},
			{MetricTotalDescendants, SubjectHawkesProcess, UnitDimensionless, SideSell},
		}

		Convey("When each complete measurement is validated", func() {
			for _, contract := range contracts {
				measurement := validMeasurement()
				measurement.Metric = contract.metric
				measurement.Subject = contract.subject
				measurement.Unit = contract.unit
				measurement.Side = contract.side

				So(measurement.Validate(), ShouldBeNil)
			}
		})
	})

	Convey("Given a spectral radius labelled as directional natural-log evidence", t, func() {
		measurement := validMeasurement()
		measurement.Metric = MetricSpectralRadius
		measurement.Subject = SubjectHawkesProcess
		measurement.Unit = UnitNat
		measurement.Side = SideBuyToSell

		Convey("When its dimensional identity is validated", func() {
			Convey("Then the incompatible quantity is rejected", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given an unregistered metric name", t, func() {
		measurement := validMeasurement()
		measurement.Metric = MetricType("unknown")

		Convey("When its contract is validated", func() {
			Convey("Then logic cannot infer its meaning from the source", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given a fractional event count", t, func() {
		measurement := validMeasurement()
		measurement.Metric = MetricEventCount
		measurement.Subject = SubjectTradeArrivals
		measurement.Unit = UnitCount
		measurement.Side = SideNone
		measurement.Raw = 1.5

		Convey("When its metric domain is validated", func() {
			Convey("Then a count cannot masquerade as a continuous value", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given a negative conditional intensity", t, func() {
		measurement := validMeasurement()
		measurement.Raw = -1

		Convey("When its metric domain is validated", func() {
			Convey("Then an impossible point-process rate is rejected", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})
}
