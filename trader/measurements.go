package trader

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
)

/*
idleRest parks the drain loop between passes while no symbol has pending rows.
*/
const idleRest = 10 * time.Millisecond

type Measurements struct {
	ctx     context.Context
	cancel  context.CancelFunc
	signals []types.Signal
	ui      chan []byte

	// ObserveModule is an optional diagnostics hook reported for each signal's
	// total measurement time so the wiring diagram can profile the signal chain.
	ObserveModule func(string, time.Duration)
	// ObserveHop is an optional diagnostics hook reported at the boundary where
	// the measurement chain hands off to the analyzer.
	ObserveHop func(string, string, time.Duration)
	// ObservePassStart / ObservePassEnd / ObserveIdleCheck report the pass
	// lifecycle so the diagram can distinguish a gated-idle measurement engine
	// from one that is blocked inside a signal or analyzer pass.
	ObservePassStart func(time.Time)
	ObservePassEnd   func(time.Time, time.Duration)
	ObserveIdleCheck func(time.Time)
}

func NewMeasurements(
	ctx context.Context,
	thesis *types.Thesis,
	ui chan []byte,
) *Measurements {
	ctx, cancel := context.WithCancel(ctx)

	signals := []types.Signal{
		correlation.NewSignal(ctx, thesis),
		cvd.NewSignal(ctx, thesis),
		depthflow.NewSignal(ctx, thesis),
		exhaust.NewSignal(ctx, thesis),
		hawkes.NewSignal(ctx, thesis),
		leadlag.NewSignal(ctx, thesis),
		liquidity.NewSignal(ctx, thesis),
		pumpdump.NewSignal(ctx, thesis),
		sentiment.NewSignal(ctx, thesis),
		toxicity.NewSignal(ctx, thesis),
	}

	return &Measurements{
		ctx:     ctx,
		cancel:  cancel,
		ui:      ui,
		signals: signals,
	}
}

/*
Generate runs the streaming map/reduce pipeline. Ingress only appends market
rows to the per-symbol queues; this loop is their sole consumer. One pass
drains every symbol that still has pending rows — signals never see a clean
symbol, so always-yielding signals cannot drive the loop — fans the dirty
symbols out across goroutines (map), joins them (fan-in), then runs the
analyzer on the joined thesis (reduce) and emits it. A quiet market rests
the loop instead of spinning the solver stack.
*/
func (measurements *Measurements) Generate(
	thesis *types.Thesis,
	analyzer *logic.Analyzer,
) chan *types.Thesis {
	theses := make(chan *types.Thesis)

	go func() {
		defer close(theses)

		for {
			select {
			case <-measurements.ctx.Done():
				return
			default:
			}

			if !thesis.AnyPending() {
				if measurements.ObserveIdleCheck != nil {
					measurements.ObserveIdleCheck(time.Now())
				}

				time.Sleep(idleRest)
				continue
			}

			thesis.Tick++
			passStarted := time.Now()

			if measurements.ObservePassStart != nil {
				measurements.ObservePassStart(passStarted)
			}

			thesis.Symbols.Range(func(_, value any) bool {
				symbol, ok := value.(*types.Symbol)

				if !ok || symbol == nil || !symbol.Pending() {
					return true
				}

				symbol.Tick = thesis.Tick

				return true
			})

			thesis.At = time.Now()

			if measurements.ObserveHop != nil {
				measurements.ObserveHop("measurements", "category", time.Since(passStarted))
			}

			if analyzer != nil {
				errnie.Error(analyzer.Process(thesis))
			}

			if measurements.ObservePassEnd != nil {
				measurements.ObservePassEnd(time.Now(), time.Since(passStarted))
			}

			select {
			case theses <- thesis:
			case <-measurements.ctx.Done():
				return
			}
		}
	}()

	return theses
}

/*
Close releases every signal conditioner owned by the measurement stage.
*/
func (measurements *Measurements) Close() error {
	if measurements == nil {
		return nil
	}

	measurements.cancel()

	for _, signal := range measurements.signals {
		if signal == nil {
			continue
		}

		if err := signal.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"trader: failed to close signal",
				err,
			))
		}
	}

	return nil
}
