package market

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

// OrderSnapshot is the envelope type tag for a full L3 book frame after subscribe.
const OrderSnapshot = "snapshot"

/*
OrderTokenSource supplies short-lived authenticated WebSocket tokens for level3.
*/
type OrderTokenSource interface {
	Token(context.Context) (string, error)
}

/*
OrderParams is the Kraken WebSocket v2 subscribe payload for the level3 channel.
*/
type OrderParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

/*
OrderEvent is one resting order or order event on the L3 book feed.

Snapshots list orders with order_id, limit_price, order_qty, and timestamp.
Updates add an event field: add, modify, or delete.
*/
type OrderEvent struct {
	Event      string  `json:"event,omitempty"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

/*
Order is one L3 order book snapshot or update from the authenticated level3 feed.

Each frame carries per-order bids and asks, a CRC32 checksum over the top ten
price levels per side, and an RFC3339 timestamp. Type records the envelope tag
(snapshot vs update) from the channel message, not the data payload.
*/
type Order struct {
	Symbol    string       `json:"symbol"`
	Bids      []OrderEvent `json:"bids"`
	Asks      []OrderEvent `json:"asks"`
	Checksum  int64        `json:"checksum"`
	Timestamp string       `json:"timestamp"`
	Type      string       `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag (snapshot or update).
*/
func (order *Order) SetEnvelopeType(kind string) {
	order.Type = kind
}

/*
IsSnapshot reports whether this frame is a full L3 book snapshot.
*/
func (order *Order) IsSnapshot() bool {
	return order.Type == OrderSnapshot
}

var (
	orderTokenSourceMu sync.RWMutex
	orderTokenSource   OrderTokenSource
)

/*
SetOrderTokenSource enables authenticated L3 market data. Pass nil to disable.
*/
func SetOrderTokenSource(source OrderTokenSource) {
	orderTokenSourceMu.Lock()
	defer orderTokenSourceMu.Unlock()

	orderTokenSource = source
}

/*
OrderAvailable reports whether authenticated L3 market data is configured.
*/
func OrderAvailable() bool {
	orderTokenSourceMu.RLock()
	defer orderTokenSourceMu.RUnlock()

	return orderTokenSource != nil
}

/*
NewOrderSubscription returns a channel of L3 book snapshots and updates for symbols
at depth when a token source is configured.
*/
func NewOrderSubscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *Order {
	orderTokenSourceMu.RLock()
	source := orderTokenSource
	orderTokenSourceMu.RUnlock()

	if source == nil {
		return nil
	}

	if depth <= 0 {
		depth = 10
	}

	token, err := source.Token(ctx)

	if err != nil {
		errnie.Error(err)

		return nil
	}

	out := make(chan *Order, 128)

	go func() {
		defer close(out)

		client := errnie.Does(func() (*kraken.Client, error) {
			return kraken.NewClient(ctx)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		if err := client.Send(public.Level3Channel, public.Subscription{
			Method: public.MethodSubscribe,
			Params: OrderParams{
				Channel:  public.Level3Channel,
				Symbol:   symbols,
				Depth:    depth,
				Snapshot: true,
				Token:    token,
			},
		}); err != nil {
			errnie.Error(err)

			return
		}

		for msg := range errnie.Does(func() (<-chan *public.SocketMessage, error) {
			stream, err := client.Stream(public.Level3Channel)

			if err != nil {
				return nil, err
			}

			return stream, nil
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value() {
			if msg == nil {
				continue
			}

			var order Order

			if err := sonic.Unmarshal(msg.Data, &order); err != nil {
				errnie.Error(err)
				continue
			}

			order.SetEnvelopeType(msg.Type)
			out <- &order
		}
	}()

	return out
}

/*
OrderEventTime parses an L3 event timestamp, falling back to now when absent.
*/
func OrderEventTime(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, raw)

		if err == nil {
			return parsed
		}
	}

	return fallback
}

// Level3 names alias the level3 WebSocket types for existing callers.

type (
	Level3TokenSource = OrderTokenSource
	Level3Params      = OrderParams
	Level3OrderEvent  = OrderEvent
	Level3Update      = Order
)

func SetLevel3TokenSource(source Level3TokenSource) {
	SetOrderTokenSource(source)
}

func Level3Available() bool {
	return OrderAvailable()
}

func NewLevel3Subscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *Level3Update {
	return NewOrderSubscription(ctx, depth, symbols...)
}

func Level3EventTime(raw string, fallback time.Time) time.Time {
	return OrderEventTime(raw, fallback)
}
