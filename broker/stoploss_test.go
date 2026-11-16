package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewStoploss(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewStoploss is called with initial pnl and arm", func() {
			sl := NewStoploss(0.5, 0.8)

			Convey("State is healthy with the given arm threshold", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.InitialPnLPct, ShouldEqual, 0.5)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.ArmPnlPct, ShouldEqual, 0.8)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.path, ShouldNotBeNil)
			})
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When pnl sits below the arm threshold", func() {
			sl := NewStoploss(2.0, 1.0)
			sl.Update(1.5, 0)
			time.Sleep(10 * time.Millisecond)
			sl.Update(1.8, 0)
			sl.Update(0.5, 0)

			Convey("State resets to healthy and clears path memory", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.Ascent.Samples, ShouldEqual, 0)
			})
		})

		Convey("When an underwater position gaps above arm", func() {
			sl := NewStoploss(-5.0, 1.0)
			sl.Update(-5.0, 0)
			sl.Update(2.0, 0)

			Convey("Path starts post-arm without triggering", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Ascent.Samples, ShouldEqual, 1)
				So(sl.PeakPnLPct, ShouldEqual, 2.0)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When crawl chops inside learned downs", func() {
			sl := NewStoploss(1.0, 1.0)
			steps := []float64{1.02, 1.03, 1.04, 1.01, 1.05, 1.06, 1.03, 1.07}

			for _, pnl := range steps {
				sl.Update(pnl, 0)
				time.Sleep(15 * time.Millisecond)
			}

			Convey("Familiar giveback stays armed", func() {
				So(sl.Triggered(), ShouldBeFalse)
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Ascent.NormalDown(), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When pnl climbs above arm with real ticks", func() {
			sl := NewStoploss(0.5, 1.0)

			for i := range 5 {
				pnl := 1.0 + float64(i)*0.3
				sl.Update(pnl, 0)
				time.Sleep(20 * time.Millisecond)
			}

			Convey("State is armed and peak tracks the climb", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 2.2)
				So(sl.CurrentPnLPct, ShouldEqual, 2.2)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When L3 repeats the same pnl", func() {
			sl := NewStoploss(1.0, 1.0)
			sl.Update(1.5, 0)
			time.Sleep(10 * time.Millisecond)
			sl.Update(2.0, 0)
			samples := sl.Ascent.Samples
			sl.Update(2.0, 0)
			sl.Update(2.0, 0)

			Convey("Unchanged pnl never advances memory or confirms", func() {
				So(sl.Ascent.Samples, ShouldEqual, samples)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When state is already triggered and still above arm", func() {
			sl := NewStoploss(3.0, 1.0)
			sl.State = StoplossTriggered
			sl.CurrentPnLPct = 2.0
			sl.PeakPnLPct = 4.0

			sl.Update(1.5, 0)

			Convey("Update keeps Triggered and refreshes current", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.CurrentPnLPct, ShouldEqual, 1.5)
				So(sl.PeakPnLPct, ShouldEqual, 4.0)
			})
		})

		Convey("When triggered then PnL falls back through the arm", func() {
			sl := NewStoploss(3.0, 1.0)
			sl.State = StoplossTriggered
			sl.PeakPnLPct = 46.0
			sl.CurrentPnLPct = 2.0

			sl.Update(-47.0, 0)

			Convey("Frozen crest clears — no more fake stop-out peak", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.PeakPnLPct, ShouldEqual, -47.0)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})
	})
}

func TestTriggered(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When state is not triggered", func() {
			So(NewStoploss(0.5, 1.0).Triggered(), ShouldBeFalse)
		})

		Convey("When state is triggered", func() {
			sl := NewStoploss(2.0, 1.0)
			sl.State = StoplossTriggered
			So(sl.Triggered(), ShouldBeTrue)
		})
	})
}

func TestStoplossReset(t *testing.T) {
	Convey("Setup", t, func() {
		sl := NewStoploss(2.5, 1.0)
		sl.State = StoplossTriggered
		sl.PeakPnLPct = 4.0

		Convey("When Reset clears a cancelled exit", func() {
			sl.Reset()

			Convey("State returns to Healthy with the prior arm", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.ArmPnlPct, ShouldEqual, 1.0)
				So(sl.CurrentPnLPct, ShouldEqual, 2.5)
				So(sl.Triggered(), ShouldBeFalse)
				So(sl.path, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkNewStoploss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NewStoploss(0.5, 1.0)
	}
}

func BenchmarkStoplossUpdate(b *testing.B) {
	sl := NewStoploss(0.0, 1.0)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		sl.Update(1.5, 0)
	}
}
