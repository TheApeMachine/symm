package adaptive_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestWeightedWindowNext(t *testing.T) {
	Convey("Stable support grows; changed regimes contract and regain support", t, func() {
		window, scaled := adaptive.NewWeightedWindow(), adaptive.NewWeightedWindow()
		var changes, growth int
		for index := 0; index < 1000; index++ {
			value := 100.0
			if index/100%2 != 0 {
				value = 110
			}
			item := statistic.Weighted{Value: value, Weight: float64(index%3 + 1)}
			var reading, other adaptive.WindowReading
			for output := range window.Next(sequence.NewOne(unsafe.Pointer(&item)).Next(nil)) {
				reading = *(*adaptive.WindowReading)(output)
			}
			item.Weight *= 1000 // Changing the volume unit cannot change the regime boundary.
			for output := range scaled.Next(sequence.NewOne(unsafe.Pointer(&item)).Next(nil)) {
				other = *(*adaptive.WindowReading)(output)
			}
			So(reading.ShedRatio, ShouldAlmostEqual, other.ShedRatio)
			if index < 100 {
				So(reading.ShedRatio, ShouldEqual, 1)
			}
			if reading.ShedRatio < 1 {
				changes++
			}
			if reading.ShedRatio == 1 {
				growth++
			}
		}
		So(changes, ShouldBeGreaterThan, 1)
		So(growth, ShouldBeGreaterThan, changes)
	})
}

func BenchmarkWeightedWindowNext(b *testing.B) {
	window := adaptive.NewWeightedWindow()
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		item := statistic.Weighted{Value: float64(index / 100 % 2), Weight: float64(index%3 + 1)}
		for range window.Next(sequence.NewOne(unsafe.Pointer(&item)).Next(nil)) {
		}
	}
}
