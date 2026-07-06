package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Order struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id"`
}

type PaperOrder struct {
	ID             string  `json:"id"`
	Pair           string  `json:"pair"`
	Price          float64 `json:"price"`
	ReservedAmount float64 `json:"reserved_amount"`
	ReservedAsset  string  `json:"reserved_asset"`
	Side           string  `json:"side"`
	Type           string  `json:"type"`
	Volume         float64 `json:"volume"`
	CreatedAt      string  `json:"created_at"`
}

type PaperOrderSlice []PaperOrder

func NewPaperOrderSlice(buf []byte) *PaperOrderSlice {
	data := &PaperOrderSlice{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}

type LimitOrderParams struct {
	OrderType    string  `json:"order_type"`
	Side         string  `json:"side"`
	LimitPrice   float64 `json:"limit_price"`
	OrderUserref int     `json:"order_userref"`
	OrderQty     float64 `json:"order_qty"`
	Symbol       string  `json:"symbol"`
	Token        string  `json:"token"`
}

type StoplossOrderParams struct {
	OrderType string        `json:"order_type"`
	Side      string        `json:"side"`
	OrderQty  int           `json:"order_qty"`
	Symbol    string        `json:"symbol"`
	Triggers  TriggerParams `json:"triggers"`
	Token     string        `json:"token"`
}

type TriggerParams struct {
	Reference string  `json:"reference"`
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type"`
}

func (order *Order) Marshal() []byte {
	buf, err := sonic.Marshal(order)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	return buf
}
