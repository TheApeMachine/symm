package linalg

import (
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
)

/*
MatrixToDense converts a Cap'n Proto Matrix reader into a Gonum *mat.Dense.
*/
func MatrixToDense(matrix Matrix) (*mat.Dense, error) {
	rows := int(matrix.Rows())
	cols := int(matrix.Cols())

	if rows <= 0 || cols <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[linalg.MatrixToDense] rows and columns must be strictly positive",
			nil,
		))
	}

	dataList, err := matrix.Data()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.MatrixToDense] failed to read matrix data",
			err,
		))
	}

	expectedLength := rows * cols

	if dataList.Len() != expectedLength {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[linalg.MatrixToDense] data length does not match rows * cols",
			nil,
		))
	}

	slice := make([]float64, expectedLength)

	for index := 0; index < expectedLength; index++ {
		slice[index] = dataList.At(index)
	}

	return mat.NewDense(rows, cols, slice), nil
}

/*
DenseToMatrix writes values from a Gonum mat.Matrix into a Cap'n Proto Matrix builder.
*/
func DenseToMatrix(dense mat.Matrix, dest Matrix) error {
	if dense == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[linalg.DenseToMatrix] source dense matrix is nil",
			nil,
		))
	}

	rows, cols := dense.Dims()
	dest.SetRows(int32(rows))
	dest.SetCols(int32(cols))

	totalElements := rows * cols
	dataList, err := dest.NewData(int32(totalElements))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.DenseToMatrix] failed to allocate matrix data",
			err,
		))
	}

	if raw, ok := dense.(mat.RawMatrixer); ok {
		rawMat := raw.RawMatrix()

		for rowIndex := 0; rowIndex < rows; rowIndex++ {
			offset := rowIndex * rawMat.Stride

			for colIndex := 0; colIndex < cols; colIndex++ {
				dataList.Set(rowIndex*cols+colIndex, rawMat.Data[offset+colIndex])
			}
		}

		return nil
	}

	for rowIndex := 0; rowIndex < rows; rowIndex++ {
		for colIndex := 0; colIndex < cols; colIndex++ {
			dataList.Set(rowIndex*cols+colIndex, dense.At(rowIndex, colIndex))
		}
	}

	return nil
}

/*
VectorToVecDense converts a Cap'n Proto Vector reader into a Gonum *mat.VecDense.
*/
func VectorToVecDense(vector Vector) (*mat.VecDense, error) {
	dataList, err := vector.Data()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.VectorToVecDense] failed to read vector data",
			err,
		))
	}

	length := dataList.Len()

	if length <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[linalg.VectorToVecDense] vector length must be strictly positive",
			nil,
		))
	}

	slice := make([]float64, length)

	for index := 0; index < length; index++ {
		slice[index] = dataList.At(index)
	}

	return mat.NewVecDense(length, slice), nil
}

/*
VecDenseToVector writes values from a Gonum mat.Vector into a Cap'n Proto Vector builder.
*/
func VecDenseToVector(vec mat.Vector, dest Vector) error {
	if vec == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[linalg.VecDenseToVector] source vector is nil",
			nil,
		))
	}

	length := vec.Len()
	dataList, err := dest.NewData(int32(length))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.VecDenseToVector] failed to allocate vector data",
			err,
		))
	}

	for index := 0; index < length; index++ {
		dataList.Set(index, vec.AtVec(index))
	}

	return nil
}
