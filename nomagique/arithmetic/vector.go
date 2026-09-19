package arithmetic

import "github.com/theapemachine/symm/nomagique/types"

type SubVec types.Value[[2][]float64, []float64]
/*
NewSubVec subtracts vector B from vector A.
Input is [2][]float64{A, B}. Returns A - B.
*/
func NewSubVec() SubVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		out := make([]float64, len(a))
		for i := range a {
			out[i] = a[i] - b[i]
		}
		return out
	}
}

type AddVec types.Value[[2][]float64, []float64]
/*
NewAddVec adds vector B to vector A.
Input is [2][]float64{A, B}. Returns A + B.
*/
func NewAddVec() AddVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		out := make([]float64, len(a))
		for i := range a {
			out[i] = a[i] + b[i]
		}
		return out
	}
}

type AddScaledVec types.Value[[3][]float64, []float64]
/*
NewAddScaledVec adds scaled vector B to vector A.
Input is [3][]float64{A, B, {scale}}. Returns A + scale * B.
*/
func NewAddScaledVec() AddScaledVec {
	return func(in [3][]float64) []float64 {
		a, b, s := in[0], in[1], in[2]
		scale := s[0]
		out := make([]float64, len(a))
		for i := range a {
			out[i] = a[i] + scale*b[i]
		}
		return out
	}
}

type MulVec types.Value[[2][]float64, []float64]
/*
NewMulVec element-wise multiplies vector A and vector B.
Input is [2][]float64{A, B}. Returns A * B.
*/
func NewMulVec() MulVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		out := make([]float64, len(a))
		for i := range a {
			out[i] = a[i] * b[i]
		}
		return out
	}
}

type ScaleVec types.Value[[2][]float64, []float64]
/*
NewScaleVec scales vector A by a scalar.
Input is [2][]float64{A, {scale}}. Returns A * scale.
*/
func NewScaleVec() ScaleVec {
	return func(in [2][]float64) []float64 {
		a, s := in[0], in[1]
		scale := s[0]
		out := make([]float64, len(a))
		for i := range a {
			out[i] = a[i] * scale
		}
		return out
	}
}

type OuterProduct types.Value[[2][]float64, [][]float64]
/*
NewOuterProduct computes the outer product of two vectors A and B.
Input is [2][]float64{A, B}. Returns a 2D matrix [][]float64.
*/
func NewOuterProduct() OuterProduct {
	return func(in [2][]float64) [][]float64 {
		a, b := in[0], in[1]
		out := make([][]float64, len(a))
		for i := range a {
			out[i] = make([]float64, len(b))
			for j := range b {
				out[i][j] = a[i] * b[j]
			}
		}
		return out
	}
}

type MatVecMul types.Value[[2]any, []float64]
/*
NewMatVecMul multiplies Matrix A by Vector B.
Input is [2]any{A, B} where A is [][]float64 and B is []float64.
We use any because Go lacks generic tuples of mixed types.
*/
func NewMatVecMul() MatVecMul {
	return func(in [2]any) []float64 {
		mat := in[0].([][]float64)
		vec := in[1].([]float64)
		
		rows := len(mat)
		out := make([]float64, rows)
		for i := range rows {
			sum := 0.0
			for j := 0; j < len(vec); j++ {
				sum += mat[i][j] * vec[j]
			}
			out[i] = sum
		}
		return out
	}
}

type VecPrecisionWeight types.Value[[2][]float64, []float64]
/*
NewVecPrecisionWeight element-wise multiplies an error vector by a precision vector.
Input is [2][]float64{error, precision}.
*/
func NewVecPrecisionWeight() VecPrecisionWeight {
	return func(in [2][]float64) []float64 {
		err, prec := in[0], in[1]
		out := make([]float64, len(err))
		for i := range err {
			out[i] = err[i] * prec[i]
		}
		return out
	}
}
