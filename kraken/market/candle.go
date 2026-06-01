package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	ctx context.Context, pool *qpool.Q, intervalMinutes int, symbols ...string,
) <-chan *CandleUpdate {
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	feed := OpenFeed(ctx, pool, public.CandlesChannel, CandleParams{
		Channel:  public.CandlesChannel,
		Symbol:   symbols,
		Interval: intervalMinutes,
		Snapshot: true,
	})

	out := make(chan *CandleUpdate, 128)

	go func() {
		defer close(out)

		for msg := range feed.Stream {
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
