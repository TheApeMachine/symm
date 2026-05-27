package engine

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestFuseMeasurements(t *testing.T) {
	joint, count := FuseMeasurements([]Measurement{
		{Source: "hawkes", Confidence: 0.8},
		{Source: "depthflow", Confidence: 0.6},
	})

	if count != 2 {
		t.Fatalf("expected two sources, got %d", count)
	}

	if joint <= 0 || joint >= 1 {
		t.Fatalf("expected joint confidence in (0,1), got %v", joint)
	}

	_, singleCount := FuseMeasurements([]Measurement{
		{Source: "pumpdump", Confidence: 0.8},
	})

	if singleCount != 1 {
		t.Fatalf("expected one source, got %d", singleCount)
	}

	if joint <= 0 {
		t.Fatalf("expected positive joint confidence, got %v", joint)
	}
}

func TestFuseMeasurementsEmpty(t *testing.T) {
	joint, count := FuseMeasurements(nil)

	if joint != 0 || count != 0 {
		t.Fatalf("expected zero fusion on empty input, got joint=%v count=%d", joint, count)
	}
}

func TestFuseMeasurementsDistinctSourcesOnly(t *testing.T) {
	_, count := FuseMeasurements([]Measurement{
		{Source: "hawkes", Confidence: 0.8, Pairs: []asset.Pair{{Wsname: "BTC/EUR"}}},
		{Source: "hawkes", Confidence: 0.7, Pairs: []asset.Pair{{Wsname: "BTC/EUR"}}},
	})

	if count != 1 {
		t.Fatalf("expected duplicate sources to count once, got %d", count)
	}
}
