package causal

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Table coordinates causal estimation over observational tables. It delegates
interventional expectation and counterfactual reasoning to dedicated primitives.
*/
type Table struct {
	*core.PrimitiveError
	minimum        int
	linear         bool
	backdoor       *Backdoor
	counterfactual *Counterfactual
	stump          *Stump
	input          data.Map[string]
	output         data.Map[float64]
}

func NewTable(minimum int, evidence ...any) *Table {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["counterfactual"] = 0
	output.Values["noise"] = 0
	output.Values["precision"] = 0
	output.Values["defined"] = 0

	op := &Table{
		PrimitiveError: core.NewPrimitiveError(),
		minimum:        minimum,
		input:          data.NewMap("level", "level"),
		output:         output,
	}

	if len(evidence) < 5 {
		return op
	}

	rows, rowsOK := evidence[0].([][]float64)
	target, targetOK := evidence[1].(int)
	treatment, treatmentOK := evidence[2].(int)
	controls, controlsOK := evidence[3].([]int)
	linear, linearOK := evidence[4].(bool)

	if !rowsOK || !targetOK || !treatmentOK || !controlsOK || !linearOK {
		op.Error(core.ErrShape)
		return op
	}

	if len(rows) < minimum {
		op.Error(fmt.Errorf(
			"causal: %d observational rows available; need %d: %w",
			len(rows), minimum, core.ErrDomain,
		))
		return op
	}

	op.linear = linear

	if linear {
		op.backdoor = NewBackdoor(1e-15, rows, target, treatment, controls)

		if err := op.backdoor.Error(); err != nil {
			op.Error(err)
			return op
		}

		features := append(controls, treatment)
		op.counterfactual = NewCounterfactual(1e-15, rows, target, treatment, features)

		if err := op.counterfactual.Error(); err != nil {
			op.Error(err)
			return op
		}

		return op
	}

	op.stump = NewStump(rows, target, treatment, controls)

	if err := op.stump.Error(); err != nil {
		op.Error(err)
		return op
	}

	return op
}

func (op *Table) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			if adapter.Has("actual_0") {
				if op.counterfactual == nil {
					op.Error(core.ErrDomain)
					return
				}

				for range op.counterfactual.Next(data.NewValue(adapter)) {
				}

				if err := op.counterfactual.Error(); err != nil {
					op.Error(err)
					return
				}

				for key, value := range op.counterfactual.output.Values {
					op.output.Values[key] = value
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			if op.linear {
				if op.backdoor == nil {
					op.Error(core.ErrDomain)
					return
				}

				for range op.backdoor.Next(data.NewValue(adapter)) {
				}

				if err := op.backdoor.Error(); err != nil {
					op.Error(err)
					return
				}

				for key, value := range op.backdoor.output.Values {
					op.output.Values[key] = value
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			if op.stump == nil {
				op.Error(core.ErrDomain)
				return
			}

			for range op.stump.Next(data.NewValue(adapter)) {
			}

			if err := op.stump.Error(); err != nil {
				op.Error(err)
				return
			}

			for key, value := range op.stump.output.Values {
				op.output.Values[key] = value
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
