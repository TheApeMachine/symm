package tests

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Consume adapts Crypto.Tick to the Market afterStep contract. An empty
measurement batch is an idle observation, not a fixture failure.
*/
func Consume(tick func() (*types.Thesis, error)) func() error {
	return func() error {
		_, err := tick()

		if errnie.IsPreconditionFailed(err) {
			return nil
		}

		return err
	}
}
