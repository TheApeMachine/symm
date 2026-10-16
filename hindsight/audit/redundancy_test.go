package audit

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestRedundancyStage(t *testing.T) {
	Convey("Given a RedundancyStage instance", t, func() {
		ctx := context.Background()
		stage := NewRedundancyStage(ctx)

		feed := func(s *RedundancyStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing independent orthogonal metrics", func() {
			measurements := make([]*data.Value, 0, 20)

			for idx := range int64(20) {
				val := data.NewValue("test", "test", "BTC/USD")
				valA := float64(idx)
				valB := math.Sin(float64(idx) * math.Pi / 2.0)

				data.AppendVector(val, []float64{valA, valB}, "metric_a", "metric_b")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing collinear metric pairs (|r| >= 0.95)", func() {
			measurements := make([]*data.Value, 0, 25)

			for idx := range int64(25) {
				val := data.NewValue("test", "test", "BTC/USD")
				valA := float64(idx) + 1.0
				valB := 2.0*valA + 3.0

				data.AppendVector(val, []float64{valA, valB}, "alpha", "beta")
				measurements = append(measurements, val)
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
			measurements := make([]*data.Value, 0, 25)

			for idx := range int64(25) {
				val := data.NewValue("test", "test", "BTC/USD")
				valA := float64(idx) + 1.0
				valB := -valA

				data.AppendVector(val, []float64{valA, valB}, "pos", "neg")
				measurements = append(measurements, val)
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
			measurements := make([]*data.Value, 0, 25)

			for idx := range int64(25) {
				val := float64(idx) + 1.0

				peerA := data.NewValue("signal_a", "signal", "BTC/USD")
				data.AppendVector(peerA, []float64{val}, "feature_x")

				peerB := data.NewValue("signal_b", "signal", "BTC/USD")
				data.AppendVector(peerB, []float64{2.0*val + 5.0}, "feature_y")

				join := data.NewValue("runtime", "runtime:join", "BTC/USD")
				join.Layers = append(peerA.Layers, peerB.Layers...)
				join.Index = append(peerA.Index, peerB.Index...)
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

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
