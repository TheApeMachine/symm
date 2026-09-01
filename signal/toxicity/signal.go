package toxicity

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the book-touch liquidity-disposition instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's TypeID:
a Level3 envelope updates the retained per-symbol touch and projects the
book-touch disposition measurement; a Trade envelope matches against that
symbol's last retained touch to attribute fill.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status

	level3 *Level3
	trade  *Trade
}

/*
NewSignal composes the Level3 (book-touch) and Trade (executed-flow) entities.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		level3: NewLevel3(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "toxicity" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	switch envelope.TypeID {
	case types.EnvelopeLevel3:
		envelope.Toxicity = signal.StepLevel3(envelope.Level3Data)
	case types.EnvelopeTrade:
		envelope.Toxicity = signal.StepTrade(envelope.TradeData)
	}

	return envelope
}

func (signal *Signal) StepLevel3(message kraken.Level3Data) *data.Measurement[float64] {
	measurement := signal.level3.Step(message)

	if measurement != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = map[string]string{}
		}

		// README §11.1: preserve the attribution source. This entity observes
		// the book touch only, so the attribution is always touch-only
		// bracketing; full-book previous-level observation would be recorded
		// here when a full-book feed supplies Q_1(P_0).
		measurement.Provenance["previous_level_disposition"] = "touch_only_bracketing"
	}

	return measurement
}

func (signal *Signal) StepTrade(tick kraken.TradeData) *data.Measurement[float64] {
	committed, found := signal.level3.number.Project(tick.Symbol)

	if !found {
		return nil
	}

	bidPrice, bidFound := committed.Get(symbolPrevBid)
	askPrice, askFound := committed.Get(symbolPrevAsk)
	bidQty, bidQtyFound := committed.Get(symbolPrevBidQty)
	askQty, askQtyFound := committed.Get(symbolPrevAskQty)

	if !bidFound || !askFound || !bidQtyFound || !askQtyFound {
		return nil
	}

	return signal.trade.Step(tick, bidPrice, askPrice, bidQty, askQty)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.level3.Close(); err != nil {
		return err
	}

	return signal.trade.Close()
}
