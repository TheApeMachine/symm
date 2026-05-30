package market

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
CandleParams is the Kraken WebSocket v2 subscribe payload for the ohlc channel.
*/
type CandleParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Interval int      `json:"interval"`
	Snapshot bool     `json:"snapshot"`
}

/*
CandleUpdate is one forming or closed OHLC bar from the public ohlc feed.

A forming or closed OHLC bar streamed as it updates: open, high, low, close,
VWAP, volume, and trade count for the interval. It is price action already
aggregated to a chosen horizon and kept current live -- VWAP gives the interval's
fair transacted price and the trade count its participation.
*/
type CandleUpdate struct {
	Symbol        string  `json:"symbol"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	VWAP          float64 `json:"vwap"`
	Trades        float64 `json:"trades"`
	Volume        float64 `json:"volume"`
	IntervalBegin string  `json:"interval_begin"`
	Interval      int     `json:"interval"`
}

/*
NewCandleSubscription opens the ohlc channel at intervalMinutes and forwards rows to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewCandleSubscription(
	ctx context.Context, intervalMinutes int, symbols ...string,
) <-chan *CandleUpdate {
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	ws, err := public.NewWebSocket(ctx)

	if err != nil {
		errnie.Error(err)
		return closed[CandleUpdate]()
	}

	if err := ws.Connect(public.WebSocketURL, public.CandlesChannel); err != nil {
		errnie.Error(err)
		return closed[CandleUpdate]()
	}

	if err := ws.Send(public.CandlesChannel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: CandleParams{
			Channel:  public.CandlesChannel,
			Symbol:   symbols,
			Interval: intervalMinutes,
			Snapshot: true,
		},
	}); err != nil {
		errnie.Error(err)
		return closed[CandleUpdate]()
	}

	stream, err := public.Stream[CandleUpdate](ws, public.CandlesChannel)

	if err != nil {
		errnie.Error(err)
		return closed[CandleUpdate]()
	}

	return stream
}
