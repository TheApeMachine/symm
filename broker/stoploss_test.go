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
				So(sl.ExhaustScore, ShouldEqual, 0)
			})
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When pnl sits below the arm threshold", func() {
			sl := NewStoploss(2.0, 1.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.Confirms = 1
			sl.Ascent.Samples = 5
			sl.Ascent.MeanUp = 4.0
			sl.Ascent.LastPnLPct = 2.5

			sl.Update(0.5, 0)

			Convey("State resets to healthy and clears ascent", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.ExhaustScore, ShouldEqual, 0)
				So(sl.Ascent.Samples, ShouldEqual, 0)
				So(sl.Ascent.MeanUp, ShouldEqual, 0)
				So(sl.Ascent.LastPnLPct, ShouldEqual, 0)
			})
		})

		Convey("When an underwater position gaps above arm", func() {
			sl := NewStoploss(-5.0, 1.0)
			sl.Update(-5.0, 0)
			sl.Update(2.0, 0)

			Convey("Ascent starts fresh post-arm without underwater jump contamination", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Ascent.Samples, ShouldEqual, 1)
				So(sl.Ascent.MeanUp, ShouldEqual, 0)
				So(sl.PeakPnLPct, ShouldEqual, 2.0)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When pnl first clears the arm floor", func() {
			sl := NewStoploss(0.0, 1.0)
			sl.Update(1.5, 0)

			Convey("State arms without triggering on sample 1", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.CurrentPnLPct, ShouldEqual, 1.5)
				So(sl.PeakPnLPct, ShouldEqual, 1.5)
				So(sl.Ascent.Samples, ShouldEqual, 1)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When arm floor is fee-net break-even", func() {
			sl := NewStoploss(0.5, 0)
			sl.Update(1.5, 1)

			Convey("Strictly green PnLPct arms", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When climb stalls at peak with full friction", func() {
			sl := NewStoploss(1.0, 1.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.CurrentPnLPct = 3.0
			sl.Ascent.Samples = 4
			sl.Ascent.PeakVelocity = 1.0
			sl.Ascent.SmoothVelocity = 0
			sl.Ascent.MeanUp = 0.2
			sl.Ascent.LastPnLPct = 3.0
			sl.Ascent.LastTime = time.Now().Add(-time.Second)
			sl.ExhaustScore = sl.Ascent.Exhaust()

			// Unchanged pnl is ignored — nudge down so crest can evaluate.
			sl.Update(2.95, 1.0)

			Convey("Crest exit trips when exhaust + friction covers the climb", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
			})
		})

		Convey("When a climb sees one small pullback at sample 4 without friction", func() {
			sl := NewStoploss(0.5, 0.8)
			sl.Update(1.0, 0)
			time.Sleep(10 * time.Millisecond)
			sl.Update(1.5, 0)
			time.Sleep(10 * time.Millisecond)
			sl.Update(1.55, 0)
			time.Sleep(10 * time.Millisecond)
			sl.Update(1.45, 0)

			Convey("Stoploss stays armed instead of hair-triggering on a single dip", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When state is already triggered", func() {
			sl := NewStoploss(3.0, 1.0)
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
			sl := NewStoploss(0.5, 1.0)

			for i := range 5 {
				pnl := 1.0 + float64(i)*0.3
				sl.Update(pnl, 0)
				time.Sleep(20 * time.Millisecond)
			}

			Convey("State is armed and peak tracks the climb", func() {
				// 1.0 is at the arm floor (not strictly above) — four green ticks remain.
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 2.2)
				So(sl.CurrentPnLPct, ShouldEqual, 2.2)
				So(sl.Ascent.Samples, ShouldEqual, 4)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When climb giveback meets exhaust-scaled trail", func() {
			sl := NewStoploss(1.0, 1.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.CurrentPnLPct = 3.0
			sl.Ascent.Samples = 4
			sl.Ascent.PeakVelocity = 1.0
			sl.Ascent.SmoothVelocity = 0.5
			sl.Ascent.MeanUp = 0.2
			sl.Ascent.LastPnLPct = 3.0
			sl.Ascent.LastTime = time.Now().Add(-time.Second)
			// Exhaust = 0.5 -> trail needs giveback >= climb * 0.5 = 1.0
			sl.Update(2.0, 0)

			Convey("Trail exit trips", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
			})
		})

		Convey("When L3 repeats the same pnl", func() {
			sl := NewStoploss(1.0, 1.0)
			sl.State = StoplossArmed
			sl.PeakPnLPct = 3.0
			sl.CurrentPnLPct = 3.0
			sl.Ascent.Samples = 4
			sl.Ascent.PeakVelocity = 1.0
			sl.Ascent.SmoothVelocity = 0
			sl.Ascent.MeanUp = 0.2
			sl.Ascent.LastPnLPct = 3.0
			sl.Ascent.LastTime = time.Now().Add(-time.Second)

			sl.Update(3.0, 1.0)
			sl.Update(3.0, 1.0)

			Convey("Unchanged pnl never confirms an exit", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})
	})
}

func TestTriggered(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When state is not triggered", func() {
			sl := NewStoploss(0.5, 1.0)

			Convey("Triggered is false", func() {
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When state is triggered", func() {
			sl := NewStoploss(2.0, 1.0)
			sl.State = StoplossTriggered

			Convey("Triggered is true", func() {
				So(sl.Triggered(), ShouldBeTrue)
			})
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

func BenchmarkTriggered(b *testing.B) {
	sl := NewStoploss(0.5, 1.0)
	sl.State = StoplossTriggered

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = sl.Triggered()
	}
}
