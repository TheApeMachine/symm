package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewAccumulate(t *testing.T) {
	Convey("Given a nested pipeline", t, func() {
		pipeline := NewDerived(WithDynamics(NewScaleIndex(0)))
		accumulate := NewAccumulate(pipeline, nil)

		Convey("It should multiply running output by pipeline factor", func() {
			first, err := accumulate.Next(0, 2, 3)

			So(err, ShouldBeNil)
			So(first, ShouldEqual, 2)

			second, err := accumulate.Next(first, 4, 5)

			So(err, ShouldBeNil)
			So(second, ShouldEqual, 8)
		})

		Convey("It should reset the nested pipeline", func() {
			_, _ = accumulate.Next(0, 2)
			So(accumulate.Reset(), ShouldBeNil)

			fresh, err := accumulate.Next(0, 3)

			So(err, ShouldBeNil)
			So(fresh, ShouldEqual, 3)
		})
	})
}

func BenchmarkAccumulateNext(b *testing.B) {
	pipeline := NewDerived(WithDynamics(NewScaleIndex(0)))
	accumulate := NewAccumulate(pipeline, nil)

	for b.Loop() {
		_, _ = accumulate.Next(1, 1.01)
	}
}
