package toxicity

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/utils"
)

/*
Trade ingests public tape rows into the shared toxicity sample.
*/
type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []spot.Trade
}

/*
NewTrade binds one shared sample and measurement stage for trade rows.
*/
func NewTrade(ctx context.Context, api *websocket.API) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []spot.Trade{},
	}

	trade.api.On("trade", trade.On)
	return trade
}

/*
On decodes an inbound market-data message and retains its relevant rows so
measurement uses the authoritative event stream.
*/
func (trade *Trade) On(data []byte) {
	if len(data) == 0 {
		return
	}

	trades := utils.UnmarshalSlice[spot.Trade](data)

	if len(trades) == 0 {
		return
	}

	trade.cache = append(trade.cache, trades...)
}
