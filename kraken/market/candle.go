package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
NewCandleSubscription opens the ohlc channel at intervalMinutes and forwards rows.
*/
func NewCandleSubscription(
	ctx context.Context, intervalMinutes int, symbols ...string,
) <-chan *CandleUpdate {
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	out := make(chan *CandleUpdate, 128)

	go func() {
		defer close(out)

		client := errnie.Does(func() (*kraken.Client, error) {
			return kraken.NewClient(ctx)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		if err := client.Send(public.CandlesChannel, public.Subscription{
			Method: public.MethodSubscribe,
			Params: CandleParams{
				Channel:  public.CandlesChannel,
				Symbol:   symbols,
				Interval: intervalMinutes,
				Snapshot: true,
			},
		}); err != nil {
			errnie.Error(err)

			return
		}

		for msg := range errnie.Does(func() (<-chan *public.SocketMessage, error) {
			stream, err := client.Stream(public.CandlesChannel)

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

			var candle CandleUpdate

			if err := sonic.Unmarshal(msg.Data, &candle); err != nil {
				errnie.Error(err)
				continue
			}

			select {
			case <-ctx.Done():
				return
			case out <- &candle:
			}
		}
	}()

	return out
}
