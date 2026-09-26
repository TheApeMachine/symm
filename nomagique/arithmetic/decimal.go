package arithmetic

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/theapemachine/symm/nomagique/core"
)

func decimalPair(left, right []byte) (*decimal.Decimal, *decimal.Decimal, error) {
	a, err := core.ReadDecimal(left, "a")

	if err != nil {
		return nil, nil, err
	}
	b, err := core.ReadDecimal(right, "b")
	return a, b, err
}
