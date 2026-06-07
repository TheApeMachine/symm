package private

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	krakenpaper "github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/market/settings"
)

/*
Runtime is one engine-registered private execution or data socket.
*/
type Runtime interface {
	Tick() error
	Close() error
}

/*
ExecutionSystems returns the private-side runtimes for engine registration.
Paper mode registers paper execution and, when configured, a separate L3 feed.
Live mode registers one authenticated private websocket.
*/
func ExecutionSystems(
	ctx context.Context,
	pool *qpool.Q[any],
	apiKey string,
	apiSecret string,
	quotes *broker.QuoteCache,
) []Runtime {
	if viper.GetViper().GetString("trading.model") != "paper" {
		errnie.Info("kraken/private live websocket", "kraken/private live websocket")

		return []Runtime{
			NewWebSocketWithQuoteCache(ctx, pool, apiKey, apiSecret, quotes),
		}
	}

	errnie.Info("kraken/private paper websocket", "kraken/private paper websocket")

	systems := []Runtime{
		krakenpaper.NewWebSocketWithQuoteCache(ctx, pool, quotes),
	}

	if !settings.L3Enabled() {
		return systems
	}

	level3, err := NewLevel3WebSocket(ctx, pool, apiKey, apiSecret)

	if err != nil {
		errnie.Error(err, "kraken/private: L3 data socket unavailable, paper only")

		return systems
	}

	errnie.Info("kraken/private L3 data websocket", "kraken/private L3 data websocket")

	return append(systems, level3)
}
