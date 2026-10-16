package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestPrecursorStage(t *testing.T) {
	Convey("Given a PrecursorStage instance", t, func() {
		ctx := context.Background()
		stage := NewPrecursorStage(ctx)

		feed := func(s *PrecursorStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing profitable excursions that clear order book spread and fees", func() {
			measurements := make([]*data.Value, 0, 30)

			// 6 excursions of 5 ticks: buy at Ask 100, price rises to Bid 108 (+8% move, clearing 1.6% fee + spread)
			for exIdx := range int64(6) {
				for tickIdx := range int64(5) {
					now := time.Now().Add(time.Duration(exIdx*5+tickIdx) * time.Millisecond)
					val := data.NewValue("test", "tick", "BTC/USD")
					val.Timestamp = uint64(now.UnixNano())
					val.Tick = uint64(exIdx*5 + tickIdx + 1)

					bid := 99.9 + float64(tickIdx)*2.0
					ask := 100.1 + float64(tickIdx)*2.0

					data.AppendVector(val, []float64{bid}, "best_price:bid")
					data.AppendVector(val, []float64{ask}, "best_price:ask")
					data.AppendVector(val, []float64{0.008}, "fee_rate")

					measurements = append(measurements, val)
				}
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing unprofitable excursions eaten by book spread and fees", func() {
			measurements := make([]*data.Value, 0, 30)

			// 6 excursions of 5 ticks where market is flat: buy at Ask 100.1, sell at Bid 99.9 (-0.2% spread loss + 1.6% fee = negative net return)
			for exIdx := range int64(6) {
				for tickIdx := range int64(5) {
					now := time.Now().Add(time.Duration(exIdx*5+tickIdx) * time.Millisecond)
					val := data.NewValue("test", "tick", "BTC/USD")
					val.Timestamp = uint64(now.UnixNano())
					val.Tick = uint64(exIdx*5 + tickIdx + 1)

					bid := 99.9 + float64(tickIdx)*0.001
					ask := 100.1 + float64(tickIdx)*0.001

					data.AppendVector(val, []float64{bid}, "best_price:bid")
					data.AppendVector(val, []float64{ask}, "best_price:ask")
					data.AppendVector(val, []float64{0.008}, "fee_rate")

					measurements = append(measurements, val)
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

		Convey("When auditing precursor token separation against background noise", func() {
			// Detector episode with precursor window [1, 20] and excursion [20, 30]
			now := time.Now()
			detectorMeas := data.NewValue("detector", "up", "BTC/USD")
			detectorMeas.Timestamp = uint64(now.UnixNano())
			data.AppendVector(detectorMeas, []float64{
				1, 20, 30, 40, 100, 110,
			}, "start_tick", "b_tick", "c_tick", "end_tick", "b_price", "c_price")

			ms := []*data.Value{detectorMeas}

			// Precursor ticks [1, 20] produce distinct signal, background ticks [31, 50] produce different signal
			for tick := int64(1); tick <= 50; tick++ {
				at := now.Add(time.Duration(tick) * time.Millisecond)

				source := "cvd"
				label := "flow_aligned_midpoint_return"
				if tick > 20 {
					source = "hawkes"
					label = "conditional_intensity_velocity"
				}

				peer := data.NewValue(source, "signal", "BTC/USD")
				peer.Tick = uint64(tick)
				peer.Timestamp = uint64(at.UnixNano())
				data.AppendVector(peer, []float64{0.9}, label)

				join := data.NewValue("runtime", "runtime:join", "BTC/USD")
				join.Tick = uint64(tick)
				join.Timestamp = uint64(at.UnixNano())
				join.Layers = append(join.Layers, peer.Layers...)
				join.Index = append(join.Index, peer.Index...)

				ms = append(ms, join)
			}

			violations := feed(stage, ms...)
			dataMap := stage.Data().(map[string]any)
			So(dataMap["jsd"].(float64), ShouldBeGreaterThan, 0.0)
			So(len(violations), ShouldEqual, 0)
		})
	})
}
