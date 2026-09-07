package adaptive_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestWindowNext(t *testing.T) {
	Convey("Fixed-field window state matches the independent reference across three regimes", t, func() {
		tests.CheckWindow(t, adaptive.NewWindow())
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
