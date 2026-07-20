/*
Package ensemble is the single source of truth for the production signal set.
Both root boot and the market-simulation tests build their signals here so a test
can never validate a friendlier subset than the system that actually runs.
*/
package ensemble

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
)

/*
Production builds the full production signal set once the instrument exists. Its
signature is assignable to stack.SignalFactory so root boot and the simulation
harness share one definition of "the ensemble".
*/
func Production(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{
		pumpdump.NewSignal(
			ctx,
			api,
			channel,
			viper.GetInt("signals.feed_track_capacity"),
		),
		liquidity.NewSignal(ctx, api, channel),
		toxicity.NewSignal(ctx, api, channel),
		leadlag.NewSignal(ctx, api, channel),
		cvd.NewSignal(ctx, api, channel),
		correlation.NewSignal(ctx, api, channel),
		exhaust.NewSignal(ctx, api, instrument, channel),
		sentiment.NewSignal(ctx, api, channel),
		depthflow.NewSignal(ctx, api, instrument, channel),
		fluid.NewSignal(ctx, api, instrument, channel),
		hawkes.NewSignal(ctx, api, channel),
	}
}
