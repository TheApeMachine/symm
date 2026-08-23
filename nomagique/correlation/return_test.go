package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestReturn(t *testing.T) {
	Convey("Given a retained positive price path", t, func() {
		path := hayashiPath([]int64{0, 1}, []float64{100, 110})
		_, output, err := Return(types.Frame{}, path)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolReturn), ShouldBeGreaterThan, 0.0)
		So(output.MustGet(SymbolMagnitude), ShouldEqual, output.MustGet(SymbolReturn))
	})
}

func BenchmarkReturn(benchmark *testing.B) {
	path := hayashiPath([]int64{0, 1}, []float64{100, 110})
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = Return(types.Frame{}, path)
	}
}
