package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
PublishFluid sends one manifold frame to a named fluid data channel, dropping it
when the transport is already saturated on the same terms as Publish.
*/
func PublishFluid(
	fluid chan types.FluidFrame, channel string, data datura.Map[any],
) {
	if data == nil || fluid == nil {
		data.Free()
		return
	}

	select {
	case fluid <- types.FluidFrame{
		Channel: channel,
		Payload: data.MarshalAndFree(),
	}:
	default:
		errnie.Warn("fluid channel is saturated")
	}
}

/*
Publish appends one replaceable dashboard wire frame to the thesis's lock-free
UI transport. The frame is retained until the hub consumer drains it, so a
publish never blocks and never drops a frame regardless of marketplace heat.
*/
func Publish(ui *transport.MapReduce[[]byte], data datura.Map[any]) {
	frame := data.MarshalAndFree()

	if ui == nil {
		return
	}

	ui.Push(frame)
}
