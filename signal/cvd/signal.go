package cvd

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the CVD executed-flow measuring instrument. It composes its market
entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	trade *Trade
}

/*
NewSignal composes the Trade (executed-flow) entity. The workspace is optional:
when supplied it provides the shared book for the response-price metrics; when
omitted the executed-flow accounting still stands and the response-price
metrics are simply undefined. The variadic form keeps the zero-workspace call
site valid.
*/
func NewSignal(ctx context.Context, workspace ...*runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	var shared *runtime.Workspace

	if len(workspace) > 0 {
		shared = workspace[0]
	}

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		trade:  NewTrade(shared),
	}
}

func (signal *Signal) Name() string { return "cvd" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(trade kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}
