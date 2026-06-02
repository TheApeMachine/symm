package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProjectNext(t *testing.T) {
	Convey("Given a vector projection", t, func() {
		project := NewProject(func(out float64, values []float64) []float64 {
			return []float64{values[0], values[1]}
		})

		Convey("It should multiply projected values", func() {
			value, err := project.Next(0, 2, 3)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 6)
		})
	})

	Convey("Given a scalar projection", t, func() {
		project := NewProjectScalar(func(out float64, values []float64) float64 {
			return values[0] + values[1]
		})

		Convey("It should return zero when only scalar is set", func() {
			value, err := project.Next(0, 2, 3)

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 0)
		})
	})
}

func BenchmarkProjectNext(b *testing.B) {
	project := NewProject(func(out float64, values []float64) []float64 {
		return values[:1]
	})

	for b.Loop() {
		_, _ = project.Next(0, 1.01)
	}
}
