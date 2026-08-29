package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLag(t *testing.T) {
	Convey("Given a lag scan over two retained paths", t, func() {
		left := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 110, 99, 118.8})
		right := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 100, 110, 99})
		paired := pairPaths(left, right)
		paired.Put(SymbolLagSpacing, 2)
		paired.Put(SymbolMaximumLag, 2)
		output := paired
		Lag("previous", "current")(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolBestLagSupport), ShouldEqual, 2.0)
		So(output.MustGet(SymbolNeighborLow), ShouldAlmostEqual, -0.02925142293097253, 1e-12)
		So(output.MustGet(SymbolNeighborHigh), ShouldAlmostEqual, -0.010041929691623815, 1e-12)
	})
}

func BenchmarkLag(benchmark *testing.B) {
	left := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 110, 99, 118.8})
	right := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 100, 110, 99})
	paired := pairPaths(left, right)
	paired.Put(SymbolLagSpacing, 2)
	paired.Put(SymbolMaximumLag, 2)
	lagPrimitive := Lag("previous", "current")
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		output := paired
		lagPrimitive(&output)
	}
}
