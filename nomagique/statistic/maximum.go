package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Maximum returns the greatest finite sample stored under "sample/" keys. An
empty sample set is provisional and produces ready=0.
*/
type Maximum struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Maximum)(nil)

func NewMaximum(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Maximum {
	return &Maximum{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

func (maximum *Maximum) Read() types.IO[types.Map[string, types.Value[float64]]] {
	if maximum.next.Error() != "" {
		return maximum.next
	}

	mapping := maximum.next.Project().Read()
	values, err := collectSamples(mapping, "maximum")

	if err != nil {
		maximum.next = types.NewErrorInput(mapping, err)
		return maximum.next
	}

	result := 0.0
	ready := 0.0

	if len(values) > 0 {
		result = -math.MaxFloat64

		for _, value := range values {
			result = math.Max(result, value)
		}

		ready = 1
	}

	mapping.Put("result", types.NewValue(result))
	mapping.Put("ready", types.NewValue(ready))
	mapping.Put("count", types.NewValue(float64(len(values))))
	maximum.initial = types.NewInput(types.NewValue(mapping))
	maximum.next = types.NewInput(types.NewValue(mapping))

	return maximum.next
}

func (maximum *Maximum) Write(
	input types.IO[types.Map[string, types.Value[float64]]],
) {
	mapping, err := stageSamples(input, "maximum")

	if err != nil {
		maximum.next = types.NewErrorInput(mapping, err)
		return
	}

	maximum.next = types.NewInput(types.NewValue(mapping))
}

func (maximum *Maximum) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return maximum.next.Project()
}

func (maximum *Maximum) Error() string { return maximum.next.Error() }

func (maximum *Maximum) Close() error {
	if maximum.initial != nil {
		if err := maximum.initial.Close(); err != nil {
			return err
		}
	}
	if maximum.next != nil {
		if err := maximum.next.Close(); err != nil {
			return err
		}
	}
	maximum.next = types.NewInput[types.Map[string, types.Value[float64]]]()
	return nil
}
