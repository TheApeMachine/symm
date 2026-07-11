package leadlag

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestTickerMeasurementFromFeatures(t *testing.T) {
	Convey("Given lag features with strong delayed correlation", t, func() {
		ticker := NewTicker(NewSection())
		features := LagFeatures{
			Price:       100,
			MoveMoved:   true,
			LagOK:       true,
			LagBars:     6,
			LagCorr:     0.8,
			ContempOK:   true,
			ContempCorr: 0.1,
			SampleCount: 64,
			StallMargin: 0,
		}

		Convey("When Ticker.measurementFromFeatures scores the evidence", func() {
			result, err := ticker.measurementFromFeatures(
				"ETH/USD",
				time.Now(),
				features,
			)

			Convey("Then delayed correlation dominates synchronized drift", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)

				if result == nil {
					return
				}

				So(result.Metrics["inefficient"], ShouldBeGreaterThan, result.Metrics["sync"])
				So(result.Metrics["inefficient"], ShouldBeGreaterThan, result.Metrics["decoupled"])
			})
		})
	})

	Convey("Given stalled-anchor features with low correlation", t, func() {
		ticker := NewTicker(NewSection())
		features := LagFeatures{
			Price:       100,
			MoveMoved:   false,
			LagOK:       true,
			LagBars:     1,
			LagCorr:     0.1,
			ContempOK:   true,
			ContempCorr: 0.1,
			SampleCount: 64,
			StallMargin: 0.9,
		}

		Convey("When Ticker.measurementFromFeatures scores the evidence", func() {
			result, err := ticker.measurementFromFeatures(
				"ETH/USD",
				time.Now(),
				features,
			)

			Convey("Then stall evidence dominates decoupling", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)

				if result == nil {
					return
				}

				So(result.Metrics["stall"], ShouldBeGreaterThan, result.Metrics["decoupled"])
				So(result.Metrics["stall"], ShouldBeGreaterThan, result.Metrics["sync"])
			})
		})
	})
}

func TestTickerMeasurementFromFeaturesPreservesSign(t *testing.T) {
	Convey("Given a follower moving inversely against the anchor", t, func() {
		ticker := NewTicker(NewSection())
		features := LagFeatures{
			Price:       100,
			MoveMoved:   true,
			LagOK:       true,
			LagBars:     6,
			LagCorr:     -0.8,
			ContempOK:   true,
			ContempCorr: -0.1,
			SampleCount: 64,
			StallMargin: 0,
		}

		Convey("When Ticker.measurementFromFeatures scores the evidence", func() {
			result, err := ticker.measurementFromFeatures(
				"ETH/USD",
				time.Now(),
				features,
			)

			Convey("Then the exported metrics keep the negative sign though category strength stays magnitude-only", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)

				if result == nil {
					return
				}

				So(result.Metrics["correlation"], ShouldBeGreaterThan, 0)
				So(result.Metrics["signedLagCorrelation"], ShouldBeLessThan, 0)
				So(result.Metrics["signedCorrelation"], ShouldBeLessThan, 0)
				So(result.Metrics["signedCorrelation"], ShouldEqual, result.Metrics["signedLagCorrelation"])
			})
		})
	})

	Convey("Given a follower moving in lockstep with the anchor", t, func() {
		ticker := NewTicker(NewSection())
		features := LagFeatures{
			Price:       100,
			MoveMoved:   true,
			LagOK:       true,
			LagBars:     6,
			LagCorr:     0.8,
			ContempOK:   true,
			ContempCorr: 0.1,
			SampleCount: 64,
			StallMargin: 0,
		}

		Convey("When Ticker.measurementFromFeatures scores the evidence", func() {
			result, err := ticker.measurementFromFeatures(
				"ETH/USD",
				time.Now(),
				features,
			)

			Convey("Then the exported correlation and its signed counterpart agree in sign", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)

				if result == nil {
					return
				}

				So(result.Metrics["signedCorrelation"], ShouldBeGreaterThan, 0)
				So(result.Metrics["signedCorrelation"], ShouldAlmostEqual, result.Metrics["correlation"], 1e-9)
			})
		})
	})
}

func TestTickerMeasureSkipsIncompleteRow(t *testing.T) {
	Convey("Given a ticker row without a last price", t, func() {
		ticker := NewTicker(NewSection())
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)
		So(crossSection.Observe([]kraken.TickerData{{
			Symbol:    "BTC/USD",
			Last:      mustDecimal("100"),
			Timestamp: time.Now(),
		}}), ShouldBeNil)

		Convey("When Measure runs before last is populated on the follower", func() {
			result, err := ticker.Measure(kraken.TickerData{
				Symbol:    "ETH/USD",
				Timestamp: time.Now(),
			}, crossSection)

			Convey("It should wait without error", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeNil)
			})
		})
	})
}

func mustDecimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return parsed
}
