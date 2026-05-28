package market

import "testing"

func TestDepthFillVWAPWalksLevels(t *testing.T) {
	levels := []BookLevel{
		{Price: 100, Volume: 1},
		{Price: 101, Volume: 1},
	}

	fill := DepthFillVWAP(levels, 150)

	if fill <= 100 || fill >= 101 {
		t.Fatalf("expected VWAP between 100 and 101, got %v", fill)
	}
}

func TestDepthFillVWAPPenalizesInsufficientDepth(t *testing.T) {
	levels := []BookLevel{{Price: 100, Volume: 0.5}}
	fill := DepthFillVWAP(levels, 100)

	if fill <= 100 {
		t.Fatalf("expected adverse fill above visible ask, got %v", fill)
	}
}

func TestDepthSlopeUsesCumulativeVolume(t *testing.T) {
	levels := []BookLevel{
		{Price: 100, Volume: 2},
		{Price: 99, Volume: 3},
	}

	slope := DepthSlope(levels)

	if slope <= 0 {
		t.Fatalf("expected positive depth slope, got %v", slope)
	}
}
