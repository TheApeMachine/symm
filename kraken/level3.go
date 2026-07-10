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
		data := Level3DataSlice{}
		if err := sonic.Unmarshal(buf, &data); err == nil && len(data) > 0 {
			return data
		}
	}

	frame := Level3{}
	errnie.Error(sonic.Unmarshal(buf, &frame))

	return frame.Data
}

type Level3Subscription struct {
	Channel string   `json:"channel"`
	Type    string   `json:"type"`
	Pairs   []string `json:"pairs"`
}

func NewLevel3Subscription(pairs []string) Level3Subscription {
	return Level3Subscription{
		Channel: "level3",
		Type:    "subscribe",
		Pairs:   pairs,
	}
}

func (ls Level3Subscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": ls.Channel,
			"symbol":  ls.Pairs,
		},
	})
}
