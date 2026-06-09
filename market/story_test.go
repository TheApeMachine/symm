package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestMeasurementWindowPush(t *testing.T) {
	Convey("Given a partially filled window", t, func() {
		window := NewMeasurementWindow(4)

		var measurements []logic.Measurement

		for index := range 2 {
			measurements = window.Push(logic.Measurement{
				Symbol:   "ETH/EUR",
				Strength: float64(index + 1),
			})
		}

		measurements = window.Push(logic.Measurement{
			Symbol:   "ETH/EUR",
			Strength: 3,
		})

		Convey("It should flush without panicking on nil ring slots", func() {
			So(len(measurements), ShouldEqual, 3)
			So(measurements[0].Strength, ShouldEqual, 1)
			So(measurements[2].Strength, ShouldEqual, 3)
		})
	})
}

func BenchmarkMeasurementWindowPush(b *testing.B) {
	window := NewMeasurementWindow(1024)
	measurement := logic.Measurement{Symbol: "ETH/EUR", Strength: 1.0}

	b.ReportAllocs()

	for b.Loop() {
		window.Push(measurement)
	}
}
