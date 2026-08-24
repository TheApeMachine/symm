package pumpdump

import (
	"context"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the volume-clock activity measuring instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
	trade  *Trade
	level3 *Level3
}

/*
NewSignal composes the Ticker (executable spread), Trade (volume clock), and
Level3 (authoritative book touch) entities.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
		trade:  NewTrade(workspace),
		level3: NewLevel3(workspace),
	}
}

func (signal *Signal) Name() string { return "pumpdump" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) StepTicker(ticker kraken.TickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) StepTrade(trade kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) StepLevel3(symbol string, at time.Time) *data.Measurement[float64] {
	return signal.level3.Step(symbol, at)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.ticker.Close(); err != nil {
		return err
	}

	if err := signal.trade.Close(); err != nil {
		return err
	}

	return signal.level3.Close()
}
