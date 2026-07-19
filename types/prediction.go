package types

import "github.com/krakenfx/api-go/v2/pkg/decimal"

type Prediction struct {
	Symbol    string          `json:"symbol"`
	Timestamp uint64          `json:"timestamp"`
	Price     decimal.Decimal `json:"price"`
}
