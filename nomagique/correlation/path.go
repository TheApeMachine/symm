package correlation

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
PathReading is one accepted, restated, or rejected timestamped observation
together with the retained history.
*/
type PathReading struct {
	temporal.Price
	Observations []temporal.Price
	PriorCount   float64
	Count        float64
	Accepted     bool
	Restated     bool
	HasSpan      bool
	From         int64
	To           int64
}

/*
Path owns timestamp acceptance and the retained observation sequence.
*/
type Path struct {
	err          error
	retention    core.Primitive
	observations []temporal.Price
	out          PathReading
}

func NewPath(retention ...core.Primitive) core.Primitive {
	path := &Path{}

	if len(retention) > 0 {
		path.retention = retention[0]
	}

	return path
}

func (op *Path) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := *(*temporal.Price)(arriving)
			priorCount := len(op.observations)
			last := int64(0)

			if priorCount > 0 {
				last = op.observations[priorCount-1].At
			}

			accepted := priorCount == 0 || sample.At >= last
			restated := priorCount > 0 && sample.At == last

			if accepted {
				observations := op.observations

				if restated {
					observations = slices.Clone(observations)
					observations[priorCount-1] = sample
				}

				if !restated {
					observations = append(observations, sample)
				}

				if op.retention != nil {
					value := sample.Value
					reading := drive[float64, adaptive.WindowReading](op.retention, &value)

					if err := op.retention.Error(); err != nil {
						op.err = err
						return
					}

					if reading.ShedRatio < 1 && len(observations) > 2 {
						retained := int(math.Max(2, math.Floor(float64(len(observations))*reading.ShedRatio)))
						observations = slices.Clone(observations[len(observations)-retained:])
					}
				}

				op.observations = observations
			}

			from, through := int64(0), int64(0)

			if len(op.observations) > 0 {
				from = op.observations[0].At
				through = op.observations[len(op.observations)-1].At
			}

			op.out = PathReading{
				Price:        sample,
				Observations: op.observations[:len(op.observations):len(op.observations)],
				PriorCount:   float64(priorCount),
				Count:        float64(len(op.observations)),
				Accepted:     accepted,
				Restated:     restated,
				HasSpan:      len(op.observations) != 0,
				From:         from,
				To:           through,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Path) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
