package types

import "github.com/theapemachine/api-go/v2/pkg/decimal"

type Prediction struct {
	Symbol    string          `json:"symbol"`
	Timestamp uint64          `json:"timestamp"`
	Price     decimal.Decimal `json:"price"`
}
