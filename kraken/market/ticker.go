package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
NewTickerSubscription opens the ticker channel and forwards unmarshaled rows to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewTickerSubscription(
	ctx context.Context,
	recv chan *TickerUpdate,
	symbols ...string,
) {
	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.TickerChannel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var rows []TickerUpdate

		if err := sonic.Unmarshal(message.Data, &rows); err != nil {
			continue
		}

		for _, row := range rows {
			update := row
			recv <- &update
		}
	}
}
