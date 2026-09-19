package arithmetic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestArithmeticAtoms(t *testing.T) {
	Convey("Given arithmetic atoms", t, func() {
		Convey("Multiply", func() {
			So(arithmetic.NewMultiply()([2]float64{3.0, 4.0}), ShouldEqual, 12.0)
		})

		Convey("Divide", func() {
			So(arithmetic.NewDivide()([2]float64{12.0, 3.0}), ShouldEqual, 4.0)
		})

		Convey("Add", func() {
			So(arithmetic.NewAdd()([2]float64{5.0, 7.0}), ShouldEqual, 12.0)
		})

		Convey("Subtract", func() {
			So(arithmetic.NewSubtract()([2]float64{10.0, 4.0}), ShouldEqual, 6.0)
		})

		Convey("SquareRoot", func() {
			So(arithmetic.NewSquareRoot()(16.0), ShouldEqual, 4.0)
		})

		Convey("DotProduct", func() {
			a := []float64{core.Unit, 2.0, 3.0}
			b := []float64{4.0, 5.0, 6.0}
			So(arithmetic.NewDotProduct()([2][]float64{a, b}), ShouldEqual, 32.0)
		})

		Convey("Transpose", func() {
			m := [][]float64{
				{core.Unit, 2.0, 3.0},
				{4.0, 5.0, 6.0},
			}
			t := arithmetic.NewTranspose()(m)
			So(len(t), ShouldEqual, 3)
			So(len(t[0]), ShouldEqual, 2)
			So(t[0][0], ShouldEqual, core.Unit)
			So(t[2][1], ShouldEqual, 6.0)
		})

		Convey("Identity", func() {
			id := arithmetic.NewIdentity()(2)
			So(id[0][0], ShouldEqual, core.Unit)
			So(id[0][1], ShouldEqual, 0.0)
			So(id[1][0], ShouldEqual, 0.0)
			So(id[1][1], ShouldEqual, core.Unit)
		})

		Convey("Sum", func() {
			sum := arithmetic.NewSum()
			So(sum(10.0), ShouldEqual, 10.0)
			So(sum(5.0), ShouldEqual, 15.0)
			So(sum(-3.0), ShouldEqual, 12.0)
		})
	})
}
