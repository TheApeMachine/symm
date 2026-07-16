package cvd

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Trade ingests public trade rows into the shared CVD sample.
*/
type Trade struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  *types.MarketFeed[kraken.TradeData]
}

func NewTrade(ctx context.Context, api *websocket.API) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	trade := &Trade{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache: types.NewMarketFeed[kraken.TradeData](
			viper.GetInt("signals.feed_timeline_capacity"),
			viper.GetInt("signals.feed_track_capacity"),
		),
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

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, data := range frame.Data {
		if err := trade.cache.Observe(data.Symbol, data.Timestamp, data); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"cvd: trade observation failed",
				err,
			))
			return
		}
	}
}
