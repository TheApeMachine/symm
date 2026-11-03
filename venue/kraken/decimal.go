package kraken

import "github.com/shopspring/decimal"

func dFloat(value float64) *decimal.Decimal {
	d := decimal.NewFromFloat(value)
	return &d
}

func dInt(value int64) *decimal.Decimal {
	d := decimal.NewFromInt(value)
	return &d
}
