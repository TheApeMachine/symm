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
		output := Lag("previous", "current")(paired)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})
}

func BenchmarkLag(benchmark *testing.B) {
	left := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 110, 99, 118.8})
	right := hayashiPath([]int64{0, 2, 4, 6}, []float64{100, 100, 110, 99})
	paired := pairPaths(left, right)
	paired.Put(SymbolLagSpacing, 2)
	paired.Put(SymbolMaximumLag, 2)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = Lag("previous", "current")(paired)
	}
}
