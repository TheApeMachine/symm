package broker

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
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
				So(sl.ArmPnlPct, ShouldEqual, system.Cfg.Risk.StoplossArmPnLPct)
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

		Convey("When friction does not move the arm line", func() {
			sl := NewStoploss(1.2)
			sl.Update(1.2, 1.0)

			Convey("Arming follows ArmPnlPct only", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
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

		Convey("When an explosive runner collapses below the arm line", func() {
			sl := NewStoploss(1.0)
			sl.Update(1.0, 0)
			sl.Update(2.6, 0)
			sl.CommitPending()
			sl.Update(0.15, 0)

			Convey("Filled Tranche 1 keeps Armed so the floor still protects", func() {
				So(sl.TranchesExited, ShouldBeGreaterThanOrEqualTo, 1)
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
				So(sl.Regime, ShouldEqual, RegimeExplosive)
			})
		})

		Convey("When current falls below the arm line after Tranche 1", func() {
			sl := NewStoploss(2.0)
			sl.Update(2.0, 0)
			sl.CommitPending()
			sl.Update(0.5, 0)

			Convey("State stays Armed without a full trip", func() {
				So(sl.TranchesExited, ShouldBeGreaterThanOrEqualTo, 1)
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When price re-enters above the arm line after disarm", func() {
			sl := NewStoploss(1.0)
			sl.Update(2.0, 0)
			sl.Update(0.5, 0)
			sl.Update(2.0, 0)

			Convey("Stoploss re-arms", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When an underwater chop finally arms", func() {
			sl := NewStoploss(-1.5)
			for _, pnl := range []float64{
				-1.55, -1.50, -1.60, -1.45, -1.70, -1.40, -1.65, -1.35,
			} {
				sl.Update(pnl, 0)
			}
			underwaterPB := sl.TypicalPullback
			sl.Update(1.0, 0)

			Convey("onArm reseeds pullback room above underwater chop scale", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.TypicalPullback, ShouldBeGreaterThanOrEqualTo, minArmedPullbackFloor)
				So(sl.TypicalPullback, ShouldBeGreaterThan, underwaterPB)
				So(len(sl.Pullbacks), ShouldEqual, 0)
				So(len(sl.Pushes), ShouldEqual, 0)
			})
		})

		Convey("When first profit peak is still inside the sharp-break margin", func() {
			sl := NewStoploss(-1.5)
			sl.Update(-1.5, 0)
			sl.Update(1.0, 0)
			sl.Update(1.18, 0)
			sl.EmaDelta = -0.05
			floor := sl.TrailingFloor
			sl.Update(floor-0.05, 0)

			Convey("trail_sharp_break is withheld; position stays armed or waits confirms", func() {
				So(sl.TriggerReason, ShouldNotEqual, "trail_sharp_break")
				So(sl.PeakPnLPct, ShouldBeLessThan, sl.ArmPnlPct+sharpBreakPeakMargin)
			})
		})

		Convey("When a multi-tick climb never prints a single 0.50% delta", func() {
			sl := NewStoploss(0.2)
			for _, pnl := range []float64{
				0.25, 0.30, 0.35, 0.40, 0.45, 0.55, 0.70, 0.85, 1.05, 1.25,
			} {
				sl.Update(pnl, 0)
			}

			Convey("Cumulative climb ignites the explosive runner path", func() {
				So(sl.ExplosiveActive, ShouldBeTrue)
				So(sl.Regime, ShouldEqual, RegimeExplosive)
				So(sl.PeakPushDelta, ShouldBeGreaterThanOrEqualTo, 0.50)
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

			Convey("State and TriggerReason stick with a full-remainder claim", func() {
				So(sl.State, ShouldEqual, StoplossTriggered)
				So(sl.TriggerReason, ShouldEqual, "trail_confirm")
				So(sl.Triggered(), ShouldBeTrue)
				So(sl.PendingExitPct, ShouldEqual, 1)
				So(sl.PendingTranches, ShouldEqual, totalTranches)
				So(sl.TranchesExited, ShouldEqual, 0)
				So(sl.WantsExit(), ShouldBeTrue)
			})
		})
	})
}

func TestAbortPending(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a trip is vetoed before fill", func() {
			sl := NewStoploss(2.0)
			sl.State = StoplossArmed
			sl.trip("trail_confirm")
			sl.AbortPending()

			Convey("Triggered clears and WantsExit stops without banking", func() {
				So(sl.State, ShouldEqual, StoplossArmed)
				So(sl.PendingExitPct, ShouldEqual, 0)
				So(sl.PendingTranches, ShouldEqual, 0)
				So(sl.TranchesExited, ShouldEqual, 0)
				So(sl.WantsExit(), ShouldBeFalse)
				So(sl.Triggered(), ShouldBeFalse)
			})
		})

		Convey("When Tranche 1 is vetoed before fill", func() {
			sl := NewStoploss(0.5)
			sl.Update(1.0, 0)
			So(sl.PendingTranches, ShouldEqual, 1)
			So(sl.TranchesExited, ShouldEqual, 0)
			sl.AbortPending()

			Convey("Claim rolls back so the next arm tick can re-emit Tranche 1", func() {
				So(sl.PendingTranches, ShouldEqual, 0)
				So(sl.PendingExitPct, ShouldEqual, 0)
				So(sl.TranchesExited, ShouldEqual, 0)
				So(sl.WantsExit(), ShouldBeFalse)

				sl.Update(1.0, 0)
				So(sl.PendingTranches, ShouldEqual, 1)
				So(sl.TriggerReason, ShouldEqual, "tranche_1_arm")
			})
		})
	})
}

func TestTrancheLadder(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When PnL climbs Arm then Arm+step (50% early banks)", func() {
			sl := NewStoploss(0.5)
			sl.BasisQty = 100
			sl.Update(1.0, 0)
			So(sl.TranchesExited, ShouldEqual, 0)
			So(sl.PendingTranches, ShouldEqual, 1)
			So(sl.PendingExitPct, ShouldEqual, 0.25)
			So(sl.NextTargetPct, ShouldEqual, sl.ArmPnlPct+sl.trancheStep())
			So(sl.Triggered(), ShouldBeFalse)
			So(sl.WantsExit(), ShouldBeTrue)

			sl.CommitPending()
			So(sl.TranchesExited, ShouldEqual, 1)
			So(sl.PendingTranches, ShouldEqual, 0)

			sl.Update(2.0, 0)
			So(sl.TranchesExited, ShouldEqual, 1)
			So(sl.PendingTranches, ShouldEqual, 1)
			So(sl.PendingExitPct, ShouldEqual, 0.25)
			So(sl.Triggered(), ShouldBeFalse)

			sl.CommitPending()
			So(sl.TranchesExited, ShouldEqual, 2)

			sl.Update(3.0, 0)
			So(sl.TranchesExited, ShouldEqual, 2)
			So(sl.PendingTranches, ShouldEqual, 0)
			So(sl.PendingExitPct, ShouldEqual, 0)
			So(sl.Triggered(), ShouldBeFalse)
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
				So(got.ArmPnlPct, ShouldEqual, system.Cfg.Risk.StoplossArmPnLPct)
				So(got.MaxWindow, ShouldEqual, 20)
				So(got.MaxSwings, ShouldEqual, 12)
				// onArm reseeds RallyStart to the arm-time mark (first Update above arm).
				So(got.RallyStart, ShouldEqual, 1.5)
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
