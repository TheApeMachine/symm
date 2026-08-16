package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTable(t *testing.T) {
	Convey("Given observational rows with a treatment effect and a control", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		table, err := NewTable(rows, 2, len(rows), true)
		So(err, ShouldBeNil)

		Convey("Intervening on treatment should standardize over the controls", func() {
			expectation, err := table.DoExpectation(1, 1, 0)
			So(err, ShouldBeNil)
			So(expectation, ShouldAlmostEqual, 5, 1e-9)
		})

		Convey("Counterfactual prediction should retain factual residual noise", func() {
			counterfactual, noise, precision, err := table.AbductiveCounterfactual(
				[]int{0, 1}, rows[0], 1, 1,
			)
			So(err, ShouldBeNil)
			So(counterfactual, ShouldAlmostEqual, 4, 1e-9)
			So(noise, ShouldAlmostEqual, 0, 1e-9)
			So(precision, ShouldAlmostEqual, 1, 1e-9)
		})
	})
}
