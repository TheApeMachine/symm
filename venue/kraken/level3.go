package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

type Level struct {
	OrderID    string           `json:"order_id"`
	LimitPrice *decimal.Decimal `json:"limit_price"`
	OrderQty   *decimal.Decimal `json:"order_qty"`
	Timestamp  time.Time        `json:"timestamp"`
	// Event is add|modify|delete on updates; omitted on snapshots.
	Event string `json:"event,omitempty"`
}

type Level3Data struct {
	Symbol   string  `json:"symbol"`
	Checksum int     `json:"checksum"`
	Bids     []Level `json:"bids"`
	Asks     []Level `json:"asks"`
}

type Level3 struct {
	Channel  string       `json:"channel"`
	Type     string       `json:"type"`
	Data     []Level3Data `json:"data"`
	onChange func(market.MarketType)
}

func NewLevel3() *Level3 {
	return &Level3{
		Channel: "level3",
		Type:    "update",
		Data:    make([]Level3Data, 0),
	}
}

func DecodeLevel3(buf []byte) *Level3 {
	level3 := NewLevel3()

	if err := sonic.Unmarshal(buf, level3); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid level3",
			err,
		))

		return nil
	}

	return level3
}

func (level3 *Level3) Request() map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "level3",
			"snapshot": true,
		},
	}
}

func (level3 *Level3) OnChange(fn func(market.MarketType)) {
	level3.onChange = fn
}

func (level3 *Level3) MarshalJSON() ([]byte, error) {
	type alias Level3
	return sonic.Marshal((*alias)(level3))
}

func (level3 *Level3) UnmarshalJSON(data []byte) error {
	type alias Level3
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := level3.onChange
	*level3 = Level3(decoded)
	level3.onChange = onChange

	if level3.Data == nil {
		level3.Data = make([]Level3Data, 0)
	}

	if level3.onChange != nil {
		level3.onChange(level3)
	}

	return nil
}

func (level3 *Level3) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Level3)

	if !ok || book == nil || level3 == nil {
		return other
	}

	rows := make([]*market.Level3Data, 0, len(level3.Data))

	for _, data := range level3.Data {
		bids := make([]*market.Level3Order, len(data.Bids))

		for i, bid := range data.Bids {
			bids[i] = &market.Level3Order{
				OrderID:    bid.OrderID,
				LimitPrice: bid.LimitPrice,
				OrderQty:   bid.OrderQty,
				Timestamp:  bid.Timestamp,
				Event:      bid.Event,
			}
		}

		asks := make([]*market.Level3Order, len(data.Asks))

		for i, ask := range data.Asks {
			asks[i] = &market.Level3Order{
				OrderID:    ask.OrderID,
				LimitPrice: ask.LimitPrice,
				OrderQty:   ask.OrderQty,
				Timestamp:  ask.Timestamp,
				Event:      ask.Event,
			}
		}

		rows = append(rows, &market.Level3Data{
			Symbol:   data.Symbol,
			Checksum: data.Checksum,
			Bids:     bids,
			Asks:     asks,
		})
	}

	out := market.NewLevel3()
	out.Channel = level3.Channel
	out.Type = level3.Type
	out.Data = rows

	return out.ToMarket(book)
}

func (level3 *Level3) FromMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Level3)

	if !ok || book == nil || level3 == nil {
		return level3
	}

	level3.Channel = book.Channel
	level3.Type = book.Type
	level3.Data = level3.Data[:0]

	for _, data := range book.Data {
		if data == nil {
			continue
		}

		bids := make([]Level, len(data.Bids))

		for i, bid := range data.Bids {
			if bid == nil {
				continue
			}

			bids[i] = Level{
				OrderID:    bid.OrderID,
				LimitPrice: bid.LimitPrice,
				OrderQty:   bid.OrderQty,
				Timestamp:  bid.Timestamp,
				Event:      bid.Event,
			}
		}

		asks := make([]Level, len(data.Asks))

		for i, ask := range data.Asks {
			if ask == nil {
				continue
			}

			asks[i] = Level{
				OrderID:    ask.OrderID,
				LimitPrice: ask.LimitPrice,
				OrderQty:   ask.OrderQty,
				Timestamp:  ask.Timestamp,
				Event:      ask.Event,
			}
		}

		level3.Data = append(level3.Data, Level3Data{
			Symbol:   data.Symbol,
			Checksum: data.Checksum,
			Bids:     bids,
			Asks:     asks,
		})
	}

	return level3
}

type Level3Subscription struct {
	Method string                   `json:"method"`
	Params Level3SubscriptionParams `json:"params"`
}

type Level3SubscriptionParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth,omitempty"`
	Snapshot bool     `json:"snapshot,omitempty"`
	Token    string   `json:"token,omitempty"`
}

func NewLevel3Subscription(symbols []string, token string, depth int) Level3Subscription {
	return Level3Subscription{
		Method: "subscribe",
		Params: Level3SubscriptionParams{
			Channel:  "level3",
			Symbol:   symbols,
			Depth:    depth,
			Snapshot: true,
			Token:    token,
		},
	}
}

func NewLevel3Unsubscription(symbols []string, token string) Level3Subscription {
	return Level3Subscription{
		Method: "unsubscribe",
		Params: Level3SubscriptionParams{
			Channel: "level3",
			Symbol:  symbols,
			Token:   token,
		},
	}
}
