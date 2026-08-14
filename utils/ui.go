package utils

import (
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

var publishSent atomic.Uint64
var publishDropped atomic.Uint64

/*
PublishCounters reports how many dashboard frames were accepted onto the UI
channel and how many were dropped because that channel was full.
*/
func PublishCounters() (sent uint64, dropped uint64) {
	return publishSent.Load(), publishDropped.Load()
}

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

	if capacity := cap(fluid); capacity > 0 && len(fluid) == capacity {
		data.Free()
		return
	}

	payload := data.MarshalAndFree()

	select {
	case fluid <- types.FluidFrame{
		Channel: channel,
		Payload: payload,
	}:
	default:
	}
}

/*
Publish sends replaceable dashboard state without letting an absent or slow UI
consume the market-data path. A full buffered channel is normal backpressure,
so that frame is freed before serialization and the trading path continues.
*/
func Publish(ui chan []byte, data datura.Map[any]) {
	publish(ui, data, false)
}

/*
PublishPriority preserves a lifecycle transition when the replaceable-state
queue is full by evicting its oldest dashboard frame. It remains non-blocking;
the Hub continues to own slow-client isolation.
*/
func PublishPriority(ui chan []byte, data datura.Map[any]) {
	publish(ui, data, true)
}

func publish(ui chan []byte, data datura.Map[any], priority bool) {
	if data == nil || ui == nil {
		data.Free()
		return
	}

	hasValue := false

	for key, value := range data {
		switch value := value.(type) {
		case []any:
			if len(value) == 0 {
				delete(data, key)
				continue
			}
		default:
			if value == nil {
				delete(data, key)
				continue
			}
		}

		hasValue = true
	}

	if !hasValue {
		data.Free()
		return
	}

	if capacity := cap(ui); capacity > 0 && len(ui) == capacity {
		if !priority {
			publishDropped.Add(1)
			data.Free()
			return
		}

		select {
		case <-ui:
		default:
		}
	}

	payload := data.MarshalAndFree()

	select {
	case ui <- payload:
		publishSent.Add(1)
		return
	default:
		publishDropped.Add(1)

		if cap(ui) > 0 {
			return
		}

		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"UI channel is saturated",
			nil,
		))
	}
}
