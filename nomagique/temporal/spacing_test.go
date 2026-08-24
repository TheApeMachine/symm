package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestSpacing(t *testing.T) {
	Convey("Given an irregular retained path", t, func() {
		path := types.NewStream(Path(""), types.Frame{})

		for _, timestamp := range []int64{0, 2, 5, 9} {
			output := path.Step(pathObservation(100, timestamp, 4))
			So(output.Err, ShouldBeNil)
		}

		output := DefaultSeries.Spacing(path.Project())

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolSpacingReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolSpacingNanos), ShouldEqual, 3.0)
	})
}

func BenchmarkSpacing(benchmark *testing.B) {
	path := types.NewStream(Path(""), types.Frame{})

	for timestamp := range MaxPathSamples {
		_ = path.Step(pathObservation(100, int64(timestamp), MaxPathSamples))
	}

	state := path.Project()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		frame := state
		_ = DefaultSeries.Spacing(frame)
	}
}
