package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Ticker struct {
	Channel string       `json:"channel"`
	Type    string       `json:"type"`
	Data    []TickerData `json:"data"`
}

type TickerData struct {
	Symbol    string    `json:"symbol"`
	Bid       float64   `json:"bid"`
	BidQty    float64   `json:"bid_qty"`
	Ask       float64   `json:"ask"`
	AskQty    float64   `json:"ask_qty"`
	Last      float64   `json:"last"`
	Volume    float64   `json:"volume"`
	Vwap      float64   `json:"vwap"`
	Low       float64   `json:"low"`
	High      float64   `json:"high"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	Timestamp time.Time `json:"timestamp"`
}

type TickerDataSlice []TickerData

func NewTickerDataSlice(buf []byte) TickerDataSlice {
	data := TickerDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, &data))
	return data
}
