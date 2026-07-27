package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func Publish(ui chan []byte, data datura.Map[any]) {
	if ui == nil || data == nil {
		data.Free()
		return
	}

	select {
	case ui <- data.MarshalAndFree():
	default:
		data.Free()
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
	}
}
