package sentiment

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Ticker ingests public ticker rows into the shared sentiment sample.
*/
type Ticker struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  *types.MarketFeed[kraken.TickerData]
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
		cache: types.NewMarketFeed[kraken.TickerData](
			viper.GetInt("signals.feed_timeline_capacity"),
			viper.GetInt("signals.feed_track_capacity"),
		),
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

	for _, data := range frame.Data {
		if err := ticker.cache.Observe(data.Symbol, data.Timestamp, data); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"sentiment: ticker observation failed",
				err,
			))
			return
		}
	}
}
