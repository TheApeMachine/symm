package algo_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSquareRootRLSNext(t *testing.T) {
	Convey("A labeled observation trains only after the prior forecast", t, func() {
		node := algo.NewSquareRootRLS(1)
		first, err := transport.Evaluate(node, transport.Values(algo.Query{
			Design:   []float64{1, 2},
			Target:   3,
			Observed: true,
			Lambda:   1,
		}))
		So(err, ShouldBeNil)
		So(first.Observed, ShouldBeTrue)
		So(first.Prediction, ShouldEqual, 0)
		So(first.Innovation, ShouldEqual, 3)

		prior := make([]float64, len(first.Beta))
		copy(prior, first.Beta)

		query, err := transport.Evaluate(node, transport.Values(algo.Query{
			Design: []float64{1, 2},
			Lambda: 1,
		}))
		So(err, ShouldBeNil)
		So(query.Observed, ShouldBeFalse)
		So(query.Beta, ShouldResemble, prior)
		So(query.Prediction, ShouldNotEqual, 0)

		Convey("A forgetting factor outside (0,1] is refused without training", func() {
			_, err := transport.Evaluate(algo.NewSquareRootRLS(1), transport.Values(algo.Query{
				Design:   []float64{1},
				Observed: true,
				Lambda:   0,
			}))
			So(errors.Is(err, core.ErrDomain), ShouldBeTrue)
		})
	})
}
