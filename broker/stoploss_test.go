package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func stallNearPeak(stop *Stoploss) {
	stop.State = StoplossArmed
	stop.PeakPnLPct = 3.0
	stop.CurrentPnLPct = 3.0
	stop.Ascent.Samples = 4
	stop.Ascent.PeakVelocity = 1.0
	stop.Ascent.SmoothVelocity = 0.2
	stop.Ascent.LastPnLPct = 3.0
	stop.Ascent.LastTime = time.Now().Add(-time.Second)
	stop.ExhaustScore = stop.Ascent.Exhaust()
}

func TestNewStoploss(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewStoploss is called with initial pnl", func() {
			sl := NewStoploss(0.5)

			Convey("State is healthy with cfg arm threshold", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.InitialPnLPct, ShouldEqual, 0.5)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.ArmPnlPct, ShouldEqual, system.Cfg.Selection.StoplossArmPnlPct)
				So(sl.ArmPnlPct, ShouldEqual, 1.0)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.ExhaustScore, ShouldEqual, 0)
			})
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When pnl sits below the arm threshold", func() {
			sl := NewStoploss(2.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.Confirms = 2
			sl.Ascent.Samples = 5

			sl.Update(0.5, 0)

			Convey("State resets to healthy, peak and confirms clear", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.ExhaustScore, ShouldEqual, 0)
				So(sl.Ascent.Samples, ShouldEqual, 0)
			})
		})

		Convey("When pnl first clears the arm floor", func() {
			sl := NewStoploss(0.0)
			sl.Update(1.5, 0)

			Convey("State arms and peak tracks pnl", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.CurrentPnLPct, ShouldEqual, 1.5)
				So(sl.PeakPnLPct, ShouldEqual, 1.5)
			})
		})

		Convey("When stall ticks corroborate with market friction", func() {
			sl := NewStoploss(1.0)
			stallNearPeak(sl)

			sl.Update(3.0, system.Cfg.Selection.StoplossPumpConfirmMin)
			So(sl.Confirms, ShouldEqual, 1)
			// Flat PnL must not invent climb decay — exhaust stays at stall seed.
			So(sl.ExhaustScore, ShouldEqual, 0.8)

			stallNearPeak(sl)
			sl.Confirms = 1
			sl.Update(3.0, system.Cfg.Selection.StoplossPumpConfirmMin)
			So(sl.Confirms, ShouldEqual, 2)

			stallNearPeak(sl)
			sl.Confirms = 2
			sl.Update(3.0, system.Cfg.Selection.StoplossPumpConfirmMin)

			Convey("Three confirms trip the stoploss", func() {
				So(sl.Confirms, ShouldEqual, 3)
				So(sl.State, ShouldEqual, StoplossTriggered)
			})
		})

		Convey("When state is already triggered", func() {
			sl := NewStoploss(3.0)
			sl.State = StoplossTriggered
			sl.CurrentPnLPct = 2.0
			sl.PeakPnLPct = 4.0

			sl.Update(1.5, 0)

			Convey("Update is a no-op", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.CurrentPnLPct, ShouldEqual, 2.0)
				So(sl.PeakPnLPct, ShouldEqual, 4.0)
			})
		})

		Convey("When pnl climbs above arm with real ticks", func() {
			sl := NewStoploss(0.5)

			for i := range 5 {
				pnl := 1.0 + float64(i)*0.3
				sl.Update(pnl, 0)
				time.Sleep(60 * time.Millisecond)
			}

			Convey("State is armed and peak tracks the climb", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 2.2)
				So(sl.CurrentPnLPct, ShouldEqual, 2.2)
				So(sl.Ascent.Samples, ShouldEqual, 5)
			})
		})

		Convey("When a grind arm gives back from peak while still above arm", func() {
			sl := NewStoploss(1.0)
			sl.State = StoplossArmed
			sl.Regime = StoplossRegimeGrind
			sl.PeakPnLPct = 3.0
			sl.CurrentPnLPct = 3.0
			sl.Ascent.Samples = 8
			sl.Ascent.PeakVelocity = 0.01
			sl.Ascent.SmoothVelocity = 0
			sl.Ascent.LastPnLPct = 3.0
			sl.Ascent.LastTime = time.Now().Add(-time.Second)

			for range 3 {
				sl.Update(2.4, 0)
			}

			Convey("Giveback trail trips the stoploss", func() {
				So(sl.Regime, ShouldEqual, StoplossRegimeGrind)
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.Confirms, ShouldBeGreaterThanOrEqualTo, 3)
			})
		})

		Convey("When a climb arms then rolls off peak by givebackPct", func() {
			sl := NewStoploss(1.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.Ascent.Samples = 4
			sl.Ascent.PeakVelocity = 1.0
			sl.Ascent.SmoothVelocity = 0.5
			sl.Ascent.LastPnLPct = 3.0
			sl.Ascent.LastTime = time.Now().Add(-time.Second)

			for range 3 {
				sl.Update(2.4, 0)
			}

			Convey("Giveback trail trips even though not near peak", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
			})
		})
	})
}

func TestTriggered(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When state is not triggered", func() {
			sl := NewStoploss(0.5)

			Convey("Triggered is false", func() {
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When state is triggered", func() {
			sl := NewStoploss(2.0)
			sl.State = StoplossTriggered

			Convey("Triggered is true", func() {
				So(sl.Triggered(), ShouldBeTrue)
			})
		})
	})
}

func BenchmarkNewStoploss(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewStoploss(0.5)
	}
}

func BenchmarkStoplossUpdate(b *testing.B) {
	sl := NewStoploss(0.0)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		sl.Update(1.5, 0)
	}
}

func BenchmarkTriggered(b *testing.B) {
	sl := NewStoploss(0.5)
	sl.State = StoplossTriggered

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = sl.Triggered()
	}
}
