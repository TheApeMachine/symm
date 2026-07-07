package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Ticker struct {
	Channel string       `json:"channel"`
	Type    string       `json:"type"`
	Data    []TickerData `json:"data"`
}

type TickerData struct {
	Symbol    string          `json:"symbol"`
	Bid       decimal.Decimal `json:"bid"`
	BidQty    float64         `json:"bid_qty"`
	Ask       decimal.Decimal `json:"ask"`
	AskQty    float64         `json:"ask_qty"`
	Last      decimal.Decimal `json:"last"`
	Volume    float64         `json:"volume"`
	Vwap      float64         `json:"vwap"`
	Low       decimal.Decimal `json:"low"`
	High      decimal.Decimal `json:"high"`
	Change    decimal.Decimal `json:"change"`
	ChangePct float64         `json:"change_pct"`
	Timestamp time.Time       `json:"timestamp"`
}

type TickerDataSlice []TickerData

func NewTickerDataSlice(buf []byte) TickerDataSlice {
	data := TickerDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, &data))
	return data
}
