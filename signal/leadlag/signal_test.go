package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestMeasureAnchorStall(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		signal := &Signal{}
		anchor := newSymbolState()

		measurement, _, err := signal.measureStall(anchor, 0.6)

		Convey("It should classify an anchor stall on the unit interval", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceLeadLag)
			So(measurement.Category, ShouldEqual, perspectives.CategoryAnchorStall)
			So(measurement.Confidence, ShouldEqual, 0.6)
		})
	})
}

func TestMeasureFollower(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		signal := &Signal{}
		anchor := newSymbolState()
		follower := newSymbolState()

		Convey("When both series lack enough overlap", func() {
			anchor.observeTicker(50000, time.Now())
			follower.observeTicker(100, time.Now())

			measurement, _, err := signal.measureFollower(anchor, follower)

			Convey("It should withhold the reading", func() {
				So(err, ShouldBeNil)
				So(measurement.Source, ShouldEqual, perspectives.SourceNone)
			})
		})

		Convey("When a warmed anchor moves independently from the follower", func() {
			signal.anchorBaseline = *newMoveBaseline()
			start := time.Date(2026, 6, 3, 10, 30, 0, 0, time.UTC)

			for index := range minLagSamples {
				at := start.Add(time.Duration(index) * barInterval)
				anchor.observeTicker(100, at)
				follower.observeTicker(100.01, at)
			}

			for range anchorMoveMinObs {
				signal.anchorMoveStatus(anchor)
			}

			origin := start.Add(time.Duration(minLagSamples) * barInterval)

			for index := range 18 {
				at := origin.Add(time.Duration(index) * barInterval)
				anchor.observeTicker(100+float64(index)*0.3, at)
				follower.observeTicker(100-float64(index)*0.35, at)
				signal.anchorMoveStatus(anchor)
			}

			finalAt := origin.Add(18 * barInterval)
			anchor.observeTicker(116, finalAt)
			follower.observeTicker(93.5, finalAt)
			move := signal.anchorMoveStatus(anchor)
			measurement, _, err := signal.measureFollower(anchor, follower)

			Convey("It should clear the anchor move gate and classify decoupling", func() {
				So(err, ShouldBeNil)
				So(move.ready, ShouldBeTrue)
				So(move.moved, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, perspectives.SourceLeadLag)
				So(measurement.Category, ShouldEqual, perspectives.CategoryDecoupledMove)
			})
		})
	})
}

func TestPublishAnchorStall(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.anchor_symbol", "BTC/EUR")

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		anchor := newSymbolState()
		signal.symbols.Store(focus.AnchorSymbol(), anchor)

		err := signal.publishAnchorStall(focus.AnchorSymbol(), anchor, 0.5)

		Convey("It should publish one anchor stall reading", func() {
			So(err, ShouldBeNil)
		})
	})
}

func TestThrottle(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		signal := &Signal{lastPublish: time.Now()}

		Convey("It should reject publishes inside the interval", func() {
			So(signal.throttle(), ShouldBeFalse)
		})
	})
}

func BenchmarkMeasureFollower(b *testing.B) {
	signal := &Signal{}
	anchor := newSymbolState()
	follower := newSymbolState()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range minLagSamples {
		at := base.Add(time.Duration(index) * time.Minute)
		anchor.observeTicker(50000+float64(index), at)
		follower.observeTicker(100+float64(index), at.Add(2*time.Minute))
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = signal.measureFollower(anchor, follower)
	}
}
