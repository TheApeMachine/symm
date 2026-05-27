package trader

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
)

func TestSynchronizedLogReturns(t *testing.T) {
	originalBar := config.System.CorrelationBarSeconds
	config.System.CorrelationBarSeconds = 10
	t.Cleanup(func() { config.System.CorrelationBarSeconds = originalBar })

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003, 0.002, -0.002,
		0.001, 0.004, 0.003, 0.002, -0.001, 0.002, 0.001, 0.003,
	}

	left := buildPriceSamples(100, returns, interval, start)
	right := buildPriceSamples(50, returns, interval, start)

	leftGrid, rightGrid, ok := synchronizedLogReturns(left, right, interval)

	if !ok {
		t.Fatal("expected synchronized returns")
	}

	if len(leftGrid) < config.System.MinCorrelationSamples {
		t.Fatalf("expected at least %d returns, got %d", config.System.MinCorrelationSamples, len(leftGrid))
	}

	correlation := pearson(leftGrid, rightGrid)

	if math.Abs(correlation-1) > 1e-6 {
		t.Fatalf("expected synchronized correlation ~1, got %v", correlation)
	}
}

func buildPriceSamples(basePrice float64, returns []float64, interval time.Duration, start time.Time) []priceSample {
	samples := make([]priceSample, 0, len(returns)+1)
	price := basePrice
	samples = append(samples, priceSample{at: start, price: price})

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		samples = append(samples, priceSample{
			at:    start.Add(time.Duration(index+1) * interval),
			price: price,
		})
	}

	return samples
}

func BenchmarkSynchronizedLogReturns(b *testing.B) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003, 0.002, -0.002,
		0.001, 0.004, 0.003, 0.002, -0.001, 0.002, 0.001, 0.003,
	}

	left := buildPriceSamples(100, returns, interval, start)
	right := buildPriceSamples(50, returns, interval, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _ = synchronizedLogReturns(left, right, interval)
	}
}
