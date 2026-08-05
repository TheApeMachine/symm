package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

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
		data.Free()
		return
	}

	select {
	case ui <- data.MarshalAndFree():
		return
	default:
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
