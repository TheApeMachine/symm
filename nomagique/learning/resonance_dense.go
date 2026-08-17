package learning

import (
	"math"

	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/blas64"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

func denseColDot(left, right *mat.VecDense) float64 {
	return floats.Dot(left.RawVector().Data, right.RawVector().Data)
}

func denseColNorm(vector *mat.VecDense) float64 {
	return floats.Norm(vector.RawVector().Data, 2)
}

func denseApplyTanhInPlace(vector *mat.VecDense) {
	data := vector.RawVector().Data

	for index, value := range data {
		data[index] = math.Tanh(value)
	}
}

func denseApplyOneMinusSquareInto(dst, src *mat.VecDense) {
	dstData := dst.RawVector().Data
	srcData := src.RawVector().Data

	for index, value := range srcData {
		dstData[index] = 1.0 - value*value
	}
}

func denseFill(vector *mat.VecDense, value float64) {
	data := vector.RawVector().Data

	for index := range data {
		data[index] = value
	}
}

func denseScaleInPlace(matrix *mat.Dense, scale float64) {
	floats.Scale(scale, matrix.RawMatrix().Data)
}

func denseVarianceEMAInto(
	variance *mat.VecDense,
	residual *mat.VecDense,
	beta float64,
	floor float64,
) {
	varianceData := variance.RawVector().Data
	residualData := residual.RawVector().Data
	retainedWeight := 1.0 - beta

	for index, residualValue := range residualData {
		varianceValue := retainedWeight*varianceData[index] +
			beta*(residualValue*residualValue)

		varianceData[index] = math.Max(floor, varianceValue)
	}
}

func densePrecisionFromVarianceInto(
	precision *mat.VecDense,
	variance *mat.VecDense,
	minimum float64,
	maximum float64,
) {
	precisionData := precision.RawVector().Data
	varianceData := variance.RawVector().Data

	for index, varianceValue := range varianceData {
		precisionData[index] = math.Min(
			maximum,
			math.Max(minimum, 1.0/varianceValue),
		)
	}
}

func denseClipColInPlace(vector *mat.VecDense, clip float64) {
	data := vector.RawVector().Data

	for index, value := range data {
		switch {
		case value > clip:
			data[index] = clip
		case value < -clip:
			data[index] = -clip
		}
	}
}

func denseOuterColsInto(
	dst *mat.Dense,
	left *mat.VecDense,
	right *mat.VecDense,
	scale float64,
) {
	dst.Outer(scale, left, right)
}

func denseMulWeightTransposeInto(
	dst *mat.VecDense,
	weight *mat.Dense,
	signal *mat.VecDense,
) {
	blas64.Gemv(
		blas.Trans,
		1,
		weight.RawMatrix(),
		signal.RawVector(),
		0,
		dst.RawVector(),
	)
}
