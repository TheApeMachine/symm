package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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
Publish sends replaceable dashboard state without letting an absent or slow UI
consume the market-data path. A full buffered channel is normal backpressure,
so that frame is freed before serialization and the trading path continues.
*/
func Publish(ui chan []byte, data datura.Map[any]) {
	if data == nil || ui == nil {
		data.Free()
		return
	}

	select {
	case ui <- data.MarshalAndFree():
		return
	default:
		errnie.Warn("UI channel is saturated")
	}
}
