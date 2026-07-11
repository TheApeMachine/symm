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

type OHLCDataSlice []OHLCData

func NewOHLCDataSlice(buf []byte) OHLCDataSlice {
	isArray := false
	for _, b := range buf {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b == '[' {
			isArray = true
		}
		break
	}

	if isArray {
		data := OHLCDataSlice{}
		if err := sonic.Unmarshal(buf, &data); err == nil && len(data) > 0 {
			return data
		}
	}

	frame := OHLC{}
	errnie.Error(sonic.Unmarshal(buf, &frame))

	return frame.Data
}

type OHLCSubscription struct {
	Channel string   `json:"channel"`
	Type    string   `json:"type"`
	Pairs   []string `json:"pairs"`
}

func NewOHLCSubscription(pairs []string) OHLCSubscription {
	return OHLCSubscription{
		Channel: "ohlc",
		Type:    "subscribe",
		Pairs:   pairs,
	}
}

func (ohlc OHLCSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": ohlc.Channel,
			"symbol":  ohlc.Pairs,
		},
	})
}
