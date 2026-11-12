package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

/*
Level is one L3 order on a side — Event distinguishes add/modify/delete updates.
*/
type Level struct {
	OrderID    string           `json:"order_id"`
	LimitPrice *decimal.Decimal `json:"limit_price"`
	OrderQty   *decimal.Decimal `json:"order_qty"`
	Timestamp  time.Time        `json:"timestamp"`
	// Event is add|modify|delete on updates; omitted on snapshots.
	Event string `json:"event,omitempty"`
}

/*
Level3Data is per-symbol L3 depth with venue checksum broker.Book validates.
*/
type Level3Data struct {
	Symbol   string  `json:"symbol"`
	Checksum int     `json:"checksum"`
	Bids     []Level `json:"bids"`
	Asks     []Level `json:"asks"`
}

/*
Level3 is authenticated ws L3 — broker.Book and signal disruptors attach here,
not the legacy aggregated book channel.
*/
type Level3 struct {
	Channel  string       `json:"channel"`
	Type     string       `json:"type"`
	Data     []Level3Data `json:"data"`
	onChange func(market.MarketType)
}

/*
NewLevel3 prepares empty Data for the first snapshot on private subscribe.
*/
func NewLevel3() *Level3 {
	return &Level3{
		Channel: "level3",
		Type:    "update",
		Data:    make([]Level3Data, 0),
	}
}

/*
DecodeLevel3 is a one-shot decode for tests and collectors outside venue.Read demux.
*/
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

/*
Request is the public subscribe stub; private L3 uses NewLevel3Subscription with token.
*/
func (level3 *Level3) Request() map[string]any {
	return map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "level3",
			"snapshot": true,
		},
	}
}

/*
OnChange wires broker.Book after each L3 decode and checksum fold.
*/
func (level3 *Level3) OnChange(fn func(market.MarketType)) {
	level3.onChange = fn
}

/*
MarshalJSON aliases Level3 so onChange stays off the wire.
*/
func (level3 *Level3) MarshalJSON() ([]byte, error) {
	type alias Level3
	return sonic.Marshal((*alias)(level3))
}

/*
UnmarshalJSON preserves onChange and notifies Book after Data is allocated.
*/
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

/*
ToMarket projects L3 rows into nomagique Level3 for broker.Book and checksum state.
*/
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

/*
FromMarket copies nomagique L3 back for cache replay without re-subscribing.
*/
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

/*
Level3Subscription is the authenticated subscribe/unsubscribe envelope including token.
*/
type Level3Subscription struct {
	Method string                   `json:"method"`
	Params Level3SubscriptionParams `json:"params"`
}

/*
Level3SubscriptionParams carries symbols, depth, snapshot flag, and ws token.
*/
type Level3SubscriptionParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth,omitempty"`
	Snapshot bool     `json:"snapshot,omitempty"`
	Token    string   `json:"token,omitempty"`
}

/*
NewLevel3Subscription builds subscribe with snapshot so Book checksum starts valid.
*/
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

/*
NewLevel3Unsubscription drops symbols without cancelling the shared decoder instance.
*/
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
