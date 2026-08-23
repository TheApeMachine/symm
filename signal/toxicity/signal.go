package toxicity

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal: Toxicity (BookFlow) - The Quality Perspective

**What it measures exactly (in isolation)**

The **Toxicity signal** analyzes the "honesty" of the book by tracking how makers behave when a trade approaches.

  - **Cancel-to-Fill Asymmetry:** Measures the ratio of liquidity being "pulled" (cancelled) versus
    liquidity being "hit" (filled).
  - **Toxic Level Detection:** Flags large, young, near-touch blocks that disappear rather than
    fill—this is the signature of a bluff.
  - **Directional BookFlow:** Emits a directional read based on which side of the book is
    "retreating" (vacuum effect).

**Semantically, what story does it tell?**

  - **The "Bluffing" Story:** It exposes makers who are "fake-bidding" to create an illusion of support,
    warning the engine that a wall is not "real" and will crumble upon contact.
  - **The "Vacuum" Story:** It identifies a "liquidity vacuum" where one side pulls away so aggressively
    that the resulting void "sucks" the price in that direction.

**Probability Categories**

| Category             | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:---------------------|:------------------|:----------------|:-----------------------|
| **Liquidity Vacuum** | High Asymmetry    | One Side        | **Vacuum Surcharge**   |
| **Toxic Bluff**      | High              | Near-Touch      | **Manipulated / Fake** |
| **Hard Support**     | Low (High Fill)   | None            | **Robust / Sincere**   |
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
	pool   *types.SymbolPool
}

/*
NewSignal creates a new Signal instance for the Toxicity (BookFlow) analysis.
*/
func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](nomagique.Pipe(
			calculus.Difference,
			nomagique.Relay(
				calculus.SymbolResult,
				nomagique.SampleValue,
			),
			nomagique.Configure(
				statistic.Baseline,
				nmtypes.Span,
				temporal.Window,
			),
			nomagique.Fork(
				statistic.ZScore,
				nomagique.Fork(
					statistic.Deviation,
					nomagique.Pipe(
						nomagique.Relay(nomagique.SampleValue, calculus.SymbolValue),
						calculus.Positive,
						nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
						nomagique.Relay(statistic.SymbolBaselineValue, calculus.SymbolScale),
						calculus.Squash,
					),
				),
			),
		)),
		pool: types.NewSymbolPool(types.ShardWorkers()),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceToxicity).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceToxicity) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceToxicity }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			if err := signal.pool.Error(); err != nil {
				signal.err = err
			}

			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceToxicity).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.pool.CaptureError(signal.ctx.Err())
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbolName := symbol.Symbol

			signal.pool.Submit(symbolName, func() {
				if err := signal.consumeSymbol(symbol); err != nil {
					signal.pool.CaptureError(errnie.Error(errnie.Err(
						errnie.Validation,
						"toxicity: failed for "+symbolName,
						err,
					)))
				}
			})
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	for frame := range symbol.MarketLevel3(
		symbol.Level3Consumers[types.Level3ConsumerToxicity],
	) {
		var filled, retreated float64

		for _, orders := range [][]kraken.Level3Order{frame.Bids, frame.Asks} {
			for _, order := range orders {
				if order.OrderQty == nil {
					continue
				}

				switch order.Event {
				case "fill":
					filled += order.OrderQty.Float64()
				case "delete":
					retreated += order.OrderQty.Float64()
				}
			}
		}

		input := types.Frame{}
		input.Put(nmtypes.AlphaQuantity, filled)
		input.Put(nmtypes.BetaQuantity, retreated)
		input.Put(calculus.SymbolLeft, retreated)
		input.Put(calculus.SymbolRight, filled)
		input.Put(nomagique.SampleValue, retreated-filled)
		input.Put(calculus.SymbolValue, retreated-filled)
		input.Put(nmtypes.EventTimeSec, float64(frame.Timestamp.Unix()))
		input.Put(nmtypes.EventTimeNsec, float64(frame.Timestamp.Nanosecond()))

		output, err := signal.number.Step(symbol.Symbol, input)

		if err != nil {
			return err
		}

		measurement := nmtypes.NewMeasurement(
			uuid.NewString(),
			signal.Name(),
			frame.Timestamp.UnixNano(),
			frame.Timestamp.UnixNano(),
		).AddMetrics(
			nmtypes.NewMetric("honesty_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewMetric("honesty_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewMetric("toxicity_intensity", output.MustGet(calculus.SymbolResult), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
		)
		measurement.StampQuality(
			statistic.StandardSeparation(output.MustGet(statistic.SymbolZScore)),
			output.MustGet(nomagique.SampleCount),
		)

		symbol.AppendMeasurement(measurement)
	}

	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.Close()
	}

	return nil
}
