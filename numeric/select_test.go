package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSelectNext(t *testing.T) {
	Convey("Given a select stage", t, func() {
		selectStage := NewSelect(func(out float64, values []float64) []float64 {
			return values[:1]
		})

		Convey("It should return the selected observation", func() {
			value, err := selectStage.Next(0, 2, 3, 4)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 2)
		})

		Convey("It should multiply multiple positive selections", func() {
			multi := NewSelect(func(out float64, values []float64) []float64 {
				return values
			})

			value, err := multi.Next(0, 2, 3)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 6)
		})
	})
}

func BenchmarkSelectNext(b *testing.B) {
	selectStage := NewSelect(func(out float64, values []float64) []float64 {
		return values[:1]
	})

	for b.Loop() {
		_, _ = selectStage.Next(0, 1.01)
	}
}
