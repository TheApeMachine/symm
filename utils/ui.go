package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
PublishFluid appends one manifold frame to the lock-free fluid transport. The
transport is an unbounded MapReduce queue, so a publish never blocks and never
drops; the WebRTC hub consumer drains it onto the channel it names.
*/
func PublishFluid(
	fluid *transport.MapReduce[types.FluidFrame], channel string, data datura.Map[any],
) {
	if data == nil || fluid == nil {
		data.Free()
		return
	}

	fluid.Push(types.FluidFrame{
		Channel: channel,
		Payload: data.MarshalAndFree(),
	})
}
