package equation

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

// Mean combines ready evidence and returns probability calculation errors.
// Its map carries "ready" and "values" (array of floats), producing "result" (geomean).
type Mean struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Mean)(nil)

// NewMean returns a Mean combination primitive.
func NewMean(initial types.Input[types.Map[string, types.Value[float64]]]) *Mean {
	return &Mean{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

// Write stages the mean map.
func (mean *Mean) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		mean.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"mean: input is nil",
			nil,
		))

		return
	}

	mean.next.Write(input)
	mean.err = nil
}

// Read computes the mean result = geomean of values if ready, else 0.
func (mean *Mean) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := mean.next.Read()

	if in.Error() != "" {
		mean.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return mean.next
	}

	mapping := in.Project().Read()
	readyVal, hasReady := mapping.Get("ready")

	if !hasReady {
		mean.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"mean: missing ready",
			nil,
		))

		return mean.next
	}

	ready := readyVal.Read()

	if !ready {
		mapping.Put("result", types.NewValue(0.0))
		mean.next.Write(types.NewInput(types.NewValue(mapping)))
		mean.err = nil

		return mean.next
	}

	// Read values from map - stored as individual Value[float64] entries
	values := make([]float64, 0, 4)
	if valsMap, hasVals := mapping.Get("values"); hasVals {
		if sliceVals, ok := valsMap.(types.Map[string, types.Value[float64]]); ok {
			for k, v := range *sliceVals.store {
				if v.Zero != (types.Value[float64]{}) {
					values = append(values, v.Read())
				}
			}
		}
	}

	if len(values) == 0 {
		mapping.Put("result", types.NewValue(0.0))
		mean.next.Write(types.NewInput(types.NewValue(mapping)))
		mean.err = nil

		return mean.next
	}

	score, err := statistic.EvidenceGeomean(values...)

	if err != nil {
		mean.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: combine empirical evidence",
			err,
		))

		return mean.next
	}

	mapping.Put("result", types.NewValue(score))

	mean.next.Write(types.NewInput(types.NewValue(mapping)))
	mean.err = nil

	return mean.next
}

/*Project returns the current projected map.*/
func (mean *Mean) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return mean.next.Project()
}

/*Error reports any execution error.*/
func (mean *Mean) Error() string {
	if mean.err != nil {
		return mean.err.Error()
	}

	return mean.next.Error()
}

/*Close releases internal state.*/
func (mean *Mean) Close() error {
	if err := mean.initial.Close(); err != nil {
		return err
	}

	if err := mean.next.Close(); err != nil {
		return err
	}

	mean.err = nil

	return nil
}