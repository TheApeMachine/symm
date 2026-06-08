package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSoftmaxPercentages(t *testing.T) {
	Convey("SoftmaxPercentages", t, func() {
		labels := []string{"a", "b"}
		logEv := map[string]float64{"a": math.Log(0.7), "b": math.Log(0.3)}
		p := SoftmaxPercentages(logEv, labels)
		So(p["a"]+p["b"], ShouldAlmostEqual, 100, 1e-6)
	})
}

func TestSoftmaxScores(t *testing.T) {
	Convey("Given raw logits", t, func() {
		probabilities := SoftmaxScores([]float64{1, 2, 3})

		sum := 0.0

		for _, probability := range probabilities {
			sum += probability
		}

		Convey("It should normalize to unity", func() {
			So(sum, ShouldAlmostEqual, 1.0, 1e-9)
			So(ArgmaxIndex(probabilities), ShouldEqual, 2)
		})
	})
}

func BenchmarkSoftmaxScores(b *testing.B) {
	scores := []float64{0.6, 0.4, 0.7, 0.3}

	b.ReportAllocs()

	for b.Loop() {
		_ = SoftmaxScores(scores)
	}
}

func BenchmarkSoftmaxPercentages(b *testing.B) {
	labels := make([]string, 32)
	logEv := make(map[string]float64, 32)

	for i := range labels {
		label := string(rune('a' + i))
		labels[i] = label
		logEv[label] = float64(i) * 0.1
	}

	b.ResetTimer()

	for b.Loop() {
		_ = SoftmaxPercentages(logEv, labels)
	}
}
