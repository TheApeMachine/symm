package strategy

import (
	"context"
	"testing"
)

func testEvidence(mark, entry, uncertainty, expected, mse float64) Evidence {
	return Evidence{
		Symbol:         "AAA/USD",
		Mark:           mark,
		Entry:          entry,
		ExpectedReturn: expected,
		Uncertainty:    uncertainty,
		IncrementalMSE: mse,
		ReturnReady:    true,
		Present:        true,
	}
}

/*
TestStoplossHoldsThroughValley ensures a dip that stays above the live stop
does not unwind lockedFloor and does not exit.
*/
func TestStoplossHoldsThroughValley(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	entry := 100.0
	uncertainty := 0.05

	first := stop.Update(testEvidence(100, entry, uncertainty, 0.02, 0.02))

	if first.Action != "hold" {
		t.Fatalf("arm: want hold, got %s (%s)", first.Action, first.Reason)
	}

	// Modest lift that does not yet lock a positive floor; valley must hold.
	lift := stop.Update(testEvidence(103, entry, uncertainty, 0.02, 0.02))
	floorAfterLift := lift.LockedFloor

	valley := stop.Update(testEvidence(99, entry, uncertainty, 0.01, 0.03))

	if valley.Action != "hold" {
		t.Fatalf(
			"valley: want hold, got %s (%s) floor=%v stop=%v mark=%v trail=%v weight=%v",
			valley.Action, valley.Reason, valley.LockedFloor, valley.StopReturn,
			valley.MarkReturn, valley.TrailDistance, valley.Weight,
		)
	}

	if valley.LockedFloor < floorAfterLift {
		t.Fatalf(
			"lockedFloor regressed: before=%v after=%v",
			floorAfterLift, valley.LockedFloor,
		)
	}
}

/*
TestStoplossFiresWhenMarkBreachesFloor proves the ratchet exit path.
*/
func TestStoplossFiresWhenMarkBreachesFloor(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	entry := 100.0

	_ = stop.Update(testEvidence(100, entry, 0.02, 0.03, 0.01))
	_ = stop.Update(testEvidence(110, entry, 0.02, 0.03, 0.01))

	breached := stop.Update(testEvidence(101, entry, 0.02, 0.0, 0.01))

	if breached.Action != "stop" {
		t.Fatalf("want stop after breach, got %s (%s)", breached.Action, breached.Reason)
	}
}

/*
TestStoplossTakeProfitNearPeakWithDeadForward fires TP when mark sits near the
peak and the forward expected return turns non-positive.
*/
func TestStoplossTakeProfitNearPeakWithDeadForward(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	entry := 100.0

	_ = stop.Update(testEvidence(100, entry, 0.01, 0.04, 0.005))
	_ = stop.Update(testEvidence(108, entry, 0.01, 0.03, 0.005))

	profit := stop.Update(testEvidence(107.5, entry, 0.01, -0.01, 0.005))

	if profit.Action != "take_profit" {
		t.Fatalf(
			"want take_profit, got %s (%s) peak=%v mark=%v",
			profit.Action, profit.Reason, profit.PeakReturn, profit.MarkReturn,
		)
	}
}

/*
TestStoplossFreezesWithoutEvidence keeps floors intact across a nil frame.
*/
func TestStoplossFreezesWithoutEvidence(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	_ = stop.Update(testEvidence(100, 100, 0.03, 0.02, 0.01))
	live := stop.Update(testEvidence(106, 100, 0.03, 0.02, 0.01))

	frozen := stop.Update(Evidence{Symbol: "AAA/USD", Present: false})

	if frozen.Action != "hold" {
		t.Fatalf("absent evidence should hold, got %s", frozen.Action)
	}

	if frozen.LockedFloor != live.LockedFloor || frozen.Weight != live.Weight {
		t.Fatalf("freeze mutated surface: live=%+v frozen=%+v", live, frozen)
	}
}

/*
TestStoplossWeightMovesWithSkill checks asymmetric skill updates move weight.
*/
func TestStoplossWeightMovesWithSkill(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	poor := stop.Update(testEvidence(100, 100, 0.02, 0.01, 0.08))
	better := stop.Update(testEvidence(101, 100, 0.02, 0.01, 0.002))

	if better.Weight <= poor.Weight {
		t.Fatalf(
			"weight should rise with skill: poor=%v better=%v",
			poor.Weight, better.Weight,
		)
	}
}

/*
BenchmarkStoplossUpdate measures one Present update for the hot regulate path.
*/
func BenchmarkStoplossUpdate(b *testing.B) {
	stop := NewStoploss(context.Background())
	evidence := testEvidence(100, 100, 0.02, 0.01, 0.01)

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		evidence.Mark = 100 + float64(index%50)*0.1
		_ = stop.Update(evidence)
	}
}
