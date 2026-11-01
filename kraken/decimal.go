package kraken

import "github.com/theapemachine/symm/decimal"

func dFloat(value float64) *decimal.Decimal {
	return decimal.NewFromFloat64(value)
}

func dInt(value int64) *decimal.Decimal {
	return decimal.NewFromInt64(value)
}