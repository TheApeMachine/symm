package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDepthflowDynamics(t *testing.T) {
	Convey("Given weighted and touch histories", t, func() {
		weighted := []float64{0.6, 0.55, 0.58, 0.62}
		level1 := []float64{0.2, 0.18, 0.22, 0.19}

		Convey("It should derive spoof contrast from medians", func() {
			contrast := spoofContrastRatio(weighted, level1)

			So(contrast, ShouldBeGreaterThan, 0)
			So(contrast, ShouldBeLessThan, 1)
		})
	})

	Convey("Given weighted and flat histories", t, func() {
		weighted := []float64{0.6, 0.55, 0.58, 0.62}
		flat := []float64{0.2, 0.18, 0.22, 0.19}

		Convey("It should derive thinning gate from medians", func() {
			gate := thinningDepthGate(weighted, flat)

			So(gate, ShouldBeGreaterThan, 0)
			So(gate, ShouldBeLessThan, 1)
		})
	})
}

func BenchmarkDepthflowDynamics(b *testing.B) {
	weighted := []float64{0.6, 0.55, 0.58, 0.62, 0.59, 0.61}
	level1 := []float64{0.2, 0.18, 0.22, 0.19, 0.21, 0.2}
	flat := []float64{0.25, 0.24, 0.26, 0.23, 0.25, 0.24}

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = spoofContrastRatio(weighted, level1)
		_ = thinningDepthGate(weighted, flat)
		_ = loadedPressureScale(0.6, 0.4)
	}
}
