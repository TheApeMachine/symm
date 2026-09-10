package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestWindowNext(t *testing.T) {
	Convey("Next and Observe share the same recurrence", t, func() {
		stream := adaptive.NewWindow()
		direct := adaptive.NewWindow()
		values := []float64{1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
		output := tests.CollectSeq(stream.Next(transport.Values(values...)))
		So(stream.Error(), ShouldBeNil)
		So(len(output), ShouldEqual, len(values))

		for index, value := range values {
			reading := direct.Observe(value)
			So(output[index].Capacity, ShouldEqual, reading.Capacity)
			So(output[index].ShedRatio, ShouldEqual, reading.ShedRatio)
			So(output[index].All.Mean, ShouldEqual, reading.All.Mean)
			So(output[index].All.Count, ShouldEqual, reading.All.Count)
		}
	})
}

func TestWindowObserve(t *testing.T) {
	Convey("A stationary window expands without allocating observation records", t, func() {
		window := adaptive.NewWindow()

		for index := range 100 {
			reading := window.Observe(7)
			So(reading.Capacity, ShouldEqual, index+3)
			So(reading.ShedRatio, ShouldEqual, 1)
		}

		allocations := testing.AllocsPerRun(100, func() { window.Observe(7) })
		So(allocations, ShouldEqual, 0)
	})
}

func BenchmarkWindowObserve(b *testing.B) {
	window := adaptive.NewWindow()
	values := [...]float64{1, 3, 5, 7, -2, -4, -6, -8}
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		window.Observe(values[index%len(values)])
		index++
	}
}
