package types

import "github.com/shopspring/decimal"

type Prediction struct {
	Symbol    string          `json:"symbol"`
	Timestamp uint64          `json:"timestamp"`
	Price     decimal.Decimal `json:"price"`
}
