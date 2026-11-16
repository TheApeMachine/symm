package broker

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewStoploss(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewStoploss is called with initial pnl and arm", func() {
			sl := NewStoploss(0.5)

			Convey("State is healthy with the given arm threshold", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.Regime, ShouldEqual, RegimeSmooth)
				So(sl.InitialPnLPct, ShouldEqual, 0.5)
				So(sl.CurrentPnLPct, ShouldEqual, 0.5)
				So(sl.PeakPnLPct, ShouldEqual, 0.5)
				So(sl.ArmPnlPct, ShouldEqual, 0.8)
				So(sl.Confirms, ShouldEqual, 0)
				So(sl.MaxWindow, ShouldEqual, 20)
				So(sl.MaxSwings, ShouldEqual, 12)
				So(sl.RallyStart, ShouldEqual, 0.5)
				So(sl.PullbackLow, ShouldEqual, 0.5)
				So(len(sl.Deltas), ShouldEqual, 0)
				So(sl.TypicalPullback, ShouldEqual, 0.02)
				So(sl.TriggerReason, ShouldEqual, "")
				So(sl.ExplosiveActive, ShouldBeFalse)
			})
		})
	})
}

func TestStoplossUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When peak stays below the arm threshold", func() {
			sl := NewStoploss(0.5)
			sl.Update(0.6, 0)
			sl.Update(0.8, 0)
			sl.Update(0.4, 0)

			Convey("State stays healthy", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.CurrentPnLPct, ShouldEqual, 0.4)
				So(sl.PeakPnLPct, ShouldEqual, 0.8)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When an underwater position gaps above arm", func() {
			sl := NewStoploss(-5.0)
			sl.Update(-5.0, 0)
			sl.Update(2.0, 0)

			Convey("State arms on peak without triggering", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 2.0)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When friction raises the effective arm", func() {
			sl := NewStoploss(1.2)
			sl.Update(1.2, 1.0)

			Convey("Peak below friction*1.5 keeps healthy", func() {
				So(sl.State, ShouldEqual, StoplossHealthy)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When crawl chops inside learned breathing room", func() {
			sl := NewStoploss(1.0)
			steps := []float64{1.02, 1.03, 1.04, 1.01, 1.05, 1.06, 1.03, 1.07}

			for _, pnl := range steps {
				sl.Update(pnl, 0)
			}

			Convey("Familiar giveback stays armed", func() {
				So(sl.Triggered(), ShouldBeFalse)
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 1.07)
			})
		})

		Convey("When pnl climbs above arm", func() {
			sl := NewStoploss(0.5)

			for i := range 5 {
				sl.Update(1.0+float64(i)*0.3, 0)
			}

			Convey("State is armed and peak tracks the climb", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PeakPnLPct, ShouldEqual, 2.2)
				So(sl.CurrentPnLPct, ShouldEqual, 2.2)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When state is already triggered", func() {
			sl := NewStoploss(3.0)
			sl.State = StoplossTriggered
			sl.CurrentPnLPct = 2.0
			sl.PeakPnLPct = 4.0

			sl.Update(1.5, 0)
			sl.Update(-47.0, 0)

			Convey("Update is a no-op while Triggered", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.CurrentPnLPct, ShouldEqual, 2.0)
				So(sl.PeakPnLPct, ShouldEqual, 4.0)
				So(sl.Triggered(), ShouldBeTrue)
			})
		})

		Convey("When armed crawl breaks the trailing floor with negative progress", func() {
			sl := NewStoploss(2.0)
			for _, pnl := range []float64{2.0, 2.5, 3.0, 2.7, 3.2, 2.4, 2.0, 1.5} {
				sl.Update(pnl, 0)
			}
			if !sl.Triggered() {
				sl.Update(1.2, 0)
				sl.Update(1.0, 0)
			}

			Convey("Peak is remembered while armed or tripped", func() {
				So(sl.State, ShouldBeIn, StoplossArmed, StoplossTriggered)
				So(sl.PeakPnLPct, ShouldBeGreaterThanOrEqualTo, 3.0)
			})
		})

		Convey("When an explosive runner reverses and breaks floor", func() {
			sl := NewStoploss(1.0)
			sl.Update(1.0, 0)
			sl.Update(2.6, 0) // ignition; floor trails under peak by ~avgStep room
			sl.Update(0.15, 0)

			Convey("Trip records an explosive exit reason", func() {
				So(sl.Triggered(), ShouldBeTrue)
				So(sl.TriggerReason, ShouldBeIn, "explosive_floor_break", "explosive_reversal")
				So(sl.Regime, ShouldEqual, RegimeExplosive)
			})
		})

		Convey("When underwater after arm, Update refuses a loss exit", func() {
			sl := NewStoploss(2.0)
			sl.Update(2.0, 0)
			sl.Update(0.005, 0)

			Convey("State stays armed without TriggerReason", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
				So(sl.TriggerReason, ShouldEqual, "")
				So(sl.Confirms, ShouldEqual, 0)
			})
		})
	})
}

