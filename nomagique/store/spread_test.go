package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSpreadNext(t *testing.T) {
	Convey("Given a Spread as a Primitive", t, func() {
		Convey("When an accumulator folds what it hands over", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewSpread(core.From([]float64{1, 2, 3, 4})),
			)), ShouldEqual, 10)
		})

		Convey("When two accumulators fold the same Spread", func() {
			spread := store.NewSpread(core.From([]float64{1, 2, 3}))

			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(spread)),
				ShouldEqual, 6)
			So(core.To[float64](arithmetic.NewMultiply(core.From(1.0)).Next(spread)),
				ShouldEqual, 6)
		})

		Convey("When the same accumulator folds it twice", func() {
			spread := store.NewSpread(core.From([]float64{1, 2, 3}))
			add := arithmetic.NewAdd(core.From(0.0))

			add.Next(spread)

			So(core.To[float64](add.Next(spread)), ShouldEqual, 12)
		})

		Convey("When its source hands over several runs", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewSpread(tests.NewRun(
					core.From([]float64{1, 2}), core.From([]float64{3}),
				)),
			)), ShouldEqual, 6)
		})

		Convey("When its source holds nothing", func() {
			So(core.To[float64](arithmetic.NewAdd(core.From(0.0)).Next(
				store.NewSpread(core.From([]float64(nil))),
			)), ShouldEqual, 0)
		})
	})
}
