package transport

import "github.com/theapemachine/symm/nomagique/core"

// Fan presents one captured input run to every configured output endpoint.
// N input values and M output endpoints use the same operation. Endpoint order
// is deterministic. Each output owns an independent cursor over the snapshot;
// the original input is consumed once, never replayed per branch.
type Fan struct {
	core.PrimitiveError
	inputs   core.Primitive
	outputs  core.Primitive
	delivery core.Primitive
	current  core.Primitive
}

func NewFan(inputs, outputs core.Primitive) *Fan { return &Fan{inputs: inputs, outputs: outputs} }
func (fan *Fan) Next(in core.Primitive) core.Primitive {
	if fan.delivery == nil {
		captured := []core.Primitive{}
		core.Yield(
			NewIO(core.From(0)),
			NewApply(fan.inputs, in),
			func(held int, value core.Primitive) int { captured = append(captured, value); return held },
			fan,
		)
		values := []core.Primitive{}
		core.Yield(
			NewIO(core.From(0)),
			fan.outputs,
			func(held int, target core.Primitive) int {
				core.Yield(
					NewIO(core.From(0)),
					NewApply(target, NewIO(captured...)),
					func(held int, value core.Primitive) int { values = append(values, value); return held },
					fan,
				)
				return held
			},
			fan,
		)
		fan.delivery = NewIO(values...)
	}
	value := fan.delivery.Next(nil)
	if value == nil {
		fan.delivery = nil
	} else {
		fan.current = value
	}
	return value
}
func (fan *Fan) Read() any { return core.To[any](fan.current) }
