package hawkes

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Hawkes arrival-dynamics measuring instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	trade *Trade
}

// NewSignal composes the Trade (arrival-dynamics) entity.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "hawkes" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(trade kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

/*
Process adapts Step to the workspace unit-processing contract: it receives one
trade and emits its raw measurement (or nothing on error). The workspace routes
whatever it emits onward, in this case onto the raw Hawkes topic the manifold
solver consumes.
*/
func (signal *Signal) Process(
	trade kraken.TradeData,
) (*data.Measurement[float64], bool, error) {
	measurement := signal.trade.Step(trade)

	if measurement == nil || measurement.Err != nil {
		return nil, false, measurement.Err
	}

	return measurement, true, nil
}

/*
RegisterWire connects the Hawkes signal as a pure unit-processing node: trades
in, raw measurements out on the Hawkes topic, with the workspace owning the
routing. There is no publish call here — the returned measurement is published
by the workspace onto the downstream topic.
*/
func (signal *Signal) RegisterWire(workspace *runtime.Workspace) {
	if signal == nil || workspace == nil {
		return
	}

	runtime.WireNode[kraken.TradeData, *data.Measurement[float64]](
		workspace,
		types.ChannelTrades,
		HawkesTopic,
		signal,
	)
}

/*
HawkesTopic names the raw Hawkes measurement topic. It is the output of the
Hawkes node and the input of the manifold solver.
*/
const HawkesTopic = "hawkes"

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}
