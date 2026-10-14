package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDynamicsStage(t *testing.T) {
	Convey("Given a DynamicsStage instance", t, func() {
		ctx := context.Background()
		stage := NewDynamicsStage(ctx)

		feed := func(s *DynamicsStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing structured state transitions", func() {
			measurements := make([]*data.Measurement, 0, 25)

			// Predictable deterministic cycle: A -> B -> C -> A ...
			tokens := []string{"state_A", "state_B", "state_C"}

			for idx := range int64(24) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				token := tokens[idx%3]
				measurement.Write(data.NewMetric(token, 1.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing an absorbing state collapse (> 80% same token)", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				// 24 of 25 are frozen_token (96% share)
				token := "frozen_token"
				if idx == 0 {
					token = "init_token"
				}

				measurement.Write(data.NewMetric(token, 1.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCollapse := false
			for _, violation := range violations {
				if violation.Name == "dynamics.absorbing_token_collapse" {
					hasCollapse = true
				}
			}
			So(hasCollapse, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "dynamics.absorbing_token_collapse")
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
