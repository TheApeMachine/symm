package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRedundancyStage(t *testing.T) {
	Convey("Given a RedundancyStage instance", t, func() {
		ctx := context.Background()
		stage := NewRedundancyStage(ctx)

		feed := func(s *RedundancyStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing independent orthogonal metrics", func() {
			measurements := make([]*data.Measurement, 0, 20)

			for idx := range int64(20) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.SeqIdx = idx + 1

				// Metric A is linear, Metric B oscillates (orthogonal / near zero correlation).
				valA := float64(idx)
				valB := math.Sin(float64(idx) * math.Pi / 2.0)

				metricA := data.NewMetric("metric_a", valA, data.UnitDimensionless, data.TimescaleInstantaneous)
				metricB := data.NewMetric("metric_b", valB, data.UnitDimensionless, data.TimescaleInstantaneous)

				measurement.Write(metricA, metricB)
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing collinear metric pairs (|r| >= 0.95)", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.SeqIdx = idx + 1

				// Metric A and Metric B are collinear: B = 2*A + 3 (r = 1.0)
				valA := float64(idx) + 1.0
				valB := 2.0*valA + 3.0

				metricA := data.NewMetric("alpha", valA, data.UnitDimensionless, data.TimescaleInstantaneous)
				metricB := data.NewMetric("beta", valB, data.UnitDimensionless, data.TimescaleInstantaneous)

				measurement.Write(metricA, metricB)
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCollinear := false
			for _, violation := range violations {
				if violation.Name == "redundancy.collinear_pair" {
					hasCollinear = true
				}
			}
			So(hasCollinear, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "collinear clones")
		})

		Convey("When auditing negatively collinear metric pairs (r <= -0.95)", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.SeqIdx = idx + 1

				// Metric A and Metric B move in opposite directions: B = -valA (r = -1.0)
				valA := float64(idx) + 1.0
				valB := -valA

				metricA := data.NewMetric("pos", valA, data.UnitDimensionless, data.TimescaleInstantaneous)
				metricB := data.NewMetric("neg", valB, data.UnitDimensionless, data.TimescaleInstantaneous)

				measurement.Write(metricA, metricB)
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasNegCollinear := false
			for _, violation := range violations {
				if violation.Name == "redundancy.collinear_pair" {
					hasNegCollinear = true
				}
			}
			So(hasNegCollinear, ShouldBeTrue)
		})

		Convey("When auditing joined multi-signal frames with cross-signal collinearity", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				val := float64(idx) + 1.0

				peerA := data.NewMeasurement(1, "BTC/USD", "signal_a", now, now)
				peerA.Write(data.NewMetric("feature_x", val, data.UnitDimensionless, data.TimescaleInstantaneous))
				peerA.Finalize()

				peerB := data.NewMeasurement(1, "BTC/USD", "signal_b", now, now)
				peerB.Write(data.NewMetric("feature_y", 2.0*val+5.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				peerB.Finalize()

				join := data.NewMeasurement(1, "BTC/USD", "runtime:join", now, now)
				join.Peers(peerA, peerB)
				join.Finalize()
				measurements = append(measurements, join)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCrossSignalCollinear := false
			for _, violation := range violations {
				if violation.Name == "redundancy.collinear_pair" {
					hasCrossSignalCollinear = true
				}
			}
			So(hasCrossSignalCollinear, ShouldBeTrue)
		})

		Convey("When auditing nil measurements and nil metric entries", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")

			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			// Uninitialized metrics slice has nil entries
			measurement.Write(&data.Metric{})
			entryStage := NewRedundancyStage(ctx)
			violations := feed(entryStage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
