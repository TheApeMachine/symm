package learning

import (
	"math/rand"
	"testing"
	"time"
)

// probeBenchStep mirrors the production resonance solver's coder config
// (logic/resonance/solver.go Update) so we can measure the per-ticker cost.
func probeBenchStep(b *testing.B, features int, pace float64) {
	coder := NewPredictiveCoder(PredictiveCoderConfig{
		CustomArch: []int{features, features * 4, features * 2, features},
		MaxHorizon: 8,
		Target:     DirectionalTarget(0.01),
		Pace:       pace,
		Learn:      true,
	})

	rng := rand.New(rand.NewSource(42))
	row := make([]float64, features)
	step := int64(0)
	now := float64(time.Now().UnixNano()) / 1e9

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		for featureIndex := range row {
			row[featureIndex] = 100 + rng.NormFloat64()*5
		}

		step++
		now += 0.25

		_, err := coder.Step(PredictiveInput{
			Features:     row,
			Reference:    row[0],
			HasReference: true,
			Step:         step,
			Time:         now,
		})

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProbePredictiveCoderStep11(b *testing.B) {
	probeBenchStep(b, 11, 0.01)
}

func BenchmarkProbePredictiveCoderStep11Pace03(b *testing.B) {
	probeBenchStep(b, 11, 0.03)
}
