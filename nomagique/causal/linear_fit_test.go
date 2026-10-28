package causal

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOlsFit(testingTB *testing.T) {
	Convey("Given predictors that contain the same information", testingTB, func() {
		target := []float64{1, 4, 7, 10, 13, 16}
		predictor := []float64{0, 1, 2, 3, 4, 5}
		duplicate := append([]float64(nil), predictor...)

		coefficients, err := olsFit(target, predictor, duplicate)

		Convey("It should retain the shared control span without unstable failure", func() {
			So(err, ShouldBeNil)
			So(coefficients, ShouldHaveLength, 3)

			for index := range target {
				prediction := coefficients[0] +
					coefficients[1]*predictor[index] +
					coefficients[2]*duplicate[index]
				So(prediction, ShouldAlmostEqual, target[index], 1e-12)
			}
		})
	})

	Convey("Given fewer observations than fitted coefficients", testingTB, func() {
		_, err := olsFit(
			[]float64{1, 2},
			[]float64{0, 1},
			[]float64{1, 0},
		)

		Convey("It should remain unresolved without manufacturing a fit", func() {
			So(err, ShouldEqual, io.EOF)
		})
	})

	Convey("Given a target with no observed variation", testingTB, func() {
		_, err := olsFit([]float64{1, 1, 1}, []float64{0, 1, 2})

		Convey("It should remain unresolved without manufacturing coefficients", func() {
			So(err, ShouldEqual, io.EOF)
		})
	})
}

func TestBackdoorDenominator(testingTB *testing.T) {
	Convey("Given a treatment residual with no independent variation", testingTB, func() {
		_, err := backdoorDenominator(0)

		Convey("It should report an unresolved estimate", func() {
			So(err, ShouldEqual, io.EOF)
		})
	})
}

func TestNodeTable_treatmentIdentifiable(testingTB *testing.T) {
	Convey("Given duplicate and constant controls with independent treatment", testingTB, func() {
		rows := [][]float64{
			{0, 0, 1, 1, 2},
			{1, 1, 1, 0, 1},
			{2, 2, 1, 1, 4},
			{3, 3, 1, 0, 3},
			{4, 4, 1, 1, 6},
			{5, 5, 1, 0, 5},
		}
		table, err := newNodeTable(rows, 4)

		So(err, ShouldBeNil)

		Convey("It should identify the treatment beyond the control span", func() {
			So(table.treatmentIdentifiable(3, 0, 1, 2), ShouldBeNil)
		})

		Convey("It should reject a treatment contained in the control span", func() {
			So(table.treatmentIdentifiable(1, 0, 2), ShouldEqual, io.EOF)
		})
	})
}
