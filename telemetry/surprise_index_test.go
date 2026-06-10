package telemetry

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSurpriseIndexRecord(t *testing.T) {
	Convey("Given a fresh surprise index", t, func() {
		index := NewSurpriseIndex()

		Convey("It should default to one when empty", func() {
			So(index.Index(), ShouldEqual, 1)
		})

		Convey("It should median normalized surprise ratios", func() {
			index.Record("fluid", 4, 2)
			index.Record("hawkes", 2, 2)

			So(index.Index(), ShouldEqual, 1.5)
		})
	})
}

func BenchmarkSurpriseIndexRecord(b *testing.B) {
	index := NewSurpriseIndex()

	b.ReportAllocs()

	for b.Loop() {
		index.Record("fluid", 3, 2)
	}
}
