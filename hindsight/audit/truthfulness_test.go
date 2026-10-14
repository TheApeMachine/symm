package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTruthfulnessStage(t *testing.T) {
	Convey("Given a TruthfulnessStage instance", t, func() {
		ctx := context.Background()
		stage := NewTruthfulnessStage(ctx)

		feed := func(s *TruthfulnessStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing truthful quotes and conserving volumes", func() {
			measurements := make([]*data.Measurement, 0, 10)

			for idx := range int64(10) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				// Bid (99.0) <= Ask (101.0) and gross_volume (15.0) == buy (10.0) + sell (5.0)
				measurement.Write(
					data.NewMetric("best_bid_price", 99.0, data.UnitPrice, data.TimescaleInstantaneous),
					data.NewMetric("best_ask_price", 101.0, data.UnitPrice, data.TimescaleInstantaneous),
					data.NewMetric("gross_volume", 15.0, data.UnitVolume, data.TimescaleInstantaneous),
					data.NewMetric("buy_volume", 10.0, data.UnitVolume, data.TimescaleInstantaneous),
					data.NewMetric("sell_volume", 5.0, data.UnitVolume, data.TimescaleInstantaneous),
				)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing crossed market order book quotes (bid > ask)", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

			// Best bid 102.0 > Best ask 100.0 (crossed market anomaly)
			measurement.Write(
				data.NewMetric("best_bid_price", 102.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask_price", 100.0, data.UnitPrice, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCrossed := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.crossed_market" {
					hasCrossed = true
				}
			}
			So(hasCrossed, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "truthfulness.crossed_market")
		})

		Convey("When auditing volume conservation breaches", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

			// gross 100.0 != buy 30.0 + sell 20.0 (diff 50.0)
			measurement.Write(
				data.NewMetric("gross_volume", 100.0, data.UnitVolume, data.TimescaleInstantaneous),
				data.NewMetric("buy_volume", 30.0, data.UnitVolume, data.TimescaleInstantaneous),
				data.NewMetric("sell_volume", 20.0, data.UnitVolume, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasVolumeViolation := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.volume_divergence" {
					hasVolumeViolation = true
				}
			}
			So(hasVolumeViolation, ShouldBeTrue)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
