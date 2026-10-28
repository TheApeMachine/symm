package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFitNonLinearTable(testingTB *testing.T) {
	Convey("Given alternating residuals with no stable split", testingTB, func() {
		rows := make([][]float64, 200)

		for index := range rows {
			target := -1.0

			if index%2 == 0 {
				target = 1.0
			}

			rows[index] = []float64{float64(index), target}
		}

		table, tableErr := newNodeTable(rows, 1, len(rows))

		So(tableErr, ShouldBeNil)

		model, ok := fitNonLinearTable(table, []int{0})

		So(ok, ShouldBeTrue)

		Convey("It should reject tiny total overfit gain", func() {
			So(model.stumps, ShouldBeEmpty)
		})
	})
}
