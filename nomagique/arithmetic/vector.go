package arithmetic

import "github.com/theapemachine/symm/nomagique/types"

type SubVec types.Value[[2][]float64, []float64]
/*
NewSubVec subtracts vector B from vector A.
Input is [2][]float64{A, B}. Returns A - B.
*/
func NewSubVec(vectors ...types.Value[any, []float64]) SubVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
		}
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
func NewAddVec(vectors ...types.Value[any, []float64]) AddVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
		}
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
func NewAddScaledVec(vectors ...types.Value[any, []float64]) AddScaledVec {
	return func(in [3][]float64) []float64 {
		a, b, s := in[0], in[1], in[2]
		if len(vectors) >= 3 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
			if vectors[2] != nil {
				s = vectors[2](in)
			}
		}
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
func NewMulVec(vectors ...types.Value[any, []float64]) MulVec {
	return func(in [2][]float64) []float64 {
		a, b := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
		}
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
func NewScaleVec(vectors ...types.Value[any, []float64]) ScaleVec {
	return func(in [2][]float64) []float64 {
		a, s := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				s = vectors[1](in)
			}
		}
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
func NewOuterProduct(vectors ...types.Value[any, []float64]) OuterProduct {
	return func(in [2][]float64) [][]float64 {
		a, b := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
		}
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
func NewMatVecMul(operands ...types.Value[any, any]) MatVecMul {
	return func(in [2]any) []float64 {
		matAny, vecAny := in[0], in[1]
		if len(operands) >= 2 {
			if operands[0] != nil {
				matAny = operands[0](in)
			}
			if operands[1] != nil {
				vecAny = operands[1](in)
			}
		}
		mat := matAny.([][]float64)
		vec := vecAny.([]float64)
		
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
func NewVecPrecisionWeight(vectors ...types.Value[any, []float64]) VecPrecisionWeight {
	return func(in [2][]float64) []float64 {
		err, prec := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				err = vectors[0](in)
			}
			if vectors[1] != nil {
				prec = vectors[1](in)
			}
		}
		out := make([]float64, len(err))
		for i := range err {
			out[i] = err[i] * prec[i]
		}
		return out
	}
}
