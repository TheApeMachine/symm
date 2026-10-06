package algo_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSquareRootRLSNext(t *testing.T) {
	Convey("A labeled observation trains only after the prior forecast", t, func() {
		node := algo.NewSquareRootRLS(1)
		outFirst := tests.CollectSeq[[][]float64](node.Next(data.NewValue([2][]float64{
			{1, 2},
			{1, 3},
		})))
		So(node.Error(), ShouldBeNil)
		So(len(outFirst), ShouldEqual, 1)
		first := outFirst[0]
		So(first[0][6], ShouldEqual, 1)
		So(first[0][0], ShouldEqual, 0)
		So(first[0][5], ShouldEqual, 3)

		prior := append([]float64(nil), first[1]...)

		outQuery := tests.CollectSeq[[][]float64](node.Next(data.NewValue([2][]float64{
			{1, 2},
			{1},
		})))
		So(node.Error(), ShouldBeNil)
		So(len(outQuery), ShouldEqual, 1)
		query := outQuery[0]
		So(query[0][6], ShouldEqual, 0)
		So(query[1], ShouldResemble, prior)
		So(query[0][0], ShouldNotEqual, 0)
		So(query[0][0], ShouldAlmostEqual, prior[0]+2*prior[1])
		So(query[0][4], ShouldEqual, 1)

		Convey("A forgetting factor outside (0,1] is refused without training", func() {
			errNode := algo.NewSquareRootRLS(1)
			_ = tests.CollectSeq[[][]float64](errNode.Next(data.NewValue([2][]float64{
				{1},
				{0, 0},
			})))
			So(errors.Is(errNode.Error(), core.ErrDomain), ShouldBeTrue)
		})
	})
}

func TestRLSPredictionAndUpdate(t *testing.T) {
	Convey("The prediction and update compose into the square-root learner", t, func() {
		prediction := algo.NewRLSPrediction()
		update := algo.NewRLSUpdate()
		posterior := [][]float64{{0, 0}, {0, 0}, {2, 0}, {0, 2}}
		design := []float64{1, 2}

		forecast := tests.CollectSeq[[]float64](prediction.Next(data.NewValue(
			append([][]float64{design, {1}}, posterior...),
		)))
		So(prediction.Error(), ShouldBeNil)
		So(forecast[0], ShouldResemble, []float64{0, 0, 0, 0, 0, 2, 4})

		updated := tests.CollectSeq[[][]float64](update.Next(data.NewValue(
			append([][]float64{{1, 3, forecast[0][0]}, forecast[0][5:]}, posterior...),
		)))
		So(update.Error(), ShouldBeNil)

		learner := algo.NewSquareRootRLS(4)
		reading := tests.CollectSeq[[][]float64](learner.Next(data.NewValue([2][]float64{design, {1, 3}})))
		So(learner.Error(), ShouldBeNil)
		So(updated[0][2:], ShouldResemble, reading[0][1:])
		So(updated[0][0][1], ShouldEqual, reading[0][0][5])

		Convey("A ragged posterior is a shape error", func() {
			errNode := algo.NewRLSPrediction()
			_ = tests.CollectSeq[[]float64](errNode.Next(data.NewValue([][]float64{{1, 2}, {1}, {0, 0}, {0, 0}, {1}})))
			So(errors.Is(errNode.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
