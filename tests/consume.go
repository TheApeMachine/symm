package tests

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Consume adapts Stack.Observe to the Market afterStep contract. An empty
measurement batch is an idle observation, not a fixture failure.
*/
func Consume(observe func() (*types.Thesis, error)) func() error {
	return func() error {
		_, err := observe()

		if errnie.IsPreconditionFailed(err) {
			return nil
		}

		return err
	}
}
