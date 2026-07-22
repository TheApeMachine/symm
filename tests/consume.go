package tests

import "github.com/theapemachine/symm/types"

/*
Consume adapts Crypto.Tick to the Market afterStep contract. Production keeps
the thesis; the fixture loop only needs the error.
*/
func Consume(tick func() (*types.Thesis, error)) func() error {
	return func() error {
		_, err := tick()
		return err
	}
}
