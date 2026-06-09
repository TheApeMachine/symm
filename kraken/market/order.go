package market

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/types"
)

type Level3Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token,omitempty"`
}

func NewLevel3Params(symbols []string) *Level3Params {
	return &Level3Params{
		Channel:  "level3",
		Symbol:   symbols,
		Snapshot: true,
	}
}

type Bid struct {
	Event      string    `json:"event"`
	OrderID    string    `json:"order_id"`
	LimitPrice float64   `json:"limit_price"`
	OrderQty   float64   `json:"order_qty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Ask struct {
	Event      string    `json:"event"`
	OrderID    string    `json:"order_id"`
	LimitPrice float64   `json:"limit_price"`
	OrderQty   float64   `json:"order_qty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Level3Update struct {
	Symbol   string `json:"symbol"`
	Checksum int    `json:"checksum"`
	Bids     []Bid  `json:"bids"`
	Asks     []Ask  `json:"asks"`
}

func (level3 *Level3Update) Unmarshal(message *types.SocketMessage) error {
	return sonic.Unmarshal(message.Data, level3)
}
