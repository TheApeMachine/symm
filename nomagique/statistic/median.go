package statistic

import (
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Median computes the median of finite samples stored under "sample/" keys. An
empty sample set is a valid provisional state with ready=0 and result=0.
*/
type Median struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Median)(nil)

func NewMedian(initial types.Input[types.Map[string, types.Value[float64]]]) *Median {
	return &Median{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

func (median *Median) Read() types.IO[types.Map[string, types.Value[float64]]] {
	if median.next.Error() != "" {
		return median.next
	}

	mapping := median.next.Project().Read()
	values, err := collectSamples(mapping, "median")

	if err != nil {
		median.next = types.NewErrorInput(mapping, err)
		return median.next
	}

	result := 0.0
	ready := 0.0

	if len(values) > 0 {
		slices.Sort(values)
		middle := len(values) / 2
		result = values[middle]
		if len(values)%2 == 0 {
			result = (values[middle-1] + values[middle]) / 2
		}
		ready = 1
	}

	mapping.Put("result", types.NewValue(result))
	mapping.Put("ready", types.NewValue(ready))
	mapping.Put("count", types.NewValue(float64(len(values))))
	median.initial = types.NewInput(types.NewValue(mapping))
	median.next = types.NewInput(types.NewValue(mapping))

	return median.next
}

func (median *Median) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	mapping, err := stageSamples(input, "median")

	if err != nil {
		median.next = types.NewErrorInput(mapping, err)
		return
	}

	median.next = types.NewInput(types.NewValue(mapping))
}

func (median *Median) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return median.next.Project()
}

func (median *Median) Error() string { return median.next.Error() }

func (median *Median) Close() error {
	if median.initial != nil {
		if err := median.initial.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"median: "+err.Error(),
				nil,
			))
		}
	}

	if median.next != nil {
		if err := median.next.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"median: "+err.Error(),
				nil,
			))
		}
	}

	median.next = types.NewInput[types.Map[string, types.Value[float64]]]()
	return nil
}

func stageSamples(
	input types.IO[types.Map[string, types.Value[float64]]],
	name string,
) (types.Map[string, types.Value[float64]], error) {
	if input == nil {
		return types.NewMap[string, types.Value[float64]](), errnie.Error(errnie.Err(
			errnie.Validation,
			name+": input is nil",
			nil,
		))
	}

	if input.Error() != "" {
		return types.NewMap[string, types.Value[float64]](), errnie.Error(errnie.Err(
			errnie.NotFound, input.Error(), nil,
		))
	}

	return input.Project().Read(), nil
}

func collectSamples(
	mapping types.Map[string, types.Value[float64]],
	name string,
) ([]float64, error) {
	values := make([]float64, 0, mapping.Len())

	for key, wrapped := range mapping.All() {
		if !strings.HasPrefix(key, "sample/") {
			continue
		}

		value := wrapped.Read()

		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				name+": "+key+" must be finite",
				nil,
			))
		}

		values = append(values, value)
	}

	return values, nil
}
