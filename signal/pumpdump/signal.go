package pumpdump

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the volume-clock activity measuring instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's
TypeID to whichever entity that data stream feeds.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status

	ticker *Ticker
	trade  *Trade
	level3 *Level3
}

/*
NewSignal composes the Ticker (executable spread), Trade (volume clock), and
Level3 (authoritative book touch) entities. quote supplies Trade with the
contemporaneous executable touch used for completed-bar midpoint response.
*/
func NewSignal(
	ctx context.Context,
	quote func(symbol string) (bid, ask *decimal.Decimal),
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	trade := NewTrade()
	trade.SetQuote(quote)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		ticker: NewTicker(),
		trade:  trade,
		level3: NewLevel3(),
	}
}

func (signal *Signal) Name() string { return "pumpdump" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	switch envelope.TypeID {
	case types.EnvelopeTicker:
		envelope.PumpDump = signal.StepTicker(envelope.TickerData)
	case types.EnvelopeTrade:
		envelope.PumpDump = signal.StepTrade(envelope.TradeData)
	case types.EnvelopeLevel3:
		if envelope.Level3Data.Bids == nil && envelope.Level3Data.Asks == nil {
			return envelope
		}

		envelope.PumpDump = signal.StepLevel3(envelope.Level3Data)
	}

	return envelope
}

func (signal *Signal) StepTicker(ticker kraken.TickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) StepTrade(trade kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) StepLevel3(message kraken.Level3Data) *data.Measurement[float64] {
	return signal.level3.Step(message)
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
