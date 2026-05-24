package fluid

import (
	"testing"
	"time"
)

func TestContinuitySourceDetectsHiddenAccumulation(t *testing.T) {
	prior := fieldSample{density: 10, flow: 0}
	current := fieldSample{density: 14, flow: 8}

	source := continuitySource(current, prior)

	if source <= 0 {
		t.Fatalf("expected positive source term, got %v", source)
	}
}

func TestBurgersShockRequiresVelocityJump(t *testing.T) {
	prior := fieldSample{velocity: 0.001, viscosity: 10}
	current := fieldSample{velocity: 0.02, viscosity: 10}

	shock := burgersShock(current, prior)

	if shock <= 0 {
		t.Fatal("expected positive shock term")
	}

	steady := fieldSample{velocity: 0.02, viscosity: 10}

	if burgersShock(steady, current) != 0 {
		t.Fatal("expected zero shock without velocity jump")
	}
}

func TestTrackStoreFiresOnAccumulationWithQuietVelocity(t *testing.T) {
	trackStore := NewTrackStore()
	trackStore.ApplyTicker("PUMP/EUR", 1, 500000)
	trackStore.ApplyTicker("BTC/EUR", 50000, 100)

	start := time.Unix(0, 0)

	for index := 0; index < minFieldHistory+1; index++ {
		at := start.Add(time.Duration(index) * time.Second)
		_, _ = trackStore.Sample("PUMP/EUR", 10, 1, 20, 0, 1, 0.2, at)
	}

	track := trackStore.bySymbol["PUMP/EUR"]

	for index := 0; index < minFieldHistory; index++ {
		track.sourceHistory = append(track.sourceHistory, 0.5)
		track.shockHistory = append(track.shockHistory, 0.001)
	}

	track.confidenceHistory = []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	at := start.Add(time.Duration(minFieldHistory+1) * time.Second)
	confidence, reason := trackStore.Sample("PUMP/EUR", 25, 1, 5, 0, 20, 0.9, at)

	if confidence <= 0 {
		t.Fatalf("expected fluid confidence, got %v", confidence)
	}

	if reason != "accumulation" && reason != "shock" {
		t.Fatalf("expected accumulation or shock reason, got %q", reason)
	}
}

func TestFieldConfidenceRequiresBuyPressure(t *testing.T) {
	if fieldConfidence(2, 0, -0.2, true) != 0 {
		t.Fatal("expected zero confidence without buy-side pressure")
	}

	if fieldConfidence(2, 0, 0.8, true) <= 0 {
		t.Fatal("expected accumulation confidence with quiet source term")
	}
}

func BenchmarkContinuitySource(b *testing.B) {
	prior := fieldSample{density: 10, flow: 1}
	current := fieldSample{density: 15, flow: 10}

	b.ReportAllocs()

	for b.Loop() {
		if continuitySource(current, prior) <= 0 {
			b.Fatal("expected source")
		}
	}
}
