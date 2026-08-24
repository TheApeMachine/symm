package derivatives

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the derivatives measuring instrument. It composes its market
entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
	trade  *Trade
}

/*
NewSignal composes the Ticker (derivative/reference basis and open interest)
and Trade (liquidation accounting) entities.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "derivatives" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) StepTicker(ticker kraken.FuturesTickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) StepTrade(trade kraken.FuturesTradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.ticker.Close(); err != nil {
		return err
	}

	return signal.trade.Close()
}
