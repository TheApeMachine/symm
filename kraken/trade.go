package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Trade struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []TradeData `json:"data"`
}

type TradeData struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	OrderType string    `json:"ord_type"`
	TradeID   int64     `json:"trade_id"`
	Timestamp time.Time `json:"timestamp"`
}

type TradeDataSlice []TradeData

func NewTradeDataSlice(buf []byte) TradeDataSlice {
	data := TradeDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, &data))
	return data
}
