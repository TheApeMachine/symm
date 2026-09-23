package arithmetic

import (
	"math/big"

	"github.com/theapemachine/symm/nomagique/core"
)

func decimalPair(left, right []byte) (*big.Rat, *big.Rat, error) {
	a, err := core.ReadDecimal(left, "a")

	if err != nil {
		return nil, nil, err
	}
	b, err := core.ReadDecimal(right, "b")
	return a, b, err
}
