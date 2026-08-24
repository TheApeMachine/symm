package toxicity

import (
	"context"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the book-touch liquidity-disposition instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	level3 *Level3
	trade  *Trade
}

/*
NewSignal composes the Level3 (book-touch) and Trade (executed-flow) entities,
which read the shared book from the workspace pool.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		level3: NewLevel3(workspace),
		trade:  NewTrade(workspace),
	}
}

func (signal *Signal) Name() string { return "toxicity" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(symbol string, at time.Time) *data.Measurement[float64] {
	measurement := signal.level3.Step(symbol, at)

	if measurement != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = map[string]string{}
		}

		// README §11.1: preserve the attribution source. This entity observes
		// the shared book touch only, so the attribution is always
		// touch-only bracketing; full-book previous-level observation would be
		// recorded here when a full-book feed supplies Q_1(P_0).
		measurement.Provenance["previous_level_disposition"] = "touch_only_bracketing"
	}

	return measurement
}

func (signal *Signal) StepTrade(tick kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(tick)
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
