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
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		makeGridJoin := func(region uint8, tick int64, now time.Time, bid, ask float64) *data.Measurement {
			peer := data.NewMeasurement(1, "BTC/USD", "cvd", now, now)
			peer.Tick = tick

			label := "midpoint_log_return"
			if region == 4 {
				label = "flow_aligned_midpoint_return"
			}

			if region == 1 {
				label = "signed_net_fraction"
			}

			metric := data.NewMetric(label, 1.0, data.UnitDimensionless, data.TimescaleInstantaneous)
			metric.SetSNM(0.95)
			metrics := []*data.Metric{metric}

			if bid > 0 && ask > 0 {
				metrics = append(metrics,
					data.NewMetric("best_bid_price", bid, data.UnitPrice, data.TimescaleInstantaneous),
					data.NewMetric("best_ask_price", ask, data.UnitPrice, data.TimescaleInstantaneous),
				)
			}

			peer.Write(metrics...)
			peer.Finalize()

			join := data.NewMeasurement(1, "BTC/USD", "runtime:join", now, now)
			join.Tick = tick
			join.Peers(peer)
			join.Finalize()
			return join
		}

		Convey("When auditing structured state transitions", func() {
			measurements := make([]*data.Measurement, 0, 24)

			// Predictable deterministic cycle: R02 -> R04 -> R01 -> R02 ...
			regions := []uint8{2, 4, 1}

			for idx := range int64(24) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				reg := regions[idx%3]
				measurements = append(measurements, makeGridJoin(reg, idx+1, now, 0, 0))
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing an absorbing state collapse (> 80% same token)", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				reg := uint8(2)
				if idx == 0 {
					reg = 4
				}

				measurements = append(measurements, makeGridJoin(reg, idx+1, now, 0, 0))
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

		Convey("When auditing joined frames with peers and prices", func() {
			now := time.Now()
			join := makeGridJoin(2, 101, now, 50000.0, 50010.0)

			feed(stage, join)
			dataMap, ok := stage.Data().(map[string]any)
			So(ok, ShouldBeTrue)
			frames, ok := dataMap["frames"].([]*FrameData)
			So(ok, ShouldBeTrue)
			So(len(frames), ShouldEqual, 1)
			So(frames[0].Tick, ShouldEqual, 101)
			So(frames[0].Price, ShouldEqual, 50005.0)
			So(len(frames[0].Brightness), ShouldEqual, 15)
		})
	})
}
