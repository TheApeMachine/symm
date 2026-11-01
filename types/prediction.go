package types

import "github.com/theapemachine/symm/decimal"

type Prediction struct {
	Symbol    string          `json:"symbol"`
	Timestamp uint64          `json:"timestamp"`
	Price     decimal.Decimal `json:"price"`
}
