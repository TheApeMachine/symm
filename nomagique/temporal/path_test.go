package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestPath(t *testing.T) {
	Convey("Given timestamped values beyond a path's capacity", t, func() {
		path := nmtypes.NewStream(Path, types.Frame{})

		for index := range 4 {
			_, err := path.Step(pathObservation(100+float64(index), int64(index), 3))
			So(err, ShouldBeNil)
		}

		Convey("It should retain the newest observations in event order", func() {
			state := path.Project()
			timestamp, value, found := PathSample(&state, 0)

			So(found, ShouldBeTrue)
			So(timestamp, ShouldEqual, int64(1))
			So(value, ShouldEqual, 101.0)

			timestamp, value, found = PathSample(&state, 2)
			So(found, ShouldBeTrue)
			So(timestamp, ShouldEqual, int64(3))
			So(value, ShouldEqual, 103.0)
		})
	})

	Convey("Given a regressing timestamp", t, func() {
		path := nmtypes.NewStream(Path, types.Frame{})
		_, err := path.Step(pathObservation(100, 2, 2))
		So(err, ShouldBeNil)
		_, err = path.Step(pathObservation(101, 1, 2))

		So(err, ShouldNotBeNil)
	})
}

func pathObservation(value float64, timestamp int64, capacity int) types.Frame {
	input := types.Frame{}
	input.Put(nmtypes.SampleValue, value)
	input.Put(SymbolUnixSec, 0)
	input.Put(SymbolUnixNsec, float64(timestamp))
	input.Put(nmtypes.Span, float64(capacity))

	return input
}

func BenchmarkPath(benchmark *testing.B) {
	path := types.NewStream(Path, types.Frame{})
	input := pathObservation(100, 1, MaxPathSamples)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = path.Step(input)
	}
}
