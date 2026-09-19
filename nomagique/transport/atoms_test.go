package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestTransportAtoms(t *testing.T) {
	Convey("Given transport atoms", t, func() {
		Convey("IO composition", func() {
			step1 := types.Value[int, int](func(x int) int { return x + 2 })
			step2 := types.Value[int, int](func(x int) int { return x * 3 })
			
			// IO just wraps a single types.Value primitive
			pipeline := transport.NewIO[int, int](func(in int) int {
				return step2(step1(in))
			})

			So(pipeline(4), ShouldEqual, 18) // (4 + 2) * 3 = 18
		})

		Convey("Tee offramp", func() {
			var tapped int
			offramp := types.Value[int, int](func(x int) int {
				tapped = x
				return x
			})
			tee := transport.NewTee(offramp)

			res := tee(42)
			So(res, ShouldEqual, 42)
			So(tapped, ShouldEqual, 42)
		})

		Convey("Fan broadcast", func() {
			b1 := types.Value[int, int](func(x int) int { return x + 1 })
			b2 := types.Value[int, int](func(x int) int { return x * 2 })
			fan := transport.NewFan(b1, b2)

			res := fan(5)
			So(res, ShouldResemble, []int{6, 10})
		})

		Convey("Parallel mapping", func() {
			worker := types.Value[int, int](func(x int) int { return x * 2 })
			parallel := transport.NewParallel(worker)

			res := parallel([]int{1, 2, 3})
			So(res, ShouldResemble, []int{2, 4, 6})
		})

		Convey("Discard", func() {
			discard := transport.NewDiscard[int]()
			So(discard(100), ShouldResemble, struct{}{})
		})
	})
}
