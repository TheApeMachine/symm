package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestDynamicsStage(t *testing.T) {
	Convey("Given a DynamicsStage instance", t, func() {
		ctx := context.Background()
		stage := NewDynamicsStage(ctx)

		feed := func(s *DynamicsStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		makeGridJoin := func(region uint8, tick int64, now time.Time, bid, ask float64) *data.Value {
			source := "cvd"
			label := "signed_net_fraction" // region 1
			switch region {
			case 2:
				source = "cvd"
				label = "midpoint_log_return" // region 2
			case 4:
				source = "cvd"
				label = "flow_aligned_midpoint_return" // region 4
			}

			peer := data.NewValue(source, "signal", "BTC/USD")
			peer.Tick = uint64(tick)
			peer.Timestamp = uint64(now.UnixNano())
			data.AppendVector(peer, []float64{0.95}, label)

			join := data.NewValue("runtime", "runtime:join", "BTC/USD")
			join.Tick = uint64(tick)
			join.Timestamp = uint64(now.UnixNano())
			join.Layers = append(join.Layers, peer.Layers...)
			join.Index = append(join.Index, peer.Index...)

			if bid > 0 && ask > 0 {
				pricePeer := data.NewValue("kraken", "quote", "BTC/USD")
				pricePeer.Tick = uint64(tick)
				pricePeer.Timestamp = uint64(now.UnixNano())
				data.AppendVector(pricePeer, []float64{bid}, "best_price:bid")
				data.AppendVector(pricePeer, []float64{ask}, "best_price:ask")
				join.Layers = append(join.Layers, pricePeer.Layers...)
				join.Index = append(join.Index, pricePeer.Index...)
			}

			return join
		}

		Convey("When auditing structured state transitions", func() {
			measurements := make([]*data.Value, 0, 24)

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
			measurements := make([]*data.Value, 0, 25)

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
