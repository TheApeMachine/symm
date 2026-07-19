package cmd

import (
	"context"

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
productionSignals builds the full production signal set used by root boot.
*/
func productionSignals(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	hawkesSignal := hawkes.NewSignal(ctx, api, channel)

	return []types.Signal{
		pumpdump.NewSignal(ctx, api, channel),
		liquidity.NewSignal(ctx, api, channel),
		toxicity.NewSignal(ctx, api, channel),
		leadlag.NewSignal(ctx, api, channel),
		cvd.NewSignal(ctx, api, channel),
		correlation.NewSignal(ctx, api, channel),
		exhaust.NewSignal(ctx, api, instrument, channel),
		sentiment.NewSignal(ctx, api, channel),
		depthflow.NewSignal(ctx, api, instrument, channel),
		fluid.NewSignal(ctx, api, instrument, channel),
		hawkesSignal,
	}
}
