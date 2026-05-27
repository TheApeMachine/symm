package engine

import (
	"math"
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

	expectedJoint := math.Sqrt(0.8 * 0.6)

	if math.Abs(joint-expectedJoint) > 1e-9 {
		t.Fatalf("expected geometric source confidence %v, got %v", expectedJoint, joint)
	}

	singleJoint, singleCount := FuseMeasurements([]Measurement{
		{Source: "pumpdump", Confidence: 0.8},
	})

	if singleCount != 1 {
		t.Fatalf("expected one source, got %d", singleCount)
	}

	if singleJoint != 0.8 {
		t.Fatalf("expected single-source confidence to pass through, got %v", singleJoint)
	}
}

func TestPerspectiveSource(t *testing.T) {
	if PerspectiveSource(PerspectiveMicrostructure) != "perspective:microstructure" {
		t.Fatal("expected microstructure perspective source")
	}

	if PerspectiveSource(PerspectiveFlow) != "perspective:flow" {
		t.Fatal("expected flow perspective source")
	}
}

func TestFuseMeasurementsEmpty(t *testing.T) {
	joint, count := FuseMeasurements(nil)

	if joint != 0 || count != 0 {
		t.Fatalf("expected zero fusion on empty input, got joint=%v count=%d", joint, count)
	}
}

func TestFuseMeasurementsDistinctSourcesOnly(t *testing.T) {
	joint, count := FuseMeasurements([]Measurement{
		{Source: "hawkes", Confidence: 0.8, Pairs: []asset.Pair{{Wsname: "BTC/EUR"}}},
		{Source: "hawkes", Confidence: 0.7, Pairs: []asset.Pair{{Wsname: "BTC/EUR"}}},
	})

	if count != 1 {
		t.Fatalf("expected duplicate sources to count once, got %d", count)
	}

	if joint != 0.8 {
		t.Fatalf("expected strongest duplicate source confidence, got %v", joint)
	}
}

func BenchmarkFuseMeasurements(b *testing.B) {
	measurements := []Measurement{
		{Source: "pumpdump", Confidence: 0.8},
		{Source: "hawkes", Confidence: 0.7},
		{Source: "depthflow", Confidence: 0.6},
		{Source: "leadlag", Confidence: 0.5},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = FuseMeasurements(measurements)
	}
}
