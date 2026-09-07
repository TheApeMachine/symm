package sensorium

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCompressorTransitionProbability(t *testing.T) {
	Convey("Given a two-position segment and observations across batch boundaries", t, func() {
		compressor := NewCompressor(2)
		So(compressor.TransitionProbability(1, 0, 2, 1), ShouldEqual, 0)
		compressor.Filter([]int64{1, 2, 1}, []int64{0, 1, 2})
		So(compressor.TransitionProbability(1, 0, 2, 1), ShouldEqual, 0.5)

		Convey("The next contiguous batch resolves the previous terminal occurrence", func() {
			compressor.Filter([]int64{2}, []int64{3})
			So(compressor.TransitionProbability(1, 0, 2, 1), ShouldEqual, 1)
		})

		Convey("A competing successor splits observed outgoing mass", func() {
			compressor.Filter([]int64{3}, []int64{3})
			So(compressor.TransitionProbability(1, 0, 2, 1), ShouldEqual, 0.5)
			So(compressor.TransitionProbability(1, 0, 3, 1), ShouldEqual, 0.5)
			So(compressor.TransitionProbability(1, 0, 4, 1), ShouldEqual, 0)
		})

		Convey("A gap creates no unobserved transition", func() {
			compressor.Filter([]int64{2}, []int64{5})
			So(compressor.TransitionProbability(1, 0, 2, 1), ShouldEqual, 0.5)
		})
	})
}

func BenchmarkCompressorTransitionProbability(b *testing.B) {
	compressor := NewCompressor(2)
	compressor.Filter([]int64{1, 2, 1, 3}, []int64{0, 1, 2, 3})
	b.ReportAllocs()

	for b.Loop() {
		if compressor.TransitionProbability(1, 0, 2, 1) != 0.5 {
			b.Fatal("unexpected transition frequency")
		}
	}
}
