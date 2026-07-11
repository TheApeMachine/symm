package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Trade struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []TradeData `json:"data"`
}

type TradeData struct {
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Price     decimal.Decimal `json:"price"`
	Qty       float64         `json:"qty"`
	OrderType string          `json:"ord_type"`
	TradeID   int64           `json:"trade_id"`
	Timestamp time.Time       `json:"timestamp"`
}

type TradeDataSlice []TradeData

func NewTradeDataSlice(buf []byte) TradeDataSlice {
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
		data := TradeDataSlice{}
		if err := sonic.Unmarshal(buf, &data); err == nil && len(data) > 0 {
			return data
		}
	}

	frame := Trade{}
	errnie.Error(sonic.Unmarshal(buf, &frame))

	return frame.Data
}

type TradeVolumeRequest struct {
	Pairs []string `json:"pairs"`
}

const TradeVolumeEndpoint = "TradeVolume"

func NewTradeVolumeRequest(pairs []string) TradeVolumeRequest {
	return TradeVolumeRequest{
		Pairs: pairs,
	}
}

func (tv TradeVolumeRequest) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"pairs": tv.Pairs,
	})
}

type TradeSubscription struct {
	Channel string   `json:"channel"`
	Type    string   `json:"type"`
	Pairs   []string `json:"pairs"`
}

func NewTradeSubscription(pairs []string) TradeSubscription {
	return TradeSubscription{
		Channel: "trade",
		Type:    "subscribe",
		Pairs:   pairs,
	}
}

/*
TradeUnsubscription requests Kraken stop streaming the trade channel for
the given pairs. Used to release the heavier trading-tier feeds once a
symbol is demoted from the trading universe.
*/
type TradeUnsubscription struct {
	Pairs []string
}

func NewTradeUnsubscription(pairs []string) TradeUnsubscription {
	return TradeUnsubscription{Pairs: pairs}
}

func (ts TradeUnsubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "trade",
			"symbol":  ts.Pairs,
		},
	})
}

func (ts TradeSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": ts.Channel,
			"symbol":  ts.Pairs,
		},
	})
}
