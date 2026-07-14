package liquidity

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/utils"
)

/*
Ticker ingests public ticker rows into the shared liquidity sample.
*/
type Ticker struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []kraken.TickerData
}

func NewTicker(ctx context.Context, api *websocket.API) *Ticker {
	ctx, cancel := context.WithCancel(ctx)

	ticker := &Ticker{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []kraken.TickerData{},
	}

	ticker.api.On("ticker", ticker.On)
	return ticker
}

/*
On decodes an inbound market-data message and retains its relevant rows so
measurement uses the authoritative event stream.
*/
func (ticker *Ticker) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := utils.Unmarshal[kraken.Ticker](data)

	if len(frame.Data) == 0 {
		return
	}

	ticker.cache = append(ticker.cache, frame.Data...)
}
