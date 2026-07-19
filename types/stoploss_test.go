package types

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestStoplossUnmarshalJSONPreservesUnlockedFloor proves recovery does not turn an
unearned ratchet floor into a break-even stop.
*/
func TestStoplossUnmarshalJSONPreservesUnlockedFloor(t *testing.T) {
	Convey("Given a newly bound stop serialized before any profitable lift", t, func() {
		bound := NewStoploss(context.Background())
		bound.Bind(100, 0.02)
		payload, err := json.Marshal(bound)
		So(err, ShouldBeNil)

		var recovered Stoploss
		So(json.Unmarshal(payload, &recovered), ShouldBeNil)
		restored := NewStoploss(context.Background())
		restored.Restore(100, &recovered)

		Convey("Then a flat mark remains above the adverse survival floor", func() {
			So(math.IsInf(restored.LockedFloor, -1), ShouldBeTrue)
			verdict := restored.Update(StopEvidence{
				Symbol:  "AAA/USD",
				Mark:    100,
				Entry:   100,
				Present: true,
			})
			So(verdict.Action, ShouldEqual, "hold")
			So(verdict.StopReturn, ShouldEqual, -0.02)
		})
	})
}

func testEvidence(mark, entry, uncertainty, expected, mse float64) StopEvidence {
	return testEvidenceAtEpoch(mark, entry, uncertainty, expected, mse, 1)
}

func testEvidenceAtEpoch(
	mark, entry, uncertainty, expected, mse float64,
	epoch uint64,
) StopEvidence {
	residual := 0.0

	if uncertainty > 0 && mse > 0 {
		residual = mse / uncertainty
	}

	return StopEvidence{
		Symbol:             "AAA/USD",
		Mark:               mark,
		Entry:              entry,
		ForecastEpoch:      epoch,
		NormalizedResidual: residual,
		ExpectedReturn:     expected,
		Uncertainty:        uncertainty,
		IncrementalMSE:     mse,
		ReturnReady:        true,
		Present:            true,
	}
}

/*
TestStoplossHoldsThroughValley ensures a dip above the live stop does not exit.
*/
func TestStoplossHoldsThroughValley(t *testing.T) {
	Convey("Given a lift then a valley above the live stop", t, func() {
		stop := NewStoploss(context.Background())
		_ = stop.Update(testEvidence(100, 100, 0.05, 0.02, 0.02))
		lift := stop.Update(testEvidence(103, 100, 0.05, 0.02, 0.02))
		valley := stop.Update(testEvidence(99, 100, 0.05, 0.01, 0.03))

		Convey("Then Action stays hold and LockedFloor does not regress", func() {
			So(valley.Action, ShouldEqual, "hold")
			So(valley.LockedFloor, ShouldBeGreaterThanOrEqualTo, lift.LockedFloor)
		})
	})
}

/*
TestStoplossFiresWhenMarkBreachesFloor proves the ratchet exit path.
*/
func TestStoplossFiresWhenMarkBreachesFloor(t *testing.T) {
	Convey("Given a peak then a mark through the locked floor", t, func() {
		stop := NewStoploss(context.Background())
		_ = stop.Update(testEvidence(100, 100, 0.02, 0.03, 0.0001))
		_ = stop.Update(testEvidence(110, 100, 0.02, 0.03, 0.0001))
		breached := stop.Update(testEvidence(101, 100, 0.02, 0.03, 0.0001))

		Convey("Then Action is stop", func() {
			So(breached.Action, ShouldEqual, "stop")
		})
	})
}

/*
TestStoplossTakeProfitNearPeakWithDeadForward fires TP near peak with dead forward.
*/
func TestStoplossTakeProfitNearPeakWithDeadForward(t *testing.T) {
	Convey("Given a peak and a non-positive forward path near the peak", t, func() {
		stop := NewStoploss(context.Background())
		_ = stop.Update(testEvidence(100, 100, 0.01, 0.04, 0.005))
		_ = stop.Update(testEvidence(108, 100, 0.01, 0.03, 0.005))
		profit := stop.Update(testEvidence(107.5, 100, 0.01, -0.01, 0.005))

		Convey("Then Action is take_profit", func() {
			So(profit.Action, ShouldEqual, "take_profit")
		})
	})
}

/*
TestStoplossBindLivesAtEntry proves fill-time Bind publishes a stop without σ.
*/
func TestStoplossBindLivesAtEntry(t *testing.T) {
	Convey("Given a Bind at entry with fee/spread distance", t, func() {
		stop := NewStoploss(context.Background())
		stop.Bind(100, 0.002)

		Convey("Then the regulator is armed with that adverse band", func() {
			So(stop.Armed(), ShouldBeTrue)
			So(stop.StopPrice(), ShouldBeGreaterThan, 0)
			So(stop.StopReturn, ShouldEqual, -0.002)
		})
	})
}

