package market

import "testing"

func TestWeightedDepthImbalanceDetectsSpoofSkew(t *testing.T) {
	bids := []BookLevel{
		{Price: 100, Volume: 1},
		{Price: 99.5, Volume: 100},
	}
	asks := []BookLevel{
		{Price: 100.1, Volume: 10},
	}

	flatImbalance, ok := FlatDepthImbalance(bids, asks)

	if !ok {
		t.Fatal("expected flat imbalance")
	}

	level1Imbalance, ok := Level1Imbalance(bids, asks)

	if !ok {
		t.Fatal("expected level1 imbalance")
	}

	if !IsSpoofSkew(flatImbalance, level1Imbalance, 0.5, -0.1) {
		t.Fatalf("expected flat spoof detection, flat=%v level1=%v", flatImbalance, level1Imbalance)
	}

	weightedImbalance, ok := WeightedDepthImbalance(bids, asks, 100.05, 1000)

	if !ok {
		t.Fatal("expected weighted imbalance")
	}

	if IsSpoofSkew(weightedImbalance, level1Imbalance, 0.5, -0.1) {
		t.Fatalf("expected decay to suppress deep spoof, weighted=%v level1=%v", weightedImbalance, level1Imbalance)
	}

	flatBid := []BookLevel{
		{Price: 100, Volume: 80},
		{Price: 99.5, Volume: 20},
	}

	weightedTouch, ok := WeightedDepthImbalance(flatBid, asks, 100.05, 1000)

	if !ok {
		t.Fatal("expected touch-weighted imbalance")
	}

	level1Touch, ok := Level1Imbalance(flatBid, asks)

	if !ok {
		t.Fatal("expected level1 touch imbalance")
	}

	if IsSpoofSkew(weightedTouch, level1Touch, 0.5, -0.1) {
		t.Fatalf("expected aligned touch to pass, weighted=%v level1=%v", weightedTouch, level1Touch)
	}
}

func TestIsSpoofSkew(t *testing.T) {
	if !IsSpoofSkew(0.85, -0.2, 0.5, -0.1) {
		t.Fatal("expected buy-side spoof detection")
	}

	if IsSpoofSkew(0.85, 0.3, 0.5, -0.1) {
		t.Fatal("expected aligned touch to pass")
	}
}

func TestFlatDepthImbalance(t *testing.T) {
	imbalance, ok := FlatDepthImbalance(
		[]BookLevel{{Price: 100, Volume: 80}, {Price: 99.5, Volume: 20}},
		[]BookLevel{{Price: 100.1, Volume: 20}},
	)

	if !ok || imbalance <= 0 {
		t.Fatalf("expected positive flat imbalance, got %v ok=%v", imbalance, ok)
	}
}
