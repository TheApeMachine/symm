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
