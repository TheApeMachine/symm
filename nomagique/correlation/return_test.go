package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReturn(t *testing.T) {
	Convey("Given a retained positive price path", t, func() {
		output := hayashiPath([]int64{0, 1}, []float64{100, 110})
		Return(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolReturn), ShouldBeGreaterThan, 0.0)
		So(output.MustGet(SymbolMagnitude), ShouldEqual, output.MustGet(SymbolReturn))
	})
}

func BenchmarkReturn(benchmark *testing.B) {
	path := hayashiPath([]int64{0, 1}, []float64{100, 110})
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		output := path
		Return(&output)
	}
}
