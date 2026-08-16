package probability

import (
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Geomean combines positive finite samples stored under "sample/" keys and
writes "result" and "count" into the same map.
*/
type Geomean struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Geomean)(nil)

func NewGeomean(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Geomean {
	return &Geomean{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

func (geomean *Geomean) Read() types.IO[types.Map[string, types.Value[float64]]] {
	if geomean.next.Error() != "" {
		return geomean.next
	}

	mapping := geomean.next.Project().Read()
	count := 0
	logSum := 0.0

	for key, wrapped := range mapping.All() {
		if !strings.HasPrefix(key, "sample/") {
			continue
		}

		value := wrapped.Read()

		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			geomean.next = types.NewErrorInput(mapping, errnie.Error(errnie.Err(
				errnie.Validation,
				"geomean: "+key+" must be positive and finite",
				nil,
			)))

			return geomean.next
		}

		logSum += math.Log(value)
		count++
	}

	if count == 0 {
		geomean.next = types.NewErrorInput(mapping, errnie.Error(errnie.Err(
			errnie.Validation,
			"geomean: at least one sample is required",
			nil,
		)))

		return geomean.next
	}

	mapping.Put("result", types.NewValue(math.Exp(logSum/float64(count))))
	mapping.Put("count", types.NewValue(float64(count)))
	geomean.initial = types.NewInput(types.NewValue(mapping))
	geomean.next = types.NewInput(types.NewValue(mapping))

	return geomean.next
}

func (geomean *Geomean) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		mapping := types.NewMap[string, types.Value[float64]]()

		geomean.next = types.NewErrorInput(mapping, errnie.Error(errnie.Err(
			errnie.Validation,
			"input is nil",
			nil,
		)))

		return
	}

	if input.Error() != "" {
		mapping := types.NewMap[string, types.Value[float64]]()

		geomean.next = types.NewErrorInput(
			mapping, errnie.Error(errnie.Err(
				errnie.NotFound, input.Error(), nil,
			)),
		)

		return
	}

	geomean.next = types.NewInput(types.NewValue(input.Project().Read()))
}

func (geomean *Geomean) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return geomean.next.Project()
}

func (geomean *Geomean) Error() string { return geomean.next.Error() }

func (geomean *Geomean) Close() error {
	if geomean.initial != nil {
		if err := geomean.initial.Close(); err != nil {
			return err
		}
	}

	if geomean.next != nil {
		if err := geomean.next.Close(); err != nil {
			return err
		}
	}

	geomean.next = types.NewInput[types.Map[string, types.Value[float64]]]()

	return nil
}
