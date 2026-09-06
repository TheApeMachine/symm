package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestEachNext(t *testing.T) {
	Convey("Given an Each as a Primitive", t, func() {
		Convey("When every value of a run is squared and summed", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewEach(
					store.NewSpread[float64](core.From([]float64{1, 2, 3})),
					calculus.NewSquare(core.From(0.0)),
				),
			)), ShouldEqual, 14)
		})

		Convey("When every value of a run is replaced by one", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewEach(
					store.NewSpread[float64](core.From([]float64{4, 7, 9})),
					core.From(1.0),
				),
			)), ShouldEqual, 3)
		})

		Convey("When a run is counted and summed for its mean", func() {
			values := []float64{4, 7, 9, 8}

			total := arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewSpread[float64](core.From(values)),
			)

			count := arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewEach(
					store.NewSpread[float64](core.From(values)), core.From(1.0),
				),
			)

			So(core.To[float64](arithmetic.NewDivide(total).Next(count)),
				ShouldEqual, 7)
		})

		Convey("When the run holds nothing", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewEach(
					store.NewSpread[float64](core.From([]float64(nil))), core.From(1.0),
				),
			)), ShouldEqual, 0)
		})

		Convey("When a value of the run is poisoned", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewEach(
					store.NewSpread[float64](core.From([]float64{1, 2})),
					calculus.NewReciprocal(core.From(0.0)),
				),
			)), ShouldEqual, 1.5)
		})
	})
}
