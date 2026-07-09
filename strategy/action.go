package strategy

import "github.com/krakenfx/api-go/v2/pkg/decimal"

type Action string

const (
	ActionHold Action = "hold"
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

type Intent struct {
	Symbol     string
	Action     Action
	Size       decimal.Decimal
	Edge       float64
	Velocity   float64
	Confidence float64
	Thesis     *Thesis
}
