package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestPairsNext(t *testing.T) {
	Convey("Given Pairs as a Primitive", t, func() {
		Convey("When a run is paired", func() {
			pairs := store.NewPairs[float64](
				store.NewSpread[float64](core.From([]float64{1, 2, 3, 4})),
			)

			caller := core.From(0.0)

			So(core.To[[2]float64](pairs.Next(caller)), ShouldResemble, [2]float64{1, 2})
			So(core.To[[2]float64](pairs.Next(caller)), ShouldResemble, [2]float64{2, 3})
			So(core.To[[2]float64](pairs.Next(caller)), ShouldResemble, [2]float64{3, 4})
			So(pairs.Next(caller), ShouldBeNil)
		})

		Convey("When a second caller pairs the same run", func() {
			pairs := store.NewPairs[float64](
				store.NewSpread[float64](core.From([]float64{1, 2, 3})),
			)

			first, second := core.From(0.0), core.From(1.0)

			pairs.Next(first)

			So(core.To[[2]float64](pairs.Next(second)), ShouldResemble, [2]float64{1, 2})
		})

		Convey("When the run holds one value", func() {
			pairs := store.NewPairs[float64](
				store.NewSpread[float64](core.From([]float64{1})),
			)

			So(pairs.Next(core.From(0.0)), ShouldBeNil)
		})

		Convey("When the run holds nothing", func() {
			pairs := store.NewPairs[float64](
				store.NewSpread[float64](core.From([]float64(nil))),
			)

			So(pairs.Next(core.From(0.0)), ShouldBeNil)
			So(pairs.Read(), ShouldBeNil)
		})
	})
}
