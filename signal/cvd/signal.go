package cvd

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
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
	status *runtime.Status
	trade  *Trade
}

/*
NewSignal composes the Trade (executed-flow) entity. quote, when non-nil,
supplies the contemporaneous top-of-book bid/ask so the response-price metrics
(midpoint and midpoint_log_return) can be computed; without it they remain
permanently undefined and only the executed-flow accounting is measured.
*/
func NewSignal(ctx context.Context, quote func(symbol string) (bid, ask *decimal.Decimal)) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	trade := NewTrade()
	trade.SetQuote(quote)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		trade:  trade,
	}
}

func (signal *Signal) Name() string { return "cvd" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeTrade {
		return envelope
	}

	envelope.CVD = signal.trade.Step(envelope.TradeData)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}
