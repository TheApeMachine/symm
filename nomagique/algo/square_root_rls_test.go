package algo_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSquareRootRLSNext(t *testing.T) {
	Convey("A labeled observation trains only after the prior forecast", t, func() {
		node := algo.NewSquareRootRLS(1)
		outFirst := tests.CollectSeq[algo.Reading](node.Next(transport.NewValues(algo.Query{
			Design:   []float64{1, 2},
			Target:   3,
			Observed: true,
			Lambda:   1,
		}).Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(outFirst), ShouldEqual, 1)
		first := outFirst[0]
		So(first.Observed, ShouldBeTrue)
		So(first.Prediction, ShouldEqual, 0)
		So(first.Innovation, ShouldEqual, 3)

		prior := make([]float64, len(first.Beta))
		copy(prior, first.Beta)

		outQuery := tests.CollectSeq[algo.Reading](node.Next(transport.NewValues(algo.Query{
			Design: []float64{1, 2},
			Lambda: 1,
		}).Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(outQuery), ShouldEqual, 1)
		query := outQuery[0]
		So(query.Observed, ShouldBeFalse)
		So(query.Beta, ShouldResemble, prior)
		So(query.Prediction, ShouldNotEqual, 0)

		Convey("A forgetting factor outside (0,1] is refused without training", func() {
			errNode := algo.NewSquareRootRLS(1)
			_ = tests.CollectSeq[algo.Reading](errNode.Next(transport.NewValues(algo.Query{
				Design:   []float64{1},
				Observed: true,
				Lambda:   0,
			}).Next(nil)))
			So(errors.Is(errNode.Error(), core.ErrDomain), ShouldBeTrue)
		})
	})
}