/*
TestStoplossWidenSurvivalWhileUnlocked raises the cold-bind floor before a
LockedFloor is earned and refuses to loosen once the ratchet owns geometry.
*/
func TestStoplossWidenSurvivalWhileUnlocked(t *testing.T) {
	Convey("Given a fee-thin cold bind", t, func() {
		stop := NewStoploss(context.Background())
		stop.Bind(100, 0.005)

		Convey("When live EntryTrail is wider while unlocked", func() {
			stop.WidenSurvival(0.02)
			So(stop.FloorDistance, ShouldEqual, 0.02)
			So(stop.TrailDistance, ShouldEqual, 0.02)
			So(stop.StopReturn, ShouldEqual, -0.02)

			hold := stop.Update(StopEvidence{
				Symbol: "AAA/USD", Mark: 99.0, Entry: 100, Present: true,
			})
			So(hold.Action, ShouldEqual, "hold")
		})

		Convey("When LockedFloor is earned, further widen is a no-op", func() {
			lift := NewStoploss(context.Background())
			_ = lift.Update(testEvidence(100, 100, 0.02, 0.05, 0.0004))
			peaked := lift.Update(testEvidence(104, 100, 0.02, 0.05, 0.0004))
			So(math.IsInf(peaked.LockedFloor, -1), ShouldBeFalse)

			before := lift.FloorDistance
			lift.WidenSurvival(before + 0.05)
			So(lift.FloorDistance, ShouldEqual, before)
		})

		Convey("When mark is sincerely through the armed floor", func() {
			deep := NewStoploss(context.Background())
			deep.Bind(100, 0.02)
			deep.WidenSurvival(0.02)
			stopped := deep.Update(StopEvidence{
				Symbol: "AAA/USD", Mark: 95, Entry: 100, Present: true,
			})
			So(stopped.Action, ShouldEqual, "stop")
		})
	})
}

/*
TestStoplossLivesOnSpreadAlone keeps an open lot protected from book width.
*/
func TestStoplossLivesOnSpreadAlone(t *testing.T) {
	Convey("Given spread-only Present evidence at entry", t, func() {
		stop := NewStoploss(context.Background())
		first := stop.Update(StopEvidence{
			Symbol: "AAA/USD", Mark: 100.1, Entry: 100, Spread: 0.001, Present: true,
		})

		Convey("Then the stop lives", func() {
			So(first.Action, ShouldEqual, "hold")
			So(stop.Armed(), ShouldBeTrue)
			So(stop.StopPrice(), ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestStoplossLockedFloorRatchetsUnderCalibratedForecast proves a peak inside
the forecast band can leave −Inf under signal-share Weight.
*/
func TestStoplossLockedFloorRatchetsUnderCalibratedForecast(t *testing.T) {
	Convey("Given a calibrated lift", t, func() {
		stop := NewStoploss(context.Background())
		_ = stop.Update(testEvidence(100, 100, 0.02, 0.05, 0.0004))
		lift := stop.Update(testEvidence(104, 100, 0.02, 0.05, 0.0004))

		Convey("Then LockedFloor is finite and positive", func() {
			So(math.IsInf(lift.LockedFloor, -1), ShouldBeFalse)
			So(lift.LockedFloor, ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestStoplossUpdateFreezesUnderRetreat ensures RetreatPressure freezes geometry.
*/
func TestStoplossUpdateFreezesUnderRetreat(t *testing.T) {
	Convey("Given retreat pressure on an adverse mark", t, func() {
		stop := NewStoploss(context.Background())
		stop.Bind(100, 0.002)
		_ = stop.Update(StopEvidence{
			Symbol: "AAA/USD", Mark: 100, Entry: 100,
			Uncertainty: 0.02, ExpectedReturn: 0.05,
			RetreatPressure: 0.9, RetreatReady: true, Present: true,
		})
		adverse := stop.Update(StopEvidence{
			Symbol: "AAA/USD", Mark: 98.5, Entry: 100,
			Uncertainty: 0.02, ExpectedReturn: 0.05,
			RetreatPressure: 0.9, RetreatReady: true, Present: true,
		})

		Convey("Then Action holds and LockedFloor stays unlocked", func() {
			So(adverse.Action, ShouldEqual, "hold")
			So(math.IsInf(stop.LockedFloor, -1), ShouldBeTrue)
			So(adverse.MarkReturn, ShouldBeLessThan, 0)
		})
	})
}

/*
TestStoplossObserveMarkDoesNotEmitStop keeps tick geometry silent for exits.
*/
func TestStoplossObserveMarkDoesNotEmitStop(t *testing.T) {
	Convey("Given ObserveMark through the live stop", t, func() {
		stop := NewStoploss(context.Background())
		stop.Bind(100, 0.01)
		stop.ObserveMark(98)

		Convey("Then Action is not an exit", func() {
			So(stop.Action, ShouldNotEqual, "stop")
			So(stop.Action, ShouldNotEqual, "take_profit")
			So(stop.MarkReturn, ShouldBeLessThan, 0)
		})
	})
}

/*
TestStoplossFreezesWithoutEvidence keeps floors intact across a nil frame.
*/
func TestStoplossFreezesWithoutEvidence(t *testing.T) {
	Convey("Given a live surface then Present false", t, func() {
		stop := NewStoploss(context.Background())
		_ = stop.Update(testEvidence(100, 100, 0.03, 0.02, 0.01))
		live := stop.Update(testEvidence(106, 100, 0.03, 0.02, 0.01))
		frozen := stop.Update(StopEvidence{Symbol: "AAA/USD", Present: false})

		Convey("Then geometry is unchanged", func() {
			So(frozen.Action, ShouldEqual, "hold")
			So(frozen.LockedFloor, ShouldEqual, live.LockedFloor)
			So(frozen.Weight, ShouldEqual, live.Weight)
			So(frozen.MarkReturn, ShouldEqual, live.MarkReturn)
		})
	})
}

/*
BenchmarkStoplossUpdate measures one Present update for the hot regulate path.
*/
func BenchmarkStoplossUpdate(b *testing.B) {
	stop := NewStoploss(context.Background())
	evidence := testEvidence(100, 100, 0.02, 0.01, 0.01)

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		evidence.Mark = 100 + float64(index%50)*0.1
		_ = stop.Update(evidence)
	}
}
