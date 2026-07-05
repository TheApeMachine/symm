package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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

				So(result.Metric("inefficient"), ShouldBeGreaterThan, result.Metric("sync"))
				So(result.Metric("inefficient"), ShouldBeGreaterThan, result.Metric("decoupled"))
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

				So(result.Metric("stall"), ShouldBeGreaterThan, result.Metric("decoupled"))
				So(result.Metric("stall"), ShouldBeGreaterThan, result.Metric("sync"))
			})
		})
	})
}
