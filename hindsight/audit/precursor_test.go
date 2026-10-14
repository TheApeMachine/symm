package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestPrecursorStage(t *testing.T) {
	Convey("Given a PrecursorStage instance", t, func() {
		ctx := context.Background()
		stage := NewPrecursorStage(ctx)

		feed := func(s *PrecursorStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing profitable excursions that clear order book spread and fees", func() {
			measurements := make([]*data.Measurement, 0, 30)

			// 6 excursions of 5 ticks: buy at Ask 100, price rises to Bid 108 (+8% move, clearing 1.6% fee + spread)
			for exIdx := range int64(6) {
				for tickIdx := range int64(5) {
					now := time.Now().Add(time.Duration(exIdx*5+tickIdx) * time.Millisecond)
					measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

					bid := 99.9 + float64(tickIdx)*2.0
					ask := 100.1 + float64(tickIdx)*2.0

					measurement.Write(
						data.NewMetric("best_bid_price", bid, data.UnitPrice, data.TimescaleInstantaneous),
						data.NewMetric("best_ask_price", ask, data.UnitPrice, data.TimescaleInstantaneous),
					)
					measurement.Finalize()
					measurements = append(measurements, measurement)
				}
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing unprofitable excursions eaten by book spread and fees", func() {
			measurements := make([]*data.Measurement, 0, 30)

			// 6 excursions of 5 ticks where market is flat: buy at Ask 100.1, sell at Bid 99.9 (-0.2% spread loss + 1.6% fee = negative net return)
			for exIdx := range int64(6) {
				for tickIdx := range int64(5) {
					now := time.Now().Add(time.Duration(exIdx*5+tickIdx) * time.Millisecond)
					measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

					bid := 99.9 + float64(tickIdx)*0.001
					ask := 100.1 + float64(tickIdx)*0.001

					measurement.Write(
						data.NewMetric("best_bid_price", bid, data.UnitPrice, data.TimescaleInstantaneous),
						data.NewMetric("best_ask_price", ask, data.UnitPrice, data.TimescaleInstantaneous),
					)
					measurement.Finalize()
					measurements = append(measurements, measurement)
				}
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasFrictionViolation := false
			for _, violation := range violations {
				if violation.Name == "precursor.unprofitable_friction" {
					hasFrictionViolation = true
				}
			}
			So(hasFrictionViolation, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "order book friction clearance")
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
