package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Publish is a convenience function to send data to the frontend,
and takes care of marshalling and channel saturation.
*/
func Publish(ui chan []byte, data datura.Map[any]) {
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
		return
	}

	select {
	case ui <- data.MarshalAndFree():
		return
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"UI channel is saturated",
			nil,
		))
	}
}
