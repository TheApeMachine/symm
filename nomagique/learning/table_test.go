package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	learning "github.com/theapemachine/symm/nomagique/learning"
)

func TestBackdoorNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := learning.NewBackdoor(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			gotEval := node
			var got learning.BackdoorReading

			for out := range gotEval.Next(sequence.NewValues(learning.Query{
				Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
			}).Next(nil)) {
				got = *(*learning.BackdoorReading)(out)
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
		node := learning.NewCounterfactual(1e-15)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				gotEval := node
				var got learning.CounterfactualReading

				for out := range gotEval.Next(sequence.NewValues(learning.Query{
					Rows: rows, Features: []int{0, 1}, Target: 2, Treatment: 1, Level: level,
					Actual: []float64{0, 0, 1 + noise},
				}).Next(nil)) {
					got = *(*learning.CounterfactualReading)(out)
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
		node := learning.NewCounterfactual(1e-15)
		gotEval := node
		var got learning.CounterfactualReading

		for out := range gotEval.Next(sequence.NewValues(learning.Query{
			Rows:     [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}},
			Features: []int{0, 1}, Target: 2, Treatment: 1, Level: 1,
			Actual: []float64{1, 0, 2},
		}).Next(nil)) {
			got = *(*learning.CounterfactualReading)(out)
		}

		err := gotEval.Error()
		So(err, ShouldBeNil)
		So(got.Defined, ShouldBeFalse)
		So(math.IsNaN(got.Counterfactual), ShouldBeTrue)
	})
}

func TestTableNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := learning.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			gotEval := node
			var got learning.TableOutput

			for out := range gotEval.Next(sequence.NewValues(learning.TableInput{
				Rows: rows, Target: 2, Treatment: 1, Level: level,
				Controls: []int{0}, Linear: true,
			}).Next(nil)) {
				got = *(*learning.TableOutput)(out)
			}

			err := gotEval.Error()
			So(err, ShouldBeNil)
			So(got.Expectation, ShouldAlmostEqual, 2+3*level)
		}
	})

	Convey("The stump strategy standardizes over the same evidence", t, func() {
		node := learning.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		gotEval := node
		var got learning.TableOutput

		for out := range gotEval.Next(sequence.NewValues(learning.TableInput{
			Rows: rows, Target: 2, Treatment: 1, Level: 1,
			Controls: []int{0}, Linear: false,
		}).Next(nil)) {
			got = *(*learning.TableOutput)(out)
		}

		err := gotEval.Error()
		So(err, ShouldBeNil)
		So(got.Expectation, ShouldAlmostEqual, 2+3*1)
	})

	Convey("Evidence short of the minimum is refused", t, func() {
		node := learning.NewTable(4)
		refusedEval := node

		for range refusedEval.Next(sequence.NewValues(learning.TableInput{
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
		node := learning.NewTable(4)
		rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}

		for _, level := range []float64{1, 0, 2, -1} {
			for _, noise := range []float64{0, 0.25, -0.5} {
				gotEval := node
				var got learning.TableOutput

				for out := range gotEval.Next(sequence.NewValues(learning.TableInput{
					Rows: rows, Target: 2, Treatment: 1, Level: level, Linear: true,
					Features: []int{0, 1}, Actual: []float64{0, 0, 1 + noise},
				}).Next(nil)) {
					got = *(*learning.TableOutput)(out)
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
		node := learning.NewTable(4)
		_Eval := node

		for range _Eval.Next(sequence.NewValues(learning.TableInput{
			Rows:     [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}},
			Features: []int{0, 1}, Target: 2, Treatment: 1, Level: 1,
			Actual: []float64{1, 0, 2}, Linear: true,
		}).Next(nil)) {
		}

		err := _Eval.Error()
		So(err, ShouldNotBeNil)
	})
}
