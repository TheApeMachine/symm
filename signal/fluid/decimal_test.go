package fluid

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func testDecimal(value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return *parsed
}

func testBookLevel(price string, quantity float64) kraken.BookLevel {
	return kraken.BookLevel{
		Price: testDecimal(price),
		Qty:   quantity,
	}
}
