package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Level3Params is the Kraken WebSocket v2 subscribe payload for the level3 channel.
*/
type Level3Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

/*
Level3OrderEvent is one add, modify, or delete event for a resting order on the L3 feed.
*/
type Level3OrderEvent struct {
	Event      string  `json:"event"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

/*
Level3Update is one per-order book delta from the authenticated level3 WebSocket feed.

The order-by-order book: individual add, modify, and delete events for each
resting order with its ID, price, size, and time, as a snapshot plus
checksum-verified deltas. It is the most granular view the exchange offers --
preserving each order exposes queue position and the full life cycle of orders
that aggregated L2 levels collapse away.
*/
type Level3Update struct {
	Symbol    string             `json:"symbol"`
	Bids      []Level3OrderEvent `json:"bids"`
	Asks      []Level3OrderEvent `json:"asks"`
	Checksum  int64              `json:"checksum"`
	Timestamp string             `json:"timestamp"`
}

/*
NewLevel3Subscription opens the level3 channel with token at depth and forwards rows to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewLevel3Subscription(
	ctx context.Context,
	recv chan *Level3Update,
	token string,
	depth int,
	symbols ...string,
) {
	if depth <= 0 {
		depth = 10
	}

	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.Level3Channel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var rows []Level3Update

		if err := sonic.Unmarshal(message.Data, &rows); err != nil {
			continue
		}

		for _, row := range rows {
			update := row
			recv <- &update
		}
	}
}
