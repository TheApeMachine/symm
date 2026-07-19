package types

import (
	"context"
	"math"
	"testing"
)

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
	t.Parallel()

	stop := NewStoploss(context.Background())
	_ = stop.Update(testEvidence(100, 100, 0.05, 0.02, 0.02))
	lift := stop.Update(testEvidence(103, 100, 0.05, 0.02, 0.02))
	valley := stop.Update(testEvidence(99, 100, 0.05, 0.01, 0.03))

	if valley.Action != "hold" {
		t.Fatalf("valley: want hold, got %s (%s)", valley.Action, valley.Reason)
	}

	if valley.LockedFloor < lift.LockedFloor {
		t.Fatalf("lockedFloor regressed: before=%v after=%v", lift.LockedFloor, valley.LockedFloor)
	}
}

/*
TestStoplossFiresWhenMarkBreachesFloor proves the ratchet exit path.
*/
func TestStoplossFiresWhenMarkBreachesFloor(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	_ = stop.Update(testEvidence(100, 100, 0.02, 0.03, 0.0001))
	_ = stop.Update(testEvidence(110, 100, 0.02, 0.03, 0.0001))
	breached := stop.Update(testEvidence(101, 100, 0.02, 0.03, 0.0001))

	if breached.Action != "stop" {
		t.Fatalf("want stop after breach, got %s (%s)", breached.Action, breached.Reason)
	}
}

/*
TestStoplossTakeProfitNearPeakWithDeadForward fires TP near peak with dead forward.
*/
func TestStoplossTakeProfitNearPeakWithDeadForward(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	_ = stop.Update(testEvidence(100, 100, 0.01, 0.04, 0.005))
	_ = stop.Update(testEvidence(108, 100, 0.01, 0.03, 0.005))
	profit := stop.Update(testEvidence(107.5, 100, 0.01, -0.01, 0.005))

	if profit.Action != "take_profit" {
		t.Fatalf("want take_profit, got %s (%s)", profit.Action, profit.Reason)
	}
}

/*
TestStoplossBindLivesAtEntry proves fill-time Bind publishes a stop without σ.
*/
func TestStoplossBindLivesAtEntry(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	stop.Bind(100, 0.002)

	if !stop.Armed() || stop.StopPrice() <= 0 || stop.StopReturn != -0.002 {
		t.Fatalf("bound stop invalid: armed=%v price=%v return=%v",
			stop.Armed(), stop.StopPrice(), stop.StopReturn)
	}
}

/*
TestStoplossLivesOnSpreadAlone keeps an open lot protected from book width.
*/
func TestStoplossLivesOnSpreadAlone(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	first := stop.Update(StopEvidence{
		Symbol: "AAA/USD", Mark: 100.1, Entry: 100, Spread: 0.001, Present: true,
	})

	if first.Action != "hold" || !stop.armed || stop.StopPrice() <= 0 {
		t.Fatalf("spread-only must live: action=%s armed=%v", first.Action, stop.armed)
	}
}

/*
TestStoplossLockedFloorRatchetsUnderCalibratedForecast proves a peak inside
the forecast band can leave −Inf under signal-share Weight.
*/
func TestStoplossLockedFloorRatchetsUnderCalibratedForecast(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	_ = stop.Update(testEvidence(100, 100, 0.02, 0.05, 0.0004))
	lift := stop.Update(testEvidence(104, 100, 0.02, 0.05, 0.0004))

	if math.IsInf(lift.LockedFloor, -1) || lift.LockedFloor <= 0 {
		t.Fatalf("calibrated peak must lock floor: trail=%v weight=%v peak=%v floor=%v",
			lift.TrailDistance, lift.Weight, lift.PeakReturn, lift.LockedFloor)
	}
}

/*
TestStoplossUpdateFreezesUnderRetreat ensures RetreatPressure freezes geometry.
*/
func TestStoplossUpdateFreezesUnderRetreat(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	stop.Bind(100, 0.002)
	_ = stop.Update(StopEvidence{
		Symbol: "AAA/USD", Mark: 100, Entry: 100,
		Uncertainty: 0.02, ExpectedReturn: 0.05, RetreatPressure: 0.9, Present: true,
	})
	adverse := stop.Update(StopEvidence{
		Symbol: "AAA/USD", Mark: 98.5, Entry: 100,
		Uncertainty: 0.02, ExpectedReturn: 0.05, RetreatPressure: 0.9, Present: true,
	})

	if adverse.Action != "hold" || !math.IsInf(stop.LockedFloor, -1) || adverse.MarkReturn >= 0 {
		t.Fatalf("retreat freeze failed: action=%s floor=%v mark=%v",
			adverse.Action, stop.LockedFloor, adverse.MarkReturn)
	}
}

/*
TestStoplossObserveMarkDoesNotEmitStop keeps tick geometry silent for exits.
*/
func TestStoplossObserveMarkDoesNotEmitStop(t *testing.T) {
	t.Parallel()

	stop := NewStoploss(context.Background())
	stop.Bind(100, 0.01)
	stop.ObserveMark(98)

	if stop.Action == "stop" || stop.Action == "take_profit" || stop.MarkReturn >= 0 {
		t.Fatalf("ObserveMark must not exit: action=%s mark=%v", stop.Action, stop.MarkReturn)
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
	frozen := stop.Update(StopEvidence{Symbol: "AAA/USD", Present: false})

	if frozen.Action != "hold" ||
		frozen.LockedFloor != live.LockedFloor ||
		frozen.Weight != live.Weight ||
		frozen.MarkReturn != live.MarkReturn {
		t.Fatalf("freeze mutated surface: live=%+v frozen=%+v", live, frozen)
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

	for index := 0; b.Loop(); index++ {
		evidence.Mark = 100 + float64(index%50)*0.1
		_ = stop.Update(evidence)
	}
}
