package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Level3 struct {
	Channel string       `json:"channel"`
	Type    string       `json:"type"`
	Data    []Level3Data `json:"data"`
}

type Level3Data struct {
	Symbol    string        `json:"symbol"`
	Timestamp time.Time     `json:"timestamp"`
	Checksum  uint32        `json:"checksum"`
	Bids      []Level3Order `json:"bids"`
	Asks      []Level3Order `json:"asks"`
}

type Level3Order struct {
	Event      string    `json:"event,omitempty"`
	OrderID    string    `json:"order_id"`
	LimitPrice float64   `json:"limit_price"`
	OrderQty   float64   `json:"order_qty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Level3DataSlice []Level3Data

func NewLevel3DataSlice(buf []byte) Level3DataSlice {
	data := Level3DataSlice{}
	errnie.Error(sonic.Unmarshal(buf, &data))
	return data
}
