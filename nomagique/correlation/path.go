package correlation

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Path owns timestamp acceptance and the retained observation sequence. Each
arrival is [2]float64{at, value}; it yields *[2][]float64 where
[0] is {at, value, priorCount, count, accepted, restated, hasSpan, from, to}
and [1] is the flattened retained observations {at0, value0, at1, value1, ...}.
Optional retention is an adapter-native Primitive (e.g. adaptive.Window) that
publishes shed_ratio.
*/
type Path struct {
	*core.PrimitiveError
	retention    core.Primitive
	observations [][2]float64
	out          [2][]float64
	flags        []float64
	flat         []float64
}

func NewPath(retention ...core.Primitive) core.Primitive {
	path := &Path{
		PrimitiveError: core.NewPrimitiveError(),
		flags:          make([]float64, 9),
	}

	if len(retention) > 0 {
		path.retention = retention[0]
	}

	return path
}

func (op *Path) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := *(*[2]float64)(arriving)
			priorCount := len(op.observations)
			last := 0.0

			if priorCount > 0 {
				last = op.observations[priorCount-1][0]
			}

			accepted := priorCount == 0 || sample[0] >= last
			restated := priorCount > 0 && sample[0] == last

			if accepted {
				observations := op.observations

				if restated {
					observations = slices.Clone(observations)
					observations[priorCount-1] = sample
				} else {
					observations = append(observations, sample)
				}

				if op.retention != nil {
					shedRatio := 1.0

					for pointer := range op.retention.Next(data.NewValue(unsafe.Pointer(&sample[1])).Next(nil)) {
						shedRatio = *(*float64)(pointer)
					}

					if err := op.retention.Error(); err != nil {
						op.Error(err)
						return
					}

					if shedRatio < 1 && len(observations) > 2 {
						retained := int(math.Max(2, math.Floor(float64(len(observations))*shedRatio)))
						observations = slices.Clone(observations[len(observations)-retained:])
					}
				}

				op.observations = observations
			}

			from, through := 0.0, 0.0
			hasSpan := 0.0

			if len(op.observations) > 0 {
				from = op.observations[0][0]
				through = op.observations[len(op.observations)-1][0]
				hasSpan = 1
			}

			acceptedFlag, restatedFlag := 0.0, 0.0

			if accepted {
				acceptedFlag = 1
			}

			if restated {
				restatedFlag = 1
			}

			op.flags[0] = sample[0]
			op.flags[1] = sample[1]
			op.flags[2] = float64(priorCount)
			op.flags[3] = float64(len(op.observations))
			op.flags[4] = acceptedFlag
			op.flags[5] = restatedFlag
			op.flags[6] = hasSpan
			op.flags[7] = from
			op.flags[8] = through

			if cap(op.flat) < len(op.observations)*2 {
				op.flat = make([]float64, len(op.observations)*2)
			} else {
				op.flat = op.flat[:len(op.observations)*2]
			}

			for index, observation := range op.observations {
				op.flat[index*2] = observation[0]
				op.flat[index*2+1] = observation[1]
			}

			op.out[0] = append([]float64(nil), op.flags...)
			op.out[1] = append([]float64(nil), op.flat...)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
