package probability_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCalibratorRetention(t *testing.T) {
	Convey("Rank is computed against the prior window, then the sample is retained", t, func() {
		checkCalibrator(types.Value[float64, float64](probability.NewCalibrator(types.Value[[]float64, []float64](sequence.NewTail[float64](4)))), 4)
		checkCalibrator(types.Value[float64, float64](probability.NewCalibrator(nil)), 0)
	})
}

func checkCalibrator(node types.Value[float64, float64], capacity int) {
	history := []float64{}

	for _, sample := range []float64{10, 20, 30, 15, 40, 50, 1, 5, 99, 4} {
		want := 0.5

		if len(history) > 0 {
			hits := 0.0
			for _, prior := range history {
				if prior > sample {
					hits++
				}
			}
			want = hits / float64(len(history))
		}

		got := node(sample)

		So(got, ShouldEqual, want)
		history = append(history, sample)

		if capacity > 0 && len(history) > capacity {
			history = history[len(history)-capacity:]
		}
	}
}
