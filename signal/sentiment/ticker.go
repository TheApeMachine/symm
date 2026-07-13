package sentiment

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/utils"
)

/*
Ticker ingests public ticker rows into the shared sentiment sample.
*/
type Ticker struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []kraken.TickerData
}

/*
NewTicker binds one shared sample and measurement stage for ticker rows.
*/
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
