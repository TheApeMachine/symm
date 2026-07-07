package trader

import "github.com/krakenfx/api-go/v2/pkg/decimal"

func testDecimal(value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return *parsed
}
