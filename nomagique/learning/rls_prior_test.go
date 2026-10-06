package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRlsPriorNext(t *testing.T) {
	Convey("Given an affine design and a configured coefficient variance", t, func() {
		node := algo.NewSquareRootRLS(9)
		output := data.Read[[][]float64](node.Next(data.NewValue([2][]float64{
			{1, 2, -3},
			{1},
		})))

		So(node.Error(), ShouldBeNil)
		So(output[1], ShouldResemble, []float64{0, 0, 0})
		So(output[3:], ShouldResemble, [][]float64{{3, 0, 0}, {0, 3, 0}, {0, 0, 3}})

		Convey("An invalid prior variance cannot create a model", func() {
			rejected := algo.NewSquareRootRLS(-1)

			for range rejected.Next(data.NewValue([2][]float64{
				{1, 2, -3},
				{1},
			})) {
			}

			err := rejected.Error()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, core.ErrDomain.Error())
		})
	})
}
