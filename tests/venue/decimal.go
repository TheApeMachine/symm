package venue

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
Decimal parses an exact decimal string and panics on failure. Test fixtures
use it so float64 representation error never leaks into accounting assertions.
*/
func Decimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return parsed
}
