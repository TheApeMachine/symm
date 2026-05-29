package correlation

import (
	"math"
	"testing"
	"time"
)

func TestHayashiYoshidaCorrelationUsesAsynchronousIntervals(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	left := []PriceSample{
		{At: start, Price: 100},
		{At: start.Add(20 * time.Second), Price: 101},
		{At: start.Add(40 * time.Second), Price: 102},
		{At: start.Add(60 * time.Second), Price: 103},
	}
	right := []PriceSample{
		{At: start.Add(10 * time.Second), Price: 50},
		{At: start.Add(30 * time.Second), Price: 50.5},
		{At: start.Add(50 * time.Second), Price: 51},
		{At: start.Add(70 * time.Second), Price: 51.5},
	}

	gridLeft, _, gridOK := SynchronizedLogReturns(left, right, 10*time.Second)

	if gridOK && len(gridLeft) > 0 {
		t.Fatalf("expected grid synchronization to drop asynchronous bars, got %d", len(gridLeft))
	}

	correlation, ok := HayashiYoshidaCorrelation(left, right)

	if !ok {
		t.Fatal("expected HY correlation")
	}

	if correlation <= 0.9 {
		t.Fatalf("expected strong asynchronous correlation, got %v", correlation)
	}
}

func TestHayashiYoshidaCorrelationRejectsStaleIntervals(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	left := []PriceSample{
		{At: start, Price: 100},
		{At: start.Add(maxHayashiYoshidaInterval + time.Second), Price: 101},
	}
	right := []PriceSample{
		{At: start.Add(time.Second), Price: 50},
		{At: start.Add(2 * time.Second), Price: 50.5},
		{At: start.Add(3 * time.Second), Price: 51},
	}

	if correlation, ok := HayashiYoshidaCorrelation(left, right); ok {
		t.Fatalf("expected stale interval rejection, got %v", correlation)
	}
}

func TestHayashiYoshidaCorrelationAllocatesNoHeap(t *testing.T) {
	start := time.Unix(0, 0)
	left := make([]PriceSample, 32)
	right := make([]PriceSample, 32)

	for index := range left {
		left[index] = PriceSample{
			At:    start.Add(time.Duration(index) * 10 * time.Second),
			Price: 100 * math.Exp(float64(index)*0.001),
		}
		right[index] = PriceSample{
			At:    start.Add(time.Duration(index)*10*time.Second + 5*time.Second),
			Price: 50 * math.Exp(float64(index)*0.001),
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := HayashiYoshidaCorrelation(left, right); !ok {
			t.Fatal("expected correlation")
		}
	})

	if allocs != 0 {
		t.Fatalf("expected zero allocations, got %v", allocs)
	}
}

func TestShiftPriceSamples(t *testing.T) {
	start := time.Unix(0, 0)
	samples := []PriceSample{{At: start, Price: 100}}
	shifted := ShiftPriceSamples(samples, time.Second)

	if len(shifted) != 1 || !shifted[0].At.Equal(start.Add(time.Second)) {
		t.Fatalf("unexpected shifted samples: %+v", shifted)
	}

	if !samples[0].At.Equal(start) {
		t.Fatal("expected source samples unchanged")
	}
}

func BenchmarkHayashiYoshidaCorrelation(b *testing.B) {
	start := time.Unix(0, 0)
	left := make([]PriceSample, 128)
	right := make([]PriceSample, 128)

	for index := range left {
		left[index] = PriceSample{
			At:    start.Add(time.Duration(index) * 10 * time.Second),
			Price: 100 * math.Exp(float64(index)*0.001),
		}
		right[index] = PriceSample{
			At:    start.Add(time.Duration(index)*10*time.Second + 5*time.Second),
			Price: 50 * math.Exp(float64(index)*0.001),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := HayashiYoshidaCorrelation(left, right); !ok {
			b.Fatal("expected correlation")
		}
	}
}
