package hawkes

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Measurement distinguishes a Hawkes reading from every other signal's
*data.Measurement[float64] by Go type alone: the manifold solver is fired
exclusively by Hawkes (it is the fluid field's forcing term), so it must be
able to want a type only Hawkes produces, distinct from the merged
*data.Measurement[float64] stream every other consumer (category, graph,
planner, the UI taps) wants.
*/
type Measurement struct {
	*data.Measurement[float64]
}

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

func (signal *Signal) Step(trade kraken.TradeData) *Measurement {
	measurement := signal.trade.Step(trade)

	if measurement == nil {
		return nil
	}

	return &Measurement{measurement}
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

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}
