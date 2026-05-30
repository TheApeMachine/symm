package market

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
TradeParams is the Kraken WebSocket v2 subscribe payload for the trade channel.
*/
type TradeParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Snapshot bool     `json:"snapshot"`
}

/*
TradeUpdate is one executed trade from the public trade WebSocket feed.

One executed trade from the public tape: price, size, aggressor side, order type,
ID, and time. It is the ground truth of what actually transacted rather than what
was merely quoted -- the aggressor side reveals which side lifted or hit, and the
stream is the finest-grained record of realized volume and price discovery as it
happens.
*/
type TradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	OrdType   string    `json:"ord_type"`
	TradeID   int64     `json:"trade_id"`
	Timestamp time.Time `json:"timestamp"`
}

/*
NewTradeSubscription opens the trade channel and forwards unmarshaled rows to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewTradeSubscription(
	ctx context.Context,
	recv chan *TradeUpdate,
	symbols ...string,
) {
	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.TradesChannel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var rows []TradeUpdate

		if err := sonic.Unmarshal(message.Data, &rows); err != nil {
			continue
		}

		for _, row := range rows {
			update := row
			recv <- &update
		}
	}
}
