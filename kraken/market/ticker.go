package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/public"
)

type TickerParams struct {
	Channel      string   `json:"channel"`
	Symbol       []string `json:"symbol"`
	Snapshot     bool     `json:"snapshot"`
	EventTrigger string   `json:"event_trigger,omitempty"`
}

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
		return ws.Generate(core.ChannelTicker)
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
