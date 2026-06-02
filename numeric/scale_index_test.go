package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScaleIndexNext(t *testing.T) {
	Convey("Given a scale index stage", t, func() {
		scale := NewScaleIndex(1)

		Convey("It should multiply out by the selected observation", func() {
			value, err := scale.Next(2, 1, 3)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 6)
		})

		Convey("It should return zero when index is out of range", func() {
			value, err := scale.Next(2, 1)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 0)
		})
	})
}

func BenchmarkScaleIndexNext(b *testing.B) {
	scale := NewScaleIndex(0)

	for b.Loop() {
		_, _ = scale.Next(1, 1.01)
	}
}
