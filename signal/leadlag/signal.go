package leadlag

import (
	"context"
	"runtime"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolLagRatio   = nomagique.MustIntern("leadlag/inefficiency")
	SymbolLagMeaning = nomagique.MustIntern("leadlag/significance")
)

/*
leadlagPairPipeline is slot-aligned: the follower's velocity and the
anchor-follower displacement (a real Difference between the two prices) are
forked together, then the displacement's ratio and standardized significance
classify the lag without any dummy operands.
*/
func leadlagPairPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			statistic.Velocity,
			calculus.Difference,
		),
		nomagique.Fork(
			nomagique.Pipe(
				nomagique.Relay(calculus.SymbolResult, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolResult, calculus.SymbolRight),
				calculus.Product,
				nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
				nomagique.Relay(nomagique.SampleValue, calculus.SymbolScale),
				calculus.Squash,
				nomagique.Relay(calculus.SymbolResult, SymbolLagRatio),
			),
			statistic.ZScore,
		),
	)
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[[2]string]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[[2]string](leadlagPairPipeline()),
	}

	go signal.run()
	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceLeadLag) }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

func (signal *Signal) run() {
	for {
		select {
		case <-signal.ctx.Done():
			return
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

				for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
					anchor, anchorPrice := pairedPrice(signal.thesis, symbol.Symbol)

					if anchor == "" {
						continue
					}

					input := nomagique.Frame{}
					input.Put(nomagique.SampleValue, ticker.Last.Float64())
					input.Put(calculus.SymbolLeft, anchorPrice)
					input.Put(calculus.SymbolRight, ticker.Last.Float64())
					input.Put(calculus.SymbolValue, ticker.Last.Float64())
					input.Put(calculus.SymbolScale, 1.0)
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
					input.Put(statistic.SymbolDispersionHalflife, 30.0)

					output, err := signal.number([2]string{anchor, symbol.Symbol}, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"leadlag: failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						ticker.Timestamp.UnixNano(),
						ticker.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("inefficiency", output.MustGet(SymbolLagRatio), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("significance", safeGet(output, statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					))
				}

				return true
			})
	}
}

/*
pending reports whether any symbol queues a Tickers frame, so the
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

		if symbol.HasTickers() {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
}
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}

func safeGet(frame nomagique.Frame, slot nomagique.Symbol) float64 {
	value, found := frame.Get(slot)

	if !found {
		return 0
	}

	return value
}

/*
pairedPrice returns the anchor symbol and its latest ticker price for this
follower, so the Difference uses two real prices rather than a zero operand.
*/
func pairedPrice(thesis *types.Thesis, follower string) (string, float64) {
	if thesis == nil {
		return "", 0
	}

	var anchor string
	var anchorPrice float64

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, ok := key.(string)

		if !ok || symbolName == follower {
			return true
		}

		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
			anchor = symbolName
			anchorPrice = ticker.Last.Float64()

			return false
		}

		return true
	})

	return anchor, anchorPrice
}
