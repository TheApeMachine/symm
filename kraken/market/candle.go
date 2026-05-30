package market

import (
	"context"

	"github.com/bytedance/sonic"
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
	ctx context.Context,
	recv chan *CandleUpdate,
	intervalMinutes int,
	symbols ...string,
) {
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.CandlesChannel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var rows []CandleUpdate

		if err := sonic.Unmarshal(message.Data, &rows); err != nil {
			continue
		}

		for _, row := range rows {
			update := row
			recv <- &update
		}
	}
}
