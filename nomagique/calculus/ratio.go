package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Ratio normalizes positive evidence against a positive baseline only when the
map's numeric "ready" gate is non-zero.
*/
type Ratio struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Ratio)(nil)

func NewRatio(initial types.Input[scalarMap]) *Ratio {
	return &Ratio{initial: initial, next: types.NewInput[scalarMap]()}
}

func (ratio *Ratio) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "ratio")
	if err != nil {
		ratio.next = types.NewErrorInput(mapping, err)
		return
	}
	ratio.next = scalarInput(mapping)
}

func (ratio *Ratio) Read() types.IO[scalarMap] {
	if ratio.next.Error() != "" {
		return ratio.next
	}

	mapping := ratio.next.Project().Read()
	value, hasValue := scalar(mapping, "value")
	baseline, hasBaseline := scalar(mapping, "baseline")
	ready, hasReady := scalar(mapping, "ready")
	if !hasValue || !hasBaseline || !hasReady {
		ratio.next = types.NewErrorInput(mapping,
			scalarValidation("ratio", "missing value, baseline, or ready"))
		return ratio.next
	}

	result := 0.0
	if ready != 0 && value > 0 && baseline > 0 &&
		!math.IsNaN(value) && !math.IsInf(value, 0) &&
		!math.IsNaN(baseline) && !math.IsInf(baseline, 0) {
		result = value / baseline
	}

	putScalar(mapping, "result", result)
	ratio.initial = scalarInput(mapping)
	ratio.next = scalarInput(mapping)
	return ratio.next
}

func (ratio *Ratio) Project() types.Value[scalarMap] { return ratio.next.Project() }
func (ratio *Ratio) Error() string                   { return ratio.next.Error() }
func (ratio *Ratio) Close() error {
	if ratio.initial != nil {
		if err := ratio.initial.Close(); err != nil {
			return err
		}
	}
	if ratio.next != nil {
		if err := ratio.next.Close(); err != nil {
			return err
		}
	}
	ratio.next = types.NewInput[scalarMap]()
	return nil
}
