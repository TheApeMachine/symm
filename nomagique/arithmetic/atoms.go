package arithmetic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type Multiply types.Value[[2]float64, float64]

/*
NewMultiply takes a pair of floats and returns their product.
*/
func NewMultiply(operands ...types.Float) Multiply {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return l * r
		}
		return in[0] * in[1]
	}
}

type Divide types.Value[[2]float64, float64]

/*
NewDivide takes a pair of floats and returns their quotient.
We do not use fallback defaults or Inf checks; we return pure mathematical results.
*/
func NewDivide(operands ...types.Float) Divide {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return l / r
		}
		return in[0] / in[1]
	}
}

type Add types.Value[[2]float64, float64]

/*
NewAdd takes a pair of floats and returns their sum.
*/
func NewAdd(operands ...types.Float) Add {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return l + r
		}
		return in[0] + in[1]
	}
}

type Subtract types.Value[[2]float64, float64]

/*
NewSubtract takes a pair of floats and returns their difference (in[0] - in[1]).
*/
func NewSubtract(operands ...types.Float) Subtract {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return l - r
		}
		return in[0] - in[1]
	}
}

type SquareRoot types.Value[float64, float64]

/*
NewSquareRoot calculates the principal square root.
*/
func NewSquareRoot(operands ...types.Float) SquareRoot {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Sqrt(val)
	}
}

type DotProduct types.Value[[2][]float64, float64]

/*
NewDotProduct calculates the dot product of two equal-length float slices.
*/
func NewDotProduct(vectors ...types.Value[any, []float64]) DotProduct {
	return func(in [2][]float64) float64 {
		a, b := in[0], in[1]
		if len(vectors) >= 2 {
			if vectors[0] != nil {
				a = vectors[0](in)
			}
			if vectors[1] != nil {
				b = vectors[1](in)
			}
		}
		n := min(len(a), len(b))
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * b[i]
		}
		return sum
	}
}

type Transpose types.Value[[][]float64, [][]float64]

/*
NewTranspose transposes a 2D float matrix.
*/
func NewTranspose(operands ...types.Value[any, [][]float64]) Transpose {
	return func(m [][]float64) [][]float64 {
		mat := m
		if len(operands) > 0 && operands[0] != nil {
			mat = operands[0](m)
		}
		if len(mat) == 0 {
			return nil
		}
		rows := len(mat)
		cols := len(mat[0])
		out := make([][]float64, cols)
		for i := range cols {
			out[i] = make([]float64, rows)
			for j := range rows {
				out[i][j] = mat[j][i]
			}
		}
		return out
	}
}

type Identity types.Value[int, [][]float64]

/*
NewIdentity returns an n x n identity matrix.
*/
func NewIdentity(n ...types.Integer) Identity {
	return func(in int) [][]float64 {
		dim := in
		if len(n) > 0 && n[0] != nil {
			dim = n[0](in)
		}
		m := make([][]float64, dim)
		for i := range dim {
			m[i] = make([]float64, dim)
			m[i][i] = core.Unit
		}
		return m
	}
}

type SpectralRadius2 types.Value[[2][2]float64, float64]

/*
NewSpectralRadius2 calculates the spectral radius of a 2x2 matrix.
Complex eigenvalues use modulus; real eigenvalues use maximum absolute value.
*/
func NewSpectralRadius2(operands ...types.Value[any, [2][2]float64]) SpectralRadius2 {
	return func(matrix [2][2]float64) float64 {
		mat := matrix
		if len(operands) > 0 && operands[0] != nil {
			mat = operands[0](matrix)
		}
		trace := mat[0][0] + mat[1][1]
		determinant := mat[0][0]*mat[1][1] - mat[0][1]*mat[1][0]
		discriminant := trace*trace - 4*determinant

		if discriminant < 0 {
			modulus := math.Sqrt(-discriminant)
			realPart := trace / 2
			imagPart := modulus / 2
			return math.Sqrt(realPart*realPart + imagPart*imagPart)
		}

		rootHigh := (trace + math.Sqrt(discriminant)) / 2
		rootLow := (trace - math.Sqrt(discriminant)) / 2

		return math.Max(math.Abs(rootHigh), math.Abs(rootLow))
	}
}

type Sum types.Value[float64, float64]

/*
NewSum creates a stateful accumulator that sums incoming values.
The state is perfectly closed over, requiring zero structs.
*/
func NewSum(values ...types.Float) Sum {
	var sum float64
	return func(in float64) float64 {
		if len(values) > 0 {
			var s float64
			for _, v := range values {
				if v != nil {
					s += v(in)
				}
			}
			return s
		}
		sum += in
		return sum
	}
}

type Exp types.Value[float64, float64]

/*
NewExp calculates the base-e exponential.
*/
func NewExp(operands ...types.Float) Exp {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Exp(val)
	}
}

type Clamp types.Value[float64, float64]

/*
NewClamp creates a state-free closure that restricts input to the specified bounds.
*/
func NewClamp(lower, upper types.Float) Clamp {
	return func(in float64) float64 {
		l := in
		if lower != nil {
			l = lower(in)
		}
		u := in
		if upper != nil {
			u = upper(in)
		}
		if in < l {
			return l
		}
		if in > u {
			return u
		}
		return in
	}
}

