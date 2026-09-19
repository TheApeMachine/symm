package nomagique_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNumber(t *testing.T) {
	Convey("Given a chain of value closures", t, func() {
		addOne := func(in int) int {
			return in + 1
		}
		double := func(in int) int {
			return in * 2
		}

		pipeline := nomagique.NewNumber(
			types.Value[int, int](addOne),
			types.Value[int, int](double),
		)

		Convey("When processing an input value", func() {
			result := pipeline(3)

			Convey("Then it transforms through each stage sequentially", func() {
				So(result, ShouldEqual, 8) // (3 + 1) * 2 = 8
			})
		})
	})
}
