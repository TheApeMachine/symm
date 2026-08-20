package pumpdump

import (
	"context"
	"runtime"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the PumpDump perspective: volume-clocked ignition conditioned per
symbol. It is ONLY a nomagique pipeline — each executed trade advances the
symbol's own volume clock and emits relative volume and per-side precursors,
with no cross-symbol aggregation and no injected venue dependency.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number nomagique.Number[string]
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](
			nomagique.Pipe(algo.Ignition()),
		),
	}

	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType {
	return types.SourcePumpDump
}

func (signal *Signal) Run() error {
	for signal.err == nil {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		default:
		}

		if !signal.pending() {
			// Nothing queued for this signal; yield before polling again.
			runtime.Gosched()
			continue
		}

		signal.thesis.Symbols.Range(func(_ any, value any) bool {
			symbol, valid := value.(*types.Symbol)

			if !valid || symbol == nil {
				return true
			}

			for trade := range symbol.MarketTrades(types.SourcePumpDump) {
				input := nomagique.Frame{}
				input.Put(algo.SymbolVolume, trade.Qty)
				input.Put(algo.SymbolLast, trade.Price.Float64())
				input.Put(algo.SymbolCapacity, nomagique.MaxSamples)
				input.Put(algo.SymbolUnixSec, float64(trade.Timestamp.Unix()))
				input.Put(algo.SymbolUnixNsec, float64(trade.Timestamp.Nanosecond()))

				// The touch prices are the print's book response; without a
				// ticker touch the ignition cannot enrich the print, so the
				// step is skipped rather than fed a fabricated bid/ask pair.
				if !touch(symbol, trade, &input) {
					continue
				}

				output, err := signal.number(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"pumpdump: number step failed for "+symbol.Symbol,
						err,
					))
					return false
				}

				symbol.AppendMeasurement(nmtypes.NewMeasurement(
					uuid.NewString(),
					signal.Name(),
					trade.Timestamp.UnixNano(),
					trade.Timestamp.UnixNano(),
				).AddMetrics(
					nmtypes.NewMetric("rvol", output.MustGet(algo.SymbolRVOL), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescalePerTick,
					}),
					nmtypes.NewMetric("precursor", output.MustGet(algo.SymbolAlphaPrecursor), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("exhaustion", output.MustGet(algo.SymbolAlphaExhaustion), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
				))
			}

			return true
		})
	}

	return signal.err
}

/*
pending reports whether any symbol queues a Trades frame, so the
run loop can yield without draining empty input.
*/
func (signal *Signal) pending() bool {
	if signal.thesis == nil {
		return false
	}

	hasWork := false

	signal.thesis.Symbols.Range(func(_ any, value any) bool {
		symbol, valid := value.(*types.Symbol)

		if !valid || symbol == nil {
			return true
		}

		if symbol.HasTrades() {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
}

/*
touch finds the most recent ticker quote at or before the print and writes its
bid and ask into the input frame. It returns false when no executable touch
exists, so the print is skipped instead of fabricating a book response.
*/
func touch(
	symbol *types.Symbol,
	trade kraken.TradeData,
	input *nomagique.Frame,
) bool {
	for ticker := range symbol.MarketTickers(types.SourcePumpDump) {
		if ticker.Timestamp.After(trade.Timestamp) {
			continue
		}

		if ticker.Bid == nil || ticker.Ask == nil {
			continue
		}

		bid := ticker.Bid.Float64()
		ask := ticker.Ask.Float64()

		if bid <= 0 || ask <= bid {
			continue
		}

		input.Put(algo.SymbolBid, bid)
		input.Put(algo.SymbolAsk, ask)

		return true
	}

	return false
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
