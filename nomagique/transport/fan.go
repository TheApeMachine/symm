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
	captured IO
	branch   IO
	buffer   IO
}

func NewFan(inputs, outputs core.Primitive) *Fan { return &Fan{inputs: inputs, outputs: outputs} }
func (fan *Fan) Next(in core.Primitive) core.Primitive {
	if fan.delivery == nil {
		fan.Error(fan.captured.appendRun(fan.inputs, in))

		if fan.outputs != nil {
			fan.Error(fan.outputs.Error())

			for target := fan.outputs.Next(nil); target != nil; target = fan.outputs.Next(nil) {
				fan.branch = IO{values: fan.captured.values}
				fan.Error(fan.buffer.appendRun(target, &fan.branch))
			}

			fan.Error(fan.outputs.Error())
		}
		fan.delivery = &fan.buffer
	}
	value := fan.delivery.Next(nil)

	if value == nil {
		fan.delivery = nil
		fan.captured.reset()
		fan.buffer.reset()
		fan.branch = IO{}

		return nil
	}

	fan.current = value
	return value
}
func (fan *Fan) Read() any { return core.To[any](fan.current) }