func TestTrip(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When trip is called", func() {
			sl := NewStoploss(2.0)
			sl.State = StoplossArmed
			sl.trip("trail_confirm")

			Convey("State and TriggerReason stick", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.TriggerReason, ShouldEqual, "trail_confirm")
				So(sl.Triggered(), ShouldBeTrue)
			})
		})
	})
}

func TestTriggered(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When state is not triggered", func() {
			So(NewStoploss(0.5).Triggered(), ShouldBeFalse)
		})

		Convey("When state is triggered", func() {
			sl := NewStoploss(2.0)
			sl.State = StoplossTriggered
			So(sl.Triggered(), ShouldBeTrue)
		})
	})
}

func TestAnalyzeMemory(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When deltas are empty", func() {
			sl := NewStoploss(1.0)
			avg, down, net, chaos, flip := sl.analyzeMemory()

			Convey("All readings are zero", func() {
				So(avg, ShouldEqual, 0)
				So(down, ShouldEqual, 0)
				So(net, ShouldEqual, 0)
				So(chaos, ShouldEqual, 0)
				So(flip, ShouldEqual, 0)
			})
		})

		Convey("When deltas progress cleanly", func() {
			sl := NewStoploss(1.0)
			sl.Deltas = []float64{0.1, 0.2, 0.1}
			avg, down, net, chaos, flip := sl.analyzeMemory()

			Convey("Net progress and avg step match the window", func() {
				So(net, ShouldEqual, 0.4)
				So(avg, ShouldEqual, 0.4/3)
				So(down, ShouldEqual, 0.4/3)
				So(chaos, ShouldBeGreaterThan, 0)
				So(flip, ShouldEqual, 0)
			})
		})
	})
}

func TestStoplossMarshal(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a lived-in stoploss is marshaled for S3", func() {
			sl := NewStoploss(1.0)
			sl.Update(1.5, 0)
			sl.Update(2.0, 0)
			sl.Update(1.8, 0)
			sl.trip("trail_confirm")

			body, err := sonic.Marshal(sl)
			So(err, ShouldBeNil)

			var got Stoploss
			So(sonic.Unmarshal(body, &got), ShouldBeNil)

			Convey("Exported lifecycle fields survive the round trip", func() {
				So(got.State, ShouldEqual, StoplossTriggered)
				So(got.TriggerReason, ShouldEqual, "trail_confirm")
				So(got.PeakPnLPct, ShouldEqual, 2.0)
				So(got.CurrentPnLPct, ShouldEqual, 1.8)
				So(got.ArmPnlPct, ShouldEqual, 1.0)
				So(got.MaxWindow, ShouldEqual, 20)
				So(got.MaxSwings, ShouldEqual, 12)
				So(got.RallyStart, ShouldEqual, 1.0)
				So(len(got.Deltas), ShouldBeGreaterThan, 0)
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
