package tests

import (
	"github.com/theapemachine/symm/types"
)

/*
Consume adapts Crypto.Tick to the Market afterStep contract without hiding a
failed or incomplete production cut.
*/
func Consume(tick func() (*types.Thesis, error)) func() error {
	return func() error {
		_, err := tick()
		return err
	}
}
