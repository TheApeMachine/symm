package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type OHLC struct {
	Channel   string     `json:"channel"`
	Type      string     `json:"type"`
	Timestamp time.Time  `json:"timestamp"`
	Data      []OHLCData `json:"data"`
}

type OHLCData struct {
	Symbol        string    `json:"symbol"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	Trades        int64     `json:"trades"`
	Volume        float64   `json:"volume"`
	Vwap          float64   `json:"vwap"`
	IntervalBegin time.Time `json:"interval_begin"`
	Interval      int64     `json:"interval"`
	Timestamp     time.Time `json:"timestamp"`
}

func NewOHLC(buf []byte) *OHLC {
	var ohlc OHLC

	if err := sonic.Unmarshal(buf, &ohlc); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid ohlc",
			err,
		))
	}

	return &ohlc
}

func (ohlc *OHLC) Action() string {
	return "ohlc"
}
