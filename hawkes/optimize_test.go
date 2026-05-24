package hawkes

import (
	"testing"
	"time"
)

func TestOptimizeBivariateFindsValidFit(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 12, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(20*time.Millisecond), 10, 45*time.Millisecond)
	horizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)

	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if !ok {
		t.Fatal("expected fit context")
	}

	fit := optimizeBivariate(buyEvents, sellEvents, horizon, context, BivariateFit{})

	if fit.MuBuy <= 0 {
		t.Fatalf("expected optimizer fit, got %+v", fit)
	}
}

func BenchmarkOptimizeBivariate(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 12, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(20*time.Millisecond), 10, 45*time.Millisecond)
	horizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)
	context, _ := newFitContext(buyEvents, sellEvents, horizon)

	b.ReportAllocs()

	for b.Loop() {
		optimizeBivariate(buyEvents, sellEvents, horizon, context, BivariateFit{})
	}
}
