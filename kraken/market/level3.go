package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/public"
)

type Level3Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

type Level3OrderEvent struct {
	Event      string  `json:"event"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

type Level3Update struct {
	Symbol    string             `json:"symbol"`
	Bids      []Level3OrderEvent `json:"bids"`
	Asks      []Level3OrderEvent `json:"asks"`
	Checksum  int64              `json:"checksum"`
	Timestamp string             `json:"timestamp"`
}

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
		return ws.Generate(core.ChannelLevel3)
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
