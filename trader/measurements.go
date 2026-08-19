package trader

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/nomagique"
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
	"golang.org/x/sync/errgroup"
)

/*
idleRest parks the drain loop between passes while no symbol has pending rows.
*/
const idleRest = 10 * time.Millisecond

type Measurements struct {
	ctx     context.Context
	cancel  context.CancelFunc
	signals []types.Signal
	streams map[string]*nomagique.KeyedStreams[string]
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
	api *websocket.API,
	instrument *broker.Instrument,
	ui chan []byte,
) *Measurements {
	ctx, cancel := context.WithCancel(ctx)

	signals := []types.Signal{
		correlation.NewSignal(ctx, api),
		cvd.NewSignal(ctx, api),
		depthflow.NewSignal(ctx, api, instrument),
		exhaust.NewSignal(ctx, api, instrument),
		hawkes.NewSignal(ctx, api),
		leadlag.NewSignal(ctx, api),
		liquidity.NewSignal(ctx, api),
		pumpdump.NewSignal(ctx, api),
		sentiment.NewSignal(ctx, api),
		toxicity.NewSignal(ctx, api),
	}

	streams := make(map[string]*nomagique.KeyedStreams[string], len(signals))

	for _, signal := range signals {
		streams[signal.Name()] = nomagique.NewKeyedStreams[string](signal.Pipeline(), nil)
	}

	return &Measurements{
		ctx:     ctx,
		cancel:  cancel,
		ui:      ui,
		signals: signals,
		streams: streams,
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

			group, ctx := errgroup.WithContext(measurements.ctx)

			thesis.Symbols.Range(func(_, value any) bool {
				symbol, ok := value.(*types.Symbol)

				if !ok || symbol == nil || !symbol.Pending() {
					return true
				}

				symbol.Tick = thesis.Tick

				group.Go(func() error {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}

					for _, signal := range measurements.signals {
						signalStarted := time.Now()

						if err := measurements.runSignal(signal, symbol); err != nil {
							errnie.Error(errnie.Err(
								errnie.Validation,
								"trader: signal failed for "+signal.Name(),
								err,
							))
						}

						if measurements.ObserveModule != nil {
							measurements.ObserveModule(signal.Name(), time.Since(signalStarted))
						}
					}

					return nil
				})

				return true
			})

			if err := group.Wait(); err != nil {
				errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("trader: failed to measure signals - [%s]", err.Error()),
					err,
				))
			}

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
runSignal drives one symbol's raw rows through the signal's composed nomagique
pipeline and appends the resulting measurement. The runner owns the per-symbol
stream and the drain; the signal contributes only its pipeline and the three
boundary adapters (Rows, Encode, Emit).
*/
func (measurements *Measurements) runSignal(
	signal types.Signal,
	symbol *types.Symbol,
) error {
	if symbol == nil {
		return nil
	}

	stream := measurements.streams[signal.Name()]

	if stream == nil {
		return nil
	}

	for row := range signal.Rows(symbol) {
		input, ok := signal.Encode(row)

		if !ok {
			continue
		}

		output, err := stream.Step(symbol.Symbol, input)

		if err != nil {
			continue
		}

		measurement, ok := signal.Emit(symbol.Symbol, output)

		if !ok {
			continue
		}

		if err := symbol.AppendMeasurement(measurement); err != nil {
			return err
		}
	}

	return nil
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
