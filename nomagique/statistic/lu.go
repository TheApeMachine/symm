package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SolveLU solves A x = b using LU decomposition with partial pivoting.

Each arrival is *[2][]float64 {a, b}: a is p×p row-major, b has length p, and
neither is modified. It yields *[]float64 x of length p, or an empty slice when
A is singular to working precision, matching the rank-deficiency signal
callers expect from a normal-equations solve. Scratch buffers are reused.
*/
type SolveLU struct {
	*core.PrimitiveError
	lu  []float64
	pvt []int
	out []float64
}

func NewSolveLU() *SolveLU {
	return &SolveLU{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *SolveLU) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			system := (*[2][]float64)(arriving)
			a, b := system[0], system[1]
			p := len(b)

			if len(a) < p*p {
				op.Error(core.ErrShape)
				return
			}

			if cap(op.lu) < p*p {
				op.lu = make([]float64, p*p)
				op.pvt = make([]int, p)
				op.out = make([]float64, p)
			}

			lu, pvt, x := op.lu[:p*p], op.pvt[:p], op.out[:p]
			copy(lu, a)
			copy(x, b)
			singular := false

			for i := range p {
				pvt[i] = i
			}

			for k := 0; k < p && !singular; k++ {
				maxVal := math.Abs(lu[k*p+k])
				maxRow := k

				for i := k + 1; i < p; i++ {
					val := math.Abs(lu[i*p+k])

					if val > maxVal {
						maxVal = val
						maxRow = i
					}
				}

				if maxVal <= 1e-15 || math.IsNaN(maxVal) {
					singular = true
					continue
				}

				if maxRow != k {
					pvt[k], pvt[maxRow] = pvt[maxRow], pvt[k]

					for j := 0; j < p; j++ {
						lu[k*p+j], lu[maxRow*p+j] = lu[maxRow*p+j], lu[k*p+j]
					}

					x[k], x[maxRow] = x[maxRow], x[k]
				}

				pivotVal := lu[k*p+k]

				for i := k + 1; i < p; i++ {
					factor := lu[i*p+k] / pivotVal
					lu[i*p+k] = factor

					for j := k + 1; j < p; j++ {
						lu[i*p+j] -= factor * lu[k*p+j]
					}

					x[i] -= factor * x[k]
				}
			}

			if singular {
				x = x[:0]
			}

			for i := len(x) - 1; i >= 0; i-- {
				sum := x[i]

				for j := i + 1; j < p; j++ {
					sum -= lu[i*p+j] * x[j]
				}

				x[i] = sum / lu[i*p+i]
			}

			if !yield(unsafe.Pointer(&x)) {
				return
			}
		}
	}
}

/*
InvertLU computes A⁻¹ using LU decomposition with partial pivoting.

Each arrival is *[]float64 a, a p×p row-major matrix that is not modified. It
yields *[]float64 holding the p×p row-major inverse, or an empty slice when A
is singular to working precision. Scratch buffers are reused.
*/
type InvertLU struct {
	*core.PrimitiveError
	lu  []float64
	pvt []int
	col []float64
	out []float64
}

func NewInvertLU() *InvertLU {
	return &InvertLU{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *InvertLU) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			a := *(*[]float64)(arriving)
			p := int(math.Round(math.Sqrt(float64(len(a)))))

			if p*p != len(a) {
				op.Error(core.ErrShape)
				return
			}

			if cap(op.lu) < p*p {
				op.lu = make([]float64, p*p)
				op.pvt = make([]int, p)
				op.col = make([]float64, p)
				op.out = make([]float64, p*p)
			}

			lu, pvt, col, inv := op.lu[:p*p], op.pvt[:p], op.col[:p], op.out[:p*p]
			copy(lu, a)
			singular := false

			for i := range p {
				pvt[i] = i
			}

			for k := 0; k < p && !singular; k++ {
				maxVal := math.Abs(lu[k*p+k])
				maxRow := k

				for i := k + 1; i < p; i++ {
					val := math.Abs(lu[i*p+k])

					if val > maxVal {
						maxVal = val
						maxRow = i
					}
				}

				if maxVal <= 1e-15 || math.IsNaN(maxVal) {
					singular = true
					continue
				}

				if maxRow != k {
					pvt[k], pvt[maxRow] = pvt[maxRow], pvt[k]

					for j := 0; j < p; j++ {
						lu[k*p+j], lu[maxRow*p+j] = lu[maxRow*p+j], lu[k*p+j]
					}
				}

				pivotVal := lu[k*p+k]

				for i := k + 1; i < p; i++ {
					factor := lu[i*p+k] / pivotVal
					lu[i*p+k] = factor

					for j := k + 1; j < p; j++ {
						lu[i*p+j] -= factor * lu[k*p+j]
					}
				}
			}

			columns := p

			if singular {
				inv = inv[:0]
				columns = 0
			}

			for j := 0; j < columns; j++ {
				for i := range p {
					col[i] = 0.0
				}

				for i := range p {
					if pvt[i] == j {
						col[i] = 1.0
						break
					}
				}

				for i := range p {
					sum := col[i]

					for k := 0; k < i; k++ {
						sum -= lu[i*p+k] * col[k]
					}

					col[i] = sum
				}

				for i := p - 1; i >= 0; i-- {
					sum := col[i]

					for k := i + 1; k < p; k++ {
						sum -= lu[i*p+k] * col[k]
					}

					col[i] = sum / lu[i*p+i]
				}

				for i := range p {
					inv[i*p+j] = col[i]
				}
			}

			if !yield(unsafe.Pointer(&inv)) {
				return
			}
		}
	}
}
