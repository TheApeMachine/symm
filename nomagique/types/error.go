package types

import (
	"errors"

	"github.com/theapemachine/errnie"
)

func PrimitiveError(message string) error {
	return errnie.Error(errors.New("nomagique: " + message))
}
