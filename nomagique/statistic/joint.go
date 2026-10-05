package statistic

import (
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Joint owns causal multivariate moments and Mahalanobis departure energy.

Its native coordinates are coordinate:0 through coordinate:n-1.
It publishes snr, maturity, and support only from covariance held before the
current observation. The current vector updates the covariance after scoring.
*/
type Joint struct {
	*core.PrimitiveError
	dimension int
	count     float64
	mean      []float64
	m2        []float64
	delta     []float64
	centered  []float64
	system    []float64
	input     data.Map[string]
	output    data.Map[float64]
}

func NewJoint(dimension int) core.Primitive {
	op := &Joint{
		PrimitiveError: core.NewPrimitiveError(),
		dimension:      dimension,
		mean:           make([]float64, dimension),
		m2:             make([]float64, dimension*dimension),
		delta:          make([]float64, dimension),
		centered:       make([]float64, dimension),
		system:         make([]float64, dimension*(dimension+1)),
		input:          data.NewMap(),
		output:         data.NewOutputMap(),
	}

	if dimension < 1 {
		op.Error(core.ErrDomain)
		return op
	}

	for index := 0; index < dimension; index++ {
		key := "coordinate:" + strconv.Itoa(index)
		op.input.Values[key] = key
	}

	return op
}

func (op *Joint) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if op.Error() != nil {
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			defined := true

			for index := 0; index < op.dimension; index++ {
				key := "coordinate:" + strconv.Itoa(index)
				value, ok := values.Values[key]

				if !ok {
					defined = false
					break
				}

				op.centered[index] = value
			}

			if !defined {
				if !yield(arriving) {
					return
				}

				continue
			}

			if op.count > 1 {
				width := op.dimension + 1

				for row := 0; row < op.dimension; row++ {
					for column := 0; column < op.dimension; column++ {
						op.system[row*width+column] = op.m2[row*op.dimension+column] / (op.count - 1)
					}

					op.system[row*width+op.dimension] = op.centered[row]
				}

				solvable := true

				for column := 0; column < op.dimension; column++ {
					pivot := column
					pivotMagnitude := math.Abs(op.system[pivot*width+column])

					for row := column + 1; row < op.dimension; row++ {
						magnitude := math.Abs(op.system[row*width+column])

						if magnitude > pivotMagnitude {
							pivot = row
							pivotMagnitude = magnitude
						}
					}

					if pivotMagnitude == 0 {
						solvable = false
						break
					}

					if pivot != column {
						for entry := column; entry < width; entry++ {
							left := column*width + entry
							right := pivot*width + entry
							op.system[left], op.system[right] = op.system[right], op.system[left]
						}
					}

					pivotValue := op.system[column*width+column]

					for entry := column; entry < width; entry++ {
						op.system[column*width+entry] /= pivotValue
					}

					for row := 0; row < op.dimension; row++ {
						if row == column {
							continue
						}

						factor := op.system[row*width+column]

						for entry := column; entry < width; entry++ {
							op.system[row*width+entry] -= factor * op.system[column*width+entry]
						}
					}
				}

				if solvable {
					energy := 0.0

					for row := 0; row < op.dimension; row++ {
						energy += op.centered[row] * op.system[row*width+op.dimension]
					}

					op.output.Values["snr"] = energy / float64(op.dimension)
					op.output.Values["support"] = op.count
					op.output.Values["maturity"] = 1 - 1/op.count

					for range adapter.Next(data.NewValue(op.output)) {
					}
				}
			}

			nextCount := op.count + 1

			for index := 0; index < op.dimension; index++ {
				op.delta[index] = op.centered[index] - op.mean[index]
				op.mean[index] += op.delta[index] / nextCount
			}

			for index := 0; index < op.dimension; index++ {
				op.centered[index] -= op.mean[index]
			}

			for row := 0; row < op.dimension; row++ {
				for column := 0; column < op.dimension; column++ {
					op.m2[row*op.dimension+column] += op.delta[row] * op.centered[column]
				}
			}

			op.count = nextCount

			if !yield(arriving) {
				return
			}
		}
	}
}
