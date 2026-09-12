package causal_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBackdoorNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := causal.NewBackdoor(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			gotEval := transport.NewEvaluate(node)
			var got causal.BackdoorReading

			for out := range gotEval.Next(transport.NewValues(causal.Query{
				Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
			}).Next(nil)) {
				got = *(*causal.BackdoorReading)(out)
			}

			err := gotEval.Error()
			So(err, ShouldBeNil)
			So(got.Defined, ShouldBeTrue)
			So(got.Expectation, ShouldAlmostEqual, 2+3*level)
		}
	})
}

func TestCounterfactualNext(t *testing.T) {
	Convey("Counterfactuals retain factual noise on the affine model", t, func() {
		node := causal.NewCounterfactual(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				gotEval := transport.NewEvaluate(node)
				var got causal.CounterfactualReading

				for out := range gotEval.Next(transport.NewValues(causal.Query{
					Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
					Actual: []float64{0, 0, 1 + noise},
				}).Next(nil)) {
					got = *(*causal.CounterfactualReading)(out)
				}

				err := gotEval.Error()
				So(err, ShouldBeNil)
				So(got.Defined, ShouldBeTrue)
				So(got.Noise, ShouldAlmostEqual, noise)
				So(got.Counterfactual, ShouldAlmostEqual, 1+3*level+noise)
			}
		}
	})
}

func TestCounterfactualRankDeficiency(t *testing.T) {
	Convey("A singular fit does not invent a counterfactual", t, func() {
		node := causal.NewCounterfactual(1e-15)
		gotEval := transport.NewEvaluate(node)
		var got causal.CounterfactualReading

		for out := range gotEval.Next(transport.NewValues(causal.Query{
			Rows:     [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}},
			Features: []int{0, 1}, Target: 2, Treatment: 1, Level: 1,
			Actual: []float64{1, 0, 2},
		}).Next(nil)) {
			got = *(*causal.CounterfactualReading)(out)
		}

		err := gotEval.Error()
		So(err, ShouldBeNil)
		So(got.Defined, ShouldBeFalse)
		So(math.IsNaN(got.Counterfactual), ShouldBeTrue)
	})
}

func TestTableNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := causal.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			gotEval := transport.NewEvaluate(node)
			var got causal.TableOutput

			for out := range gotEval.Next(transport.NewValues(causal.TableInput{
				Rows: rows, Target: 2, Treatment: 1, Level: level,
				Controls: []int{0}, Linear: true,
			}).Next(nil)) {
				got = *(*causal.TableOutput)(out)
			}

			err := gotEval.Error()
			So(err, ShouldBeNil)
			So(got.Expectation, ShouldAlmostEqual, 2+3*level)
		}
	})

	Convey("The stump strategy standardizes over the same evidence", t, func() {
		node := causal.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		gotEval := transport.NewEvaluate(node)
		var got causal.TableOutput

		for out := range gotEval.Next(transport.NewValues(causal.TableInput{
			Rows: rows, Target: 2, Treatment: 1, Level: 1,
			Controls: []int{0}, Linear: false,
		}).Next(nil)) {
			got = *(*causal.TableOutput)(out)
		}

		err := gotEval.Error()
		So(err, ShouldBeNil)
		So(got.Expectation, ShouldAlmostEqual, 2+3*1)
	})

	Convey("Evidence short of the minimum is refused", t, func() {
		node := causal.NewTable(4)
		refusedEval := transport.NewEvaluate(node)

		for range refusedEval.Next(transport.NewValues(causal.TableInput{
			Rows: [][]float64{{0, 0, 1}, {1, 0, 2}}, Target: 2, Treatment: 1,
			Controls: []int{0}, Linear: true,
		}).Next(nil)) {
		}

		err := refusedEval.Error()
		So(err, ShouldNotBeNil)
	})
}

func TestTableAbductive(t *testing.T) {
	Convey("Counterfactuals retain factual noise on the affine model", t, func() {
		node := causal.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				gotEval := transport.NewEvaluate(node)
				var got causal.TableOutput

				for out := range gotEval.Next(transport.NewValues(causal.TableInput{
					Rows: rows, Target: 2, Treatment: 1, Level: level, Linear: true,
					Features: []int{0, 1}, Actual: []float64{0, 0, 1 + noise},
				}).Next(nil)) {
					got = *(*causal.TableOutput)(out)
				}

				err := gotEval.Error()
				So(err, ShouldBeNil)
				So(got.Noise, ShouldAlmostEqual, noise)
				So(got.Counterfactual, ShouldAlmostEqual, 1+3*level+noise)
				So(got.Precision, ShouldAlmostEqual, 1/(1+math.Abs(noise)))
			}
		}
	})

	Convey("A singular fit surfaces the non-identifiable state", t, func() {
		node := causal.NewTable(4)
		_Eval := transport.NewEvaluate(node)

		for range _Eval.Next(transport.NewValues(causal.TableInput{
			Rows:     [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}},
			Features: []int{0, 1}, Target: 2, Treatment: 1, Level: 1,
			Actual: []float64{1, 0, 2}, Linear: true,
		}).Next(nil)) {
		}

		err := _Eval.Error()
		So(err, ShouldNotBeNil)
	})
}
