package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
)

func TestDenseColDot(testingTB *testing.T) {
	Convey("Given two dense vectors", testingTB, func() {
		left := mat.NewVecDense(3, []float64{1, 2, 3})
		right := mat.NewVecDense(3, []float64{4, 5, 6})

		Convey("It should calculate their dot product", func() {
			So(denseColDot(left, right), ShouldEqual, 32)
		})
	})
}

func TestDenseColNorm(testingTB *testing.T) {
	Convey("Given a dense vector", testingTB, func() {
		vector := mat.NewVecDense(2, []float64{3, 4})

		Convey("It should calculate its Euclidean norm", func() {
			So(denseColNorm(vector), ShouldEqual, 5)
		})
	})
}

func TestDenseApplyTanhInPlace(testingTB *testing.T) {
	Convey("Given a dense vector", testingTB, func() {
		vector := mat.NewVecDense(3, []float64{-1, 0, 1})

		denseApplyTanhInPlace(vector)

		Convey("It should apply tanh to every element", func() {
			expected := mat.NewVecDense(3, []float64{
				math.Tanh(-1),
				0,
				math.Tanh(1),
			})
			So(mat.Equal(vector, expected), ShouldBeTrue)
		})
	})
}

func TestDenseApplyOneMinusSquareInto(testingTB *testing.T) {
	Convey("Given source and destination vectors", testingTB, func() {
		source := mat.NewVecDense(3, []float64{-0.5, 0, 0.5})
		destination := mat.NewVecDense(3, nil)

		denseApplyOneMinusSquareInto(destination, source)

		Convey("It should write one minus each squared value", func() {
			expected := mat.NewVecDense(3, []float64{0.75, 1, 0.75})
			So(mat.Equal(destination, expected), ShouldBeTrue)
		})
	})
}

func TestDenseFill(testingTB *testing.T) {
	Convey("Given a dense column", testingTB, func() {
		column := mat.NewVecDense(3, nil)

		denseFill(column, 2.5)

		Convey("It should fill every element", func() {
			expected := mat.NewVecDense(3, []float64{2.5, 2.5, 2.5})
			So(mat.Equal(column, expected), ShouldBeTrue)
		})
	})
}

func TestDenseScaleInPlace(testingTB *testing.T) {
	Convey("Given a dense matrix", testingTB, func() {
		matrix := mat.NewDense(2, 2, []float64{1, 2, 3, 4})

		denseScaleInPlace(matrix, 0.5)

		Convey("It should scale every element without changing shape", func() {
			expected := mat.NewDense(2, 2, []float64{0.5, 1, 1.5, 2})
			So(mat.Equal(matrix, expected), ShouldBeTrue)
		})
	})
}

func TestDenseVarianceEMAInto(testingTB *testing.T) {
	Convey("Given retained variances and a residual column", testingTB, func() {
		variance := mat.NewVecDense(3, []float64{1, 2, 3})
		residual := mat.NewVecDense(3, []float64{2, 0, -4})

		denseVarianceEMAInto(variance, residual, 0.25, 1)

		Convey("It should update the whole column and apply the floor", func() {
			expected := mat.NewVecDense(3, []float64{1.75, 1.5, 6.25})
			So(mat.EqualApprox(variance, expected, 1e-15), ShouldBeTrue)
		})
	})
}

func TestDensePrecisionFromVarianceInto(testingTB *testing.T) {
	Convey("Given a variance column spanning both precision bounds", testingTB, func() {
		variance := mat.NewVecDense(3, []float64{0.01, 2, 100})
		precision := mat.NewVecDense(3, nil)

		densePrecisionFromVarianceInto(precision, variance, 0.1, 5)

		Convey("It should invert and clamp the whole column", func() {
			expected := mat.NewVecDense(3, []float64{5, 0.5, 0.1})
			So(mat.EqualApprox(precision, expected, 1e-15), ShouldBeTrue)
		})
	})
}

func TestDenseClipColInPlace(testingTB *testing.T) {
	Convey("Given values on both sides of a symmetric bound", testingTB, func() {
		vector := mat.NewVecDense(5, []float64{-3, -1, 0, 1, 3})

		denseClipColInPlace(vector, 2)

		Convey("It should clip only the values outside the bound", func() {
			expected := mat.NewVecDense(5, []float64{-2, -1, 0, 1, 2})
			So(mat.Equal(vector, expected), ShouldBeTrue)
		})
	})
}

func TestDenseOuterColsInto(testingTB *testing.T) {
	Convey("Given two vectors and a destination matrix", testingTB, func() {
		left := mat.NewVecDense(2, []float64{1, 2})
		right := mat.NewVecDense(3, []float64{3, 4, 5})
		destination := mat.NewDense(2, 3, nil)

		denseOuterColsInto(destination, left, right, 0.5)

		Convey("It should write their scaled outer product", func() {
			expected := mat.NewDense(2, 3, []float64{1.5, 2, 2.5, 3, 4, 5})
			So(mat.Equal(destination, expected), ShouldBeTrue)
		})
	})
}

func TestDenseMulWeightTransposeInto(testingTB *testing.T) {
	Convey("Given a weight matrix and compatible signal", testingTB, func() {
		weight := mat.NewDense(2, 3, []float64{1, 2, 3, 4, 5, 6})
		signal := mat.NewVecDense(2, []float64{2, -1})
		destination := mat.NewVecDense(3, nil)

		denseMulWeightTransposeInto(destination, weight, signal)

		Convey("It should multiply by the transposed weight", func() {
			expected := mat.NewVecDense(3, []float64{-2, -1, 0})
			So(mat.Equal(destination, expected), ShouldBeTrue)
		})
	})
}
