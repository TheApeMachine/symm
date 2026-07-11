package strategy

import "github.com/krakenfx/api-go/v2/pkg/decimal"

type Action string

const (
	ActionHold Action = "hold"
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

type Intent struct {
	Symbol     string          `json:"symbol"`
	Action     Action          `json:"action"`
	Size       decimal.Decimal `json:"size"`
	Edge       decimal.Decimal `json:"edge"`
	Velocity   float64         `json:"velocity"`
	Confidence float64         `json:"confidence"`
	Thesis     *Thesis         `json:"-"`
}
