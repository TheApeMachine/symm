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
	*core.PrimitiveError

	retention    core.Primitive
	observations []temporal.Price
	out          PathReading
}

func NewPath(retention ...core.Primitive) *Path {
	path := &Path{PrimitiveError: core.NewPrimitiveError()}

	if len(retention) > 0 {
		path.retention = retention[0]
	}

	return path
}

func (path *Path) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := *(*temporal.Price)(arriving)
			priorCount := len(path.observations)
			last := int64(0)

			if priorCount > 0 {
				last = path.observations[priorCount-1].At
			}

			accepted := priorCount == 0 || sample.At >= last
			restated := priorCount > 0 && sample.At == last

			if accepted {
				observations := path.observations

				if restated {
					observations = slices.Clone(observations)
					observations[priorCount-1] = sample
				}

				if !restated {
					observations = append(observations, sample)
				}

				if path.retention != nil {
					value := sample.Value
					reading := drive[float64, adaptive.WindowReading](path.retention, &value)

					if err := path.retention.Error(); err != nil {
						path.Error(err)
						return
					}

					if reading.ShedRatio < 1 && len(observations) > 2 {
						retained := int(math.Max(2, math.Floor(float64(len(observations))*reading.ShedRatio)))
						observations = slices.Clone(observations[len(observations)-retained:])
					}
				}

				path.observations = observations
			}

			from, through := int64(0), int64(0)

			if len(path.observations) > 0 {
				from = path.observations[0].At
				through = path.observations[len(path.observations)-1].At
			}

			path.out = PathReading{
				Price:        sample,
				Observations: path.observations[:len(path.observations):len(path.observations)],
				PriorCount:   float64(priorCount),
				Count:        float64(len(path.observations)),
				Accepted:     accepted,
				Restated:     restated,
				HasSpan:      len(path.observations) != 0,
				From:         from,
				To:           through,
			}

			if !yield(unsafe.Pointer(&path.out)) {
				return
			}
		}
	}
}
