package strategy

import (
	"testing"

	"github.com/theapemachine/symm/types"
)

/*
TestClearScoreNativeStop is certain when Stoploss already selected stop.
*/
func TestClearScoreNativeStop(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{Action: "stop", PeakReturn: 0.1, TrailDistance: 0.05}
	got := clearScore(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got != 1 {
		t.Fatalf("want 1, got %v", got)
	}
}

/*
TestClearScoreNativeTakeProfit is certain when Stoploss selected take_profit.
*/
func TestClearScoreNativeTakeProfit(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{Action: "take_profit", PeakReturn: 0.1, TrailDistance: 0.05}
	got := clearScore(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got != 1 {
		t.Fatalf("want 1, got %v", got)
	}
}

/*
TestClearScoreMissingStopDoesNotInvent clears wait optionality.
*/
func TestClearScoreMissingStopDoesNotInvent(t *testing.T) {
	t.Parallel()

	if got := clearScore(nil, types.Forecasts{}); got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}

/*
TestClearScorePositiveForwardBlocksTPClear keeps score at zero while the
path still expects lift.
*/
func TestClearScorePositiveForwardBlocksTPClear(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold", PeakReturn: 0.1, MarkReturn: 0.09, TrailDistance: 0.05,
	}
	got := clearScore(stop, types.Forecasts{ExpectedReturn: 0.02})

	if got != 0 {
		t.Fatalf("want 0 when forward return positive, got %v", got)
	}
}

/*
TestClearScoreProximityMapsTrailBand scales clear mass by peak proximity.
*/
func TestClearScoreProximityMapsTrailBand(t *testing.T) {
	t.Parallel()

	stop := &types.Stoploss{
		Action: "hold", PeakReturn: 0.1, MarkReturn: 0.075, TrailDistance: 0.05,
	}
	got := clearScore(stop, types.Forecasts{ExpectedReturn: -0.01})

	if got < 0.499 || got > 0.501 {
		t.Fatalf("want ~0.5, got %v", got)
	}
}

/*
TestShouldRotateHighClearForcesWait even when raw surplus is positive.
*/
func TestShouldRotateHighClearForcesWait(t *testing.T) {
	t.Parallel()

	if shouldRotate(0.08, 0.03, 0.01, 0.9) {
		t.Fatal("high clear score must force wait")
	}
}

/*
TestShouldRotateLowClearAllowsDisplace when edge clears exit after wait charge.
*/
func TestShouldRotateLowClearAllowsDisplace(t *testing.T) {
	t.Parallel()

	if !shouldRotate(0.08, 0.03, 0.01, 0) {
		t.Fatal("zero clear score with positive surplus must rotate")
	}
}

/*
TestBestRotationPicksLargestAdvantage across eligible incumbents.
*/
func TestBestRotationPicksLargestAdvantage(t *testing.T) {
	t.Parallel()

	index, found := bestRotation(0.10, []Incumbent{
		{Symbol: "A", HoldUtility: 0.08, ExitCost: 0.01, ClearScore: 0},
		{Symbol: "B", HoldUtility: 0.02, ExitCost: 0.01, ClearScore: 0},
	})

	if !found || index != 1 {
		t.Fatalf("want incumbent B, got found=%v index=%d", found, index)
	}
}

/*
BenchmarkClearScore measures path-to-score conversion.
*/
func BenchmarkClearScore(b *testing.B) {
	stop := &types.Stoploss{
		Action: "hold", PeakReturn: 0.1, MarkReturn: 0.08, TrailDistance: 0.05,
	}
	forecast := types.Forecasts{ExpectedReturn: -0.01}

	b.ReportAllocs()

	for b.Loop() {
		_ = clearScore(stop, forecast)
	}
}
