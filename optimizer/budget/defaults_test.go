package budget

import (
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultTuneOptions(t *testing.T) {
	Convey("Given zero workers", t, func() {
		options := DefaultTuneOptions(0)

		Convey("It should default workers to CPU count", func() {
			So(options.Workers, ShouldEqual, runtime.NumCPU())
		})
	})

	Convey("Given explicit workers", t, func() {
		options := DefaultTuneOptions(4)

		Convey("It should preserve the worker count", func() {
			So(options.Workers, ShouldEqual, 4)
		})
	})
}

func BenchmarkDefaultTuneOptions(b *testing.B) {
	for b.Loop() {
		_ = DefaultTuneOptions(4)
	}
}
