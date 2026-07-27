package utils

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func Publish(ui chan []byte, data datura.Map[any]) {
	select {
	case ui <- data.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
	}
}
