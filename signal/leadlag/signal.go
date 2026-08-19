package leadlag

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolLagRatio    = nomagique.MustIntern("leadlag/inefficiency")
	SymbolLagMeaning  = nomagique.MustIntern("leadlag/significance")
)

/*
leadlagPairPipeline composes the pairwise leader-follower relation from the
shared atomic math units: aligned elapsed duration, follower velocity beside
the anchor-follower displacement, then an inefficiency ratio and a lag
significance gate.
*/
func leadlagPairPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Duration,
		nomagique.Fork(
			statistic.Velocity,
			calculus.Difference,
		),
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Ratio,
				calculus.Squash,
				nomagique.Relay(calculus.SymbolResult, SymbolLagRatio),
			),
			nomagique.Pipe(
				statistic.ZScore,
				logic.Gate,
				nomagique.Relay(logic.SymbolResult, SymbolLagMeaning),
			),
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

	signal.run()
	return signal
}

func (signal *Signal) Name() string { return string(types.SourceLeadLag) }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

func (signal *Signal) run() {
	for {
		select {
		case <-signal.ctx.Done():
			return
		default:
			signal.thesis.Symbols.Range(func(_ any, value any) bool {
				symbol, valid := value.(*types.Symbol)

				if !valid || symbol == nil {
					return true
				}

				for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
					anchor := anchorFor(signal.thesis, symbol.Symbol)

					if anchor == "" {
						continue
					}

					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, ticker.Last.Float64())
					input.Put(calculus.SymbolCurrent, ticker.Last.Float64())
					input.Put(calculus.SymbolPrevious, ticker.Last.Float64())
					input.Put(calculus.SymbolLeft, ticker.Last.Float64())
					input.Put(calculus.SymbolRight, ticker.Last.Float64())
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

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
						nmtypes.NewMetric("significance", output.MustGet(SymbolLagMeaning), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					))
				}

				return true
			})
		}
	}
}

/*
anchorFor returns the thesis's anchor symbol for one follower, if one has been
established. Anchor selection is cross-sectional state owned by the thesis; the
signal's recipe only consumes the pair.
*/
func anchorFor(thesis *types.Thesis, follower string) string {
	if thesis == nil {
		return ""
	}

	var anchor string

	thesis.Symbols.Range(func(key, _ any) bool {
		symbolName, ok := key.(string)

		if !ok {
			return true
		}

		anchor = symbolName

		return false
	})

	if anchor == follower {
		return ""
	}

	return anchor
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
