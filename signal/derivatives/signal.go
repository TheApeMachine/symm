package derivatives

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the derivatives measuring instrument. It composes its market
entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's
TypeID to whichever entity that futures data stream feeds — mirrors
signal/pumpdump's dispatch shape, but over the futures ticker/trade
envelope kinds rather than spot ones.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status

	ticker *Ticker
	trade  *Trade
}

/*
NewSignal composes the Ticker (derivative/reference basis and open interest)
and Trade (liquidation accounting) entities.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		ticker: NewTicker(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "derivatives" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	switch envelope.TypeID {
	case types.EnvelopeFuturesTicker:
		envelope.Derivatives = signal.StepTicker(envelope.FuturesTickerData)
	case types.EnvelopeFuturesTrade:
		envelope.Derivatives = signal.StepTrade(envelope.FuturesTradeData)
	}

	return envelope
}

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
