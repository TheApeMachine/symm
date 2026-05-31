package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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

func TestMeasure(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		signal := &Signal{}
		anchor := newSymbolState()
		follower := newSymbolState()

		Convey("When the anchor has not moved", func() {
			anchor.observeTicker(0.01, 50000, time.Now())
			follower.observeTicker(2.0, 100, time.Now())

			measurement, ok := signal.measure(anchor, false, follower)

			Convey("It should classify an anchor stall", func() {
				So(ok, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, perspectives.SourceLeadLag)
				So(measurement.Category, ShouldEqual, perspectives.CategoryAnchorStall)
			})
		})

		Convey("When both series lack enough overlap", func() {
			anchor.observeTicker(1.0, 50000, time.Now())
			follower.observeTicker(1.5, 100, time.Now())

			_, ok := signal.measure(anchor, true, follower)

			Convey("It should withhold the reading", func() {
				So(ok, ShouldBeFalse)
			})
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

func BenchmarkMeasure(b *testing.B) {
	signal := &Signal{}
	anchor := newSymbolState()
	follower := newSymbolState()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range minLagSamples {
		at := base.Add(time.Duration(index) * time.Minute)
		anchor.observeTicker(0.5, 50000+float64(index), at)
		follower.observeTicker(1.0, 100+float64(index), at.Add(2*time.Minute))
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.measure(anchor, true, follower)
	}
}
