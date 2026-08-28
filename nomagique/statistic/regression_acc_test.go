package statistic

import "testing"

/*
BenchmarkRegressionAccumulatorFit guards Fit's allocation cost: it is called
once per resident row in a prequential walk (hundreds of times per candidate
pair, per estimate cycle), so any allocation added here is a direct multiplier
on process-wide memory pressure.
*/
func BenchmarkRegressionAccumulatorFit(b *testing.B) {
	accumulator := NewRegressionAccumulator(4)

	for i := 0; i < 10; i++ {
		accumulator.Add([]float64{1, float64(i), float64(i) * 2, float64(i) * 3}, float64(i)*1.5)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = accumulator.Fit()
	}
}
