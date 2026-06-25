package statutil

import "testing"

func BenchmarkWindowDepth(b *testing.B) {
	stamps := make([]float64, 128)

	for index := range stamps {
		stamps[index] = float64(index) * 0.05
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = WindowDepth(stamps)
	}
}

func BenchmarkSampleBudgetFromCadence(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = SampleBudgetFromCadence(0.05)
	}
}
