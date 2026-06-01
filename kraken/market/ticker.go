package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

/*
TickerParams is the Kraken WebSocket v2 subscribe payload for the ticker channel.
*/
type TickerParams struct {
	Channel      string   `json:"channel"`
	Symbol       []string `json:"symbol"`
	Snapshot     bool     `json:"snapshot"`
	EventTrigger string   `json:"event_trigger,omitempty"`
}

/*
TickerUpdate is one top-of-book and 24h summary row from the public ticker feed.

A live rolling 24-hour summary per symbol, pushed on change: best bid and ask with
their sizes, last price, session high and low, volume, VWAP, and the absolute and
percent change. It is the lowest-latency at-a-glance state of a market, already
reduced to the day's move and activity without computing it from the tape.
*/
type TickerUpdate struct {
	Symbol    string  `json:"symbol"`
	Ask       float64 `json:"ask"`
	AskQty    float64 `json:"ask_qty"`
	Bid       float64 `json:"bid"`
	BidQty    float64 `json:"bid_qty"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`
	High      float64 `json:"high"`
	Last      float64 `json:"last"`
	Low       float64 `json:"low"`
	Volume    float64 `json:"volume"`
	VWAP      float64 `json:"vwap"`
	Timestamp string  `json:"timestamp"`
}

/*
NewTickerSubscription returns a channel of ticker rows for symbols.
*/
func NewTickerSubscription(
	ctx context.Context, symbols ...string,
) <-chan *TickerUpdate {
	out := make(chan *TickerUpdate, 128)

	client := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	if err := client.Send(public.TickerChannel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: TickerParams{
			Channel:  public.TickerChannel,
			Symbol:   symbols,
			Snapshot: true,
		},
	}); err != nil {
		errnie.Error(err)
		return closed[TickerUpdate]()
	}

	for msg := range errnie.Does(func() (<-chan *public.SocketMessage, error) {
		stream, err := client.Stream(public.TickerChannel)

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

		var ticker TickerUpdate

		if err := sonic.Unmarshal(msg.Data, &ticker); err != nil {
			errnie.Error(err)
			continue
		}

		out <- &ticker
	}

	return out
}
