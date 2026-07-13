package cvd

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Trade ingests public trade rows into the shared CVD sample.
*/
type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []kraken.TradeData
}

func NewTrade(ctx context.Context, api *websocket.API) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []kraken.TradeData{},
	}

	trade.api.On("trade", trade.On)
	return trade
}

func (trade *Trade) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	trade.cache = append(trade.cache, frame.Data...)
}
