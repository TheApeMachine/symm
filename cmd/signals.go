package cmd

import (
	"context"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/ensemble"
	"github.com/theapemachine/symm/types"
)

/*
productionSignals builds the full production signal set used by root boot. It
delegates to signal/ensemble so tests and boot share one ensemble definition.
*/
func productionSignals(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return ensemble.Production(ctx, api, instrument, channel)
}
