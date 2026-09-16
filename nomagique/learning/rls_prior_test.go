package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

func TestRlsPriorNext(t *testing.T) {
	Convey("Given an affine design and a configured coefficient variance", t, func() {
		node := algo.NewSquareRootRLS(9)
		outputEval := node
		var output algo.Reading

		for out := range outputEval.Next(sequence.NewValues(algo.Query{
			Design: []float64{1, 2, -3},
			Lambda: 1,
		}).Next(nil)) {
			output = *(*algo.Reading)(out)
		}

		err := outputEval.Error()
		So(err, ShouldBeNil)
		So(output.Beta, ShouldResemble, []float64{0, 0, 0})
		So(output.Root, ShouldResemble, [][]float64{{3, 0, 0}, {0, 3, 0}, {0, 0, 3}})

		Convey("An invalid prior variance cannot create a model", func() {
			_Eval := algo.NewSquareRootRLS(-1)

			for range _Eval.Next(sequence.NewValues(algo.Query{
				Design: []float64{1, 2, -3},
				Lambda: 1,
			}).Next(nil)) {
			}

			err := _Eval.Error()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, core.ErrDomain.Error())
		})
	})
}
