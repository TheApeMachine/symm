package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
JointSNR3 owns causal covariance of three residual coordinates and evaluates
their Mahalanobis energy against the covariance that existed before the current
observation.
*/
type JointSNR3 struct {
	*core.PrimitiveError
	count      float64
	mean       [3]float64
	m2         [9]float64
	covariance [9]float64
	inverse    [9]float64
	lu         [9]float64
	pivots     [3]int
	column     [3]float64
	input      data.Map[string]
	output     data.Map[float64]
}

func NewJointSNR3() core.Primitive {
	output := data.NewOutputMap()
	output.Values["snr"] = 0
	output.Values["maturity"] = 0

	return &JointSNR3{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"first", "first",
			"second", "second",
			"third", "third",
		),
		output: output,
	}
}

func (op *JointSNR3) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			first, firstOK := values.Values["first"]
			second, secondOK := values.Values["second"]
			third, thirdOK := values.Values["third"]

			if !firstOK || !secondOK || !thirdOK {
				if !yield(arriving) {
					return
				}
				continue
			}

			vector := [3]float64{first, second, third}

			if op.count > 1 {
				op.output.Values["maturity"] = 1 - 1/op.count
				divisor := op.count - 1

				for index := range op.covariance {
					op.covariance[index] = op.m2[index] / divisor
				}

				if invertLU(
					op.covariance[:],
					op.inverse[:],
					3,
					op.lu[:],
					op.pivots[:],
					op.column[:],
				) {
					energy := 0.0

					for row := range 3 {
						projected := 0.0

						for column := range 3 {
							projected += op.inverse[row*3+column] * vector[column]
						}

						energy += vector[row] * projected
					}

					op.output.Values["snr"] = energy / 3

					for range adapter.Next(data.NewValue(op.output)) {
					}
				}
			}

			nextCount := op.count + 1
			var delta, adjusted [3]float64

			for index := range 3 {
				delta[index] = vector[index] - op.mean[index]
				op.mean[index] += delta[index] / nextCount
				adjusted[index] = vector[index] - op.mean[index]
			}

			for row := range 3 {
				for column := range 3 {
					op.m2[row*3+column] += delta[row] * adjusted[column]
				}
			}

			op.count = nextCount

			if !yield(arriving) {
				return
			}
		}
	}
}
