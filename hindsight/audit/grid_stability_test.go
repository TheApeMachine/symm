package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestStabilityStage(t *testing.T) {
	Convey("Given a StabilityStage instance", t, func() {
		ctx := context.Background()
		stage := NewStabilityStage(ctx)

		feed := func(s *StabilityStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		makeGridJoin := func(region uint8, tick int64, now time.Time) *data.Value {
			source := "cvd"
			label := "signed_net_fraction"
			if region == 2 {
				source = "liquidity"
				label = "relative_spread"
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
			return join
		}

		Convey("When auditing stationary grid activations across halves", func() {
			measurements := make([]*data.Value, 0, 30)

			// Both halves alternate between R02 and R04 evenly
			for idx := range int64(30) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				reg := uint8(2)
				if idx%2 == 1 {
					reg = 4
				}

				measurements = append(measurements, makeGridJoin(reg, idx+1, now))
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing severe non-stationary regime shift between halves", func() {
			measurements := make([]*data.Value, 0, 30)

			// First half: 15 frames of R02
			for idx := range int64(15) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurements = append(measurements, makeGridJoin(2, idx+1, now))
			}

			// Second half: 15 frames of R04 (completely disjoint distributions, JSD = 1.0 bit)
			for idx := int64(15); idx < 30; idx++ {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurements = append(measurements, makeGridJoin(4, idx+1, now))
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasNonStationary := false
			for _, violation := range violations {
				if violation.Name == "grid.non_stationary" {
					hasNonStationary = true
				}
			}
			So(hasNonStationary, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "grid.non_stationary")
		})

		Convey("When auditing incomplete grid observations", func() {
			measurements := make([]*data.Value, 0, 25)

			// 25 runtime:join measurements without active bright regions (0% completeness)
			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				join := data.NewValue("runtime", "runtime:join", "BTC/USD")
				join.Tick = uint64(idx + 1)
				join.Timestamp = uint64(now.UnixNano())
				measurements = append(measurements, join)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasIncomplete := false
			for _, violation := range violations {
				if violation.Name == "grid.incomplete" {
					hasIncomplete = true
				}
			}
			So(hasIncomplete, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "grid.incomplete")
		})

		Convey("When auditing nil measurements", func() {
			violations := feed(stage, nil)
			So(len(violations), ShouldEqual, 1)
			So(violations[0].Name, ShouldEqual, "measurement")
		})
	})
}
