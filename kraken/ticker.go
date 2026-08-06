package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Ticker struct {
	Channel string       `json:"channel"`
	Type    string       `json:"type"`
	Data    []TickerData `json:"data"`
}

type TickerData struct {
	Symbol    string           `json:"symbol"`
	Bid       *decimal.Decimal `json:"bid"`
	BidQty    float64          `json:"bid_qty"`
	Ask       *decimal.Decimal `json:"ask"`
	AskQty    float64          `json:"ask_qty"`
	Last      *decimal.Decimal `json:"last"`
	Volume    float64          `json:"volume"`
	Vwap      float64          `json:"vwap"`
	Low       *decimal.Decimal `json:"low"`
	High      *decimal.Decimal `json:"high"`
	Change    *decimal.Decimal `json:"change"`
	ChangePct float64          `json:"change_pct"`
	Timestamp time.Time        `json:"timestamp"`
}

func NewTicker(buf []byte) *Ticker {
	var ticker Ticker

	if err := sonic.Unmarshal(buf, &ticker); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid ticker",
			err,
		))
	}

	return &ticker
}

func (ticker *Ticker) Action() string {
	return "ticker"
}

func (ticker *Ticker) IsSuccess() bool {
	return len(ticker.Data) > 0
}

type TickerSubscription struct {
	Pairs []string
}

func NewTickerSubscription(pairs []string) TickerSubscription {
	return TickerSubscription{Pairs: pairs}
}

func (subscription TickerSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "ticker",
			"symbol":  subscription.Pairs,
		},
	})
}
