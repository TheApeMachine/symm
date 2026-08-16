package equation

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*RatioScale derives the typical retained ratio against its own baseline.
Its map carries "values" (array of floats) and "baseline", producing "result" (median/baseline).*/
type RatioScale struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*RatioScale)(nil)

/*NewRatioScale returns a RatioScale primitive.*/
func NewRatioScale(initial types.Input[types.Map[string, types.Value[float64]]]) *RatioScale {
	return &RatioScale{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*Write stages the ratio scale map.*/
func (rs *RatioScale) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		rs.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ratio_scale: input is nil",
			nil,
		))

		return
	}

	rs.next.Write(input)
	rs.err = nil
}

/*Read computes the ratio scale result = median(values) / baseline.*/
func (rs *RatioScale) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := rs.next.Read()

	if in.Error() != "" {
		rs.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return rs.next
	}

	mapping := in.Project().Read()
	baselineVal, hasBaseline := mapping.Get("baseline")

	if !hasBaseline {
		rs.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ratio_scale: missing baseline",
			nil,
		))

		return rs.next
	}

	baseline := baselineVal.Read()

	// Read values from map - stored as individual Value[float64] entries
	values := make([]float64, 0, 4)
	if valsMap, hasVals := mapping.Get("values"); hasVals {
		if sliceVals, ok := valsMap.(types.Map[string, types.Value[float64]]); ok {
			if iterVals := sliceVals.GetIterator(); iterVals != nil {
				for {
					v, stop := iterVals.Next()
					if stop {
						break
					}
					values = append(values, v.Read())
				}
			}
		}
	}

	if baseline <= 0 || len(values) == 0 {
		rs.err = nil
		rs.next.Write(types.NewInput(types.NewValue(mapping)))
		return rs.next
	}

	median, ready := statistic.MedianOf(values)

	if !ready || median <= 0 || math.IsNaN(median) || math.IsInf(median, 0) {
		rs.err = nil
		rs.next.Write(types.NewInput(types.NewValue(mapping)))
		return rs.next
	}

	result := median / baseline

	mapping.Put("result", types.NewValue(result))

	rs.next.Write(types.NewInput(types.NewValue(mapping)))
	rs.err = nil

	return rs.next
}

/*Project returns the current projected map.*/
func (rs *RatioScale) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return rs.next.Project()
}

/*Error reports any execution error.*/
func (rs *RatioScale) Error() string {
	if rs.err != nil {
		return rs.err.Error()
	}

	return rs.next.Error()
}

/*Close releases internal state.*/
func (rs *RatioScale) Close() error {
	if err := rs.initial.Close(); err != nil {
		return err
	}

	if err := rs.next.Close(); err != nil {
		return err
	}

	rs.err = nil

	return nil
}