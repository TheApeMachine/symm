package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestCausalLearning(t *testing.T) {
	Convey("Given linear fit and prediction closures", t, func() {
		// Model: Y = 2*X + 1
		rows := [][]float64{
			{1, 3},
			{2, 5},
			{3, 7},
			{4, 9},
			{5, 11},
		}

		fit := NewLinearFit(1e-9, []int{0}, 1)
		coeffs := fit(rows)

		So(coeffs, ShouldNotBeNil)
		So(coeffs[0], ShouldAlmostEqual, core.Unit, 1e-6) // Intercept = 1
		So(coeffs[1], ShouldAlmostEqual, 2.0, 1e-6)       // Slope = 2

		predict := NewLinearPrediction([]int{0})
		pred := predict([2][]float64{coeffs, {10.0, 0.0}})
		So(pred, ShouldAlmostEqual, 21.0, 1e-6)
	})

	Convey("Given backdoor adjustment closure", t, func() {
		// Treatment X at col 0, Confounder Z at col 1, Outcome Y at col 2
		// Y = 2*X + 3*Z + 1
		rows := [][]float64{
			{1, 2, 9},   // 2(1) + 3(2) + 1 = 9
			{2, 1, 8},   // 2(2) + 3(1) + 1 = 8
			{3, 4, 19},  // 2(3) + 3(4) + 1 = 19
			{4, 3, 18},  // 2(4) + 3(3) + 1 = 18
			{5, 5, 26},  // 2(5) + 3(5) + 1 = 26
		}

		// Intervene: do(X = 10).
		// With X=10:
		// row 0: 2(10) + 3(2) + 1 = 27
		// row 1: 2(10) + 3(1) + 1 = 24
		// row 2: 2(10) + 3(4) + 1 = 33
		// row 3: 2(10) + 3(3) + 1 = 30
		// row 4: 2(10) + 3(5) + 1 = 36
		// Mean = (27 + 24 + 33 + 30 + 36) / 5 = 150 / 5 = 30.0
		backdoor := NewBackdoor(1e-9, []int{0, 1}, 2, 0, 10.0)
		expY := backdoor(rows)

		So(expY, ShouldAlmostEqual, 30.0, 1e-6)
	})

	Convey("Given counterfactual estimation closure", t, func() {
		// Model: Y = 2*X + 1 + u
		history := [][]float64{
			{1, 3},
			{2, 5},
			{3, 7},
			{4, 9},
			{5, 11},
		}

		// Factual observation: X = 4, but observed Y = 10 (factual noise u = 10 - (2*4+1) = 1.0)
		factualRow := []float64{4, 10}

		// Counterfactual query: what would Y have been if X had been 1?
		// Model counterfactual: 2(1) + 1 + u = 3 + 1.0 = 4.0
		cf := NewCounterfactual(1e-9, []int{0}, 1, 0, 1.0)
		result := cf([2][][]float64{history, {factualRow}})

		So(result[0], ShouldAlmostEqual, 4.0, 1e-6)
		So(result[1], ShouldAlmostEqual, 0.5, 1e-6) // 1 / (1 + |1.0|) = 0.5
	})
}
