package market

import (
	"context"
	"time"

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
NewTradeSubscription returns a channel of executed trades for symbols. All
callers share one upstream connection via the trade feed, so the connection count
stays flat no matter how many signals subscribe. The caller's ctx detaches it
from the shared feed; the upstream keeps running for the others.
*/
func NewTradeSubscription(
	ctx context.Context, symbols ...string,
) <-chan *TradeUpdate {
	return tradeFeed.subscribe(ctx, func() <-chan *TradeUpdate {
		return dialTrades(context.Background(), symbols)
	})
}

// dialTrades opens one upstream connection to the public trade channel for
// symbols. It is the shared feed's reopen function, not called per consumer.
func dialTrades(
	ctx context.Context, symbols []string,
) <-chan *TradeUpdate {
	ws, err := public.NewWebSocket(ctx)

	if err != nil {
		errnie.Error(err)
		return closed[TradeUpdate]()
	}

	if err := ws.Connect(public.WebSocketURL, public.TradesChannel); err != nil {
		errnie.Error(err)
		return closed[TradeUpdate]()
	}

	if err := ws.Send(public.TradesChannel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: TradeParams{
			Channel:  public.TradesChannel,
			Symbol:   symbols,
			Snapshot: true,
		},
	}); err != nil {
		errnie.Error(err)
		return closed[TradeUpdate]()
	}

	stream, err := public.Stream[TradeUpdate](ws, public.TradesChannel)

	if err != nil {
		errnie.Error(err)
		return closed[TradeUpdate]()
	}

	return stream
}
