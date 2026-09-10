package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRlsPriorNext(t *testing.T) {
	Convey("Given an affine design and a configured coefficient variance", t, func() {
		node := algo.NewSquareRootRLS(9)
		output, err := transport.Evaluate(node, transport.Values(algo.Query{
			Design: []float64{1, 2, -3},
			Lambda: 1,
		}))
		So(err, ShouldBeNil)
		So(output.Beta, ShouldResemble, []float64{0, 0, 0})
		So(output.Root, ShouldResemble, [][]float64{{3, 0, 0}, {0, 3, 0}, {0, 0, 3}})

		Convey("An invalid prior variance cannot create a model", func() {
			_, err := transport.Evaluate(algo.NewSquareRootRLS(-1), transport.Values(algo.Query{
				Design: []float64{1, 2, -3},
				Lambda: 1,
			}))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, core.ErrDomain.Error())
		})
	})
}
