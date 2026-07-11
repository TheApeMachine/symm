package kraken

import "github.com/theapemachine/errnie"

type Data interface {
	Action() string
	IsSuccess() bool
}

func Validate(data Data) error {
	if data == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "kraken data: required, was nil", nil,
		))
	}

	if !data.IsSuccess() {
		return errnie.Error(errnie.Err(
			errnie.Validation, data.Action()+" failed", nil,
		))
	}

	return nil
}
