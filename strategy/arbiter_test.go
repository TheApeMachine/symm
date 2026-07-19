package strategy

import (
	"testing"

	"github.com/theapemachine/symm/types"
)

/*
TestClearProbabilityNativeStop is certain when Stoploss already selected stop.
*/
func TestClearProbabilityNativeStop(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "stop",
		Trail:  types.Trail{PeakReturn: 0.1, TrailDistance: 0.05},
	}
	got := NewRotate().Clear(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got != 1 {
		t.Fatalf("want 1, got %v", got)
	}
}

/*
TestClearProbabilityNativeTakeProfit is certain when Stoploss selected take_profit.
*/
func TestClearProbabilityNativeTakeProfit(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "take_profit",
		Trail:  types.Trail{PeakReturn: 0.1, TrailDistance: 0.05},
	}
	got := NewRotate().Clear(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got != 1 {
		t.Fatalf("want 1, got %v", got)
	}
}

/*
TestClearProbabilityMissingStopDoesNotInvent clears wait optionality.
*/
func TestClearProbabilityMissingStopDoesNotInvent(t *testing.T) {
	t.Parallel()

	if got := NewRotate().Clear(nil, types.Forecasts{}); got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}

/*
TestClearProbabilityPositiveForwardBlocksTPClear keeps probability at zero
while the path still expects lift.
*/
func TestClearProbabilityPositiveForwardBlocksTPClear(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold",
		Trail: types.Trail{
			PeakReturn: 0.1, MarkReturn: 0.09, TrailDistance: 0.05,
		},
	}
	got := NewRotate().Clear(stop, types.Forecasts{
		ExpectedReturn: 0.02, Calibrated: true, Confidence: 1,
	})

	if got != 0 {
		t.Fatalf("want 0 when forward return positive, got %v", got)
	}
}

/*
TestClearProbabilityUncalibratedIsZero refuses inventing wait mass.
*/
func TestClearProbabilityUncalibratedIsZero(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold",
		Trail: types.Trail{
			PeakReturn: 0.1, MarkReturn: 0.075, TrailDistance: 0.05,
		},
	}
	got := NewRotate().Clear(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got != 0 {
		t.Fatalf("want 0 without calibration, got %v", got)
	}
}

/*
TestClearProbabilityProximityTimesConfidence maps trail band through Confidence.
*/
func TestClearProbabilityProximityTimesConfidence(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold",
		Trail: types.Trail{
			PeakReturn: 0.1, MarkReturn: 0.075, TrailDistance: 0.05,
		},
	}
	got := NewRotate().Clear(stop, types.Forecasts{
		ExpectedReturn: -0.01,
		Calibrated:     true,
		Confidence:     1,
	})

	if got < 0.499 || got > 0.501 {
		t.Fatalf("want ~0.5, got %v", got)
	}
}

/*
TestClearProbabilityResidualSkillScales hazard by σ/(√MSE+σ).
*/
func TestClearProbabilityResidualSkillScales(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold",
		Trail: types.Trail{
			PeakReturn: 0.1, MarkReturn: 0.075, TrailDistance: 0.05,
		},
	}
	got := NewRotate().Clear(stop, types.Forecasts{
		ExpectedReturn: -0.01,
		Calibrated:     true,
		Confidence:     1,
		Uncertainty:    0.02,
		IncrementalMSE: 0.0004, // √MSE = 0.02 → skill = 0.5
	})

	if got < 0.249 || got > 0.251 {
		t.Fatalf("want ~0.25, got %v", got)
	}

	// Higher RMSE must lower clear score (σ/(rmse+σ) shrinks as rmse grows).
	worse := NewRotate().Clear(stop, types.Forecasts{
		ExpectedReturn: -0.01,
		Calibrated:     true,
		Confidence:     1,
		Uncertainty:    0.02,
		IncrementalMSE: 0.01, // √MSE = 0.1 → skill ≈ 0.167 → clear ≈ 0.083
	})

	if worse >= got {
		t.Fatalf("higher residual must lower clear score: base=%v worse=%v", got, worse)
	}
}

/*
TestShouldRotateHighClearForcesWait even when raw surplus is positive.
*/
func TestShouldRotateHighClearForcesWait(t *testing.T) {
	t.Parallel()

	if NewRotate().Gate(0.08, 0.03, 0.01, 0.9) {
		t.Fatal("high clear probability must force wait")
	}
}

/*
TestShouldRotateLowClearAllowsDisplace when edge clears exit after wait charge.
*/
func TestShouldRotateLowClearAllowsDisplace(t *testing.T) {
	t.Parallel()

	if !NewRotate().Gate(0.08, 0.03, 0.01, 0) {
		t.Fatal("zero clear probability with positive surplus must rotate")
	}
}

/*
TestBestRotationPicksLargestAdvantage across eligible incumbents.
*/
func TestBestRotationPicksLargestAdvantage(t *testing.T) {
	t.Parallel()

	index, found := NewRotate().Best(0.10, []Incumbent{
		{Symbol: "A", HoldUtility: 0.08, ExitCost: 0.01, ClearProb: 0},
		{Symbol: "B", HoldUtility: 0.02, ExitCost: 0.01, ClearProb: 0},
	})

	if !found || index != 1 {
		t.Fatalf("want incumbent B, got found=%v index=%d", found, index)
	}
}

/*
BenchmarkClearProbability measures path-to-probability conversion.
*/
func BenchmarkClearProbability(b *testing.B) {
	stop := &types.Stoploss{
		Action: "hold",
		Trail: types.Trail{
			PeakReturn: 0.1, MarkReturn: 0.08, TrailDistance: 0.05,
		},
	}
	forecast := types.Forecasts{
		ExpectedReturn: -0.01,
		Calibrated:     true,
		Confidence:     0.8,
		Uncertainty:    0.01,
		IncrementalMSE: 0.0001,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewRotate().Clear(stop, forecast)
	}
}
