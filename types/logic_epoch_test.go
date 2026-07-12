package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogicEpochClone(t *testing.T) {
	Convey("Given a logic epoch with one measurement", t, func() {
		epoch := LogicEpoch{
			Symbol:       "BTC/USD",
			At:           validMeasurement().At,
			Measurements: []Measurement{validMeasurement()},
		}

		Convey("When a cloned measurement is changed", func() {
			cloned := epoch.Clone()
			cloned.Measurements[0].Raw = 99

			Convey("Then the original epoch remains immutable", func() {
				So(epoch.Measurements[0].Raw, ShouldEqual, 3.0)
			})
		})
	})

	Convey("Given a transitional epoch containing mutable compatibility fields", t, func() {
		epoch := LogicEpoch{
			Measurements: []Measurement{{
				Categories: []Category{{Type: OrganicTrend}},
				Metrics:    map[string]float64{"price": 100},
			}},
		}

		Convey("When a reader mutates a clone", func() {
			cloned := epoch.Clone()
			cloned.Measurements[0].Categories[0].Type = Exhaustion
			cloned.Measurements[0].Metrics["price"] = 200

			Convey("Then caller-owned containers cannot rewrite the original", func() {
				So(epoch.Measurements[0].Categories[0].Type, ShouldEqual, OrganicTrend)
				So(epoch.Measurements[0].Metrics["price"], ShouldEqual, 100.0)
			})
		})
	})
}

func TestLogicEpochValidate(t *testing.T) {
	Convey("Given a coherent exact-time measurement epoch", t, func() {
		measurement := validMeasurement()
		epoch := LogicEpoch{
			Symbol:       measurement.Symbol,
			At:           measurement.At,
			Measurements: []Measurement{measurement},
		}

		Convey("When its provenance is validated", func() {
			Convey("Then every retained measurement agrees with the epoch", func() {
				So(epoch.Validate(), ShouldBeNil)
			})
		})

		Convey("When a retained measurement names another symbol", func() {
			epoch.Measurements[0].Symbol = "ETH/USD"

			Convey("Then the ambiguous epoch is rejected", func() {
				So(epoch.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When the epoch contains no measurements", func() {
			epoch.Measurements = nil

			Convey("Then an empty causal record is rejected", func() {
				So(epoch.Validate(), ShouldNotBeNil)
			})
		})
	})
}
