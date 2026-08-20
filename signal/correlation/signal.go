package correlation

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
	SymbolCohortRelation = nomagique.MustIntern("correlation/relation")
	SymbolCohortSign     = nomagique.MustIntern("correlation/sign")
)

/*
correlationPairPipeline composes the pairwise relation from the shared atomic
math units: the two returns are referenced against each other through
log-ratio and ratio, then standardized around the pair's own baseline so a
symbol is measured against its peer rather than against itself.
*/
func correlationPairPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolLeft),
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolRight),
		calculus.Product,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
		nomagique.Relay(calculus.SymbolValue, calculus.SymbolBaseline),
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Squash,
				nomagique.Relay(calculus.SymbolResult, SymbolCohortRelation),
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
		number: nomagique.NewNumber[[2]string](correlationPairPipeline()),
	}

	go signal.run()
	return signal
}

func (signal *Signal) Name() string  { return string(types.SourceCorrelation) }
func (signal *Signal) Type() types.SourceType {
	return types.SourceCorrelation
}

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

				peer := peerFor(signal.thesis, symbol.Symbol)

				if peer == "" {
					return true
				}

				for ticker := range symbol.MarketTickers(types.SourceCorrelation) {
					signal.thesis.Symbols.Range(func(peerKey, peerValue any) bool {
						peerSymbol, ok := peerValue.(*types.Symbol)

						if !ok || peerSymbol == nil || peerSymbol.Symbol != peer {
							return true
						}

						for peerTicker := range peerSymbol.MarketTickers(types.SourceCorrelation) {
							input := nomagique.Frame{}
							input.Put(calculus.SymbolCurrent, ticker.Last.Float64())
							input.Put(calculus.SymbolPrevious, ticker.Last.Float64())
							input.Put(calculus.SymbolLeft, ticker.Last.Float64())
							input.Put(calculus.SymbolRight, peerTicker.Last.Float64())
							input.Put(calculus.SymbolScale, 1.0)
							input.Put(nomagique.SampleValue, ticker.Last.Float64())
							input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
							input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
							input.Put(statistic.SymbolDispersionHalflife, 30.0)

							output, err := signal.number(
								[2]string{symbol.Symbol, peer},
								input,
							)

							if err != nil {
								errnie.Error(errnie.Err(
									errnie.Validation,
									"correlation: failed for "+symbol.Symbol,
									err,
								))
								return false
							}

							symbol.Measurements.Push(nmtypes.NewMeasurement(
								uuid.NewString(),
								signal.Name(),
								ticker.Timestamp.UnixNano(),
								ticker.Timestamp.UnixNano(),
							).AddMetrics(
								nmtypes.NewMetric("relation", output.MustGet(SymbolCohortRelation), nmtypes.Descriptor{
									Unit:      nmtypes.UnitDimensionless,
									Timescale: nmtypes.TimescaleInstantaneous,
								}),
							))

							return false
						}

						return true
					})
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
/*
peerFor returns some other registered symbol against which this symbol's
relation is measured. The pair key isolates each relation in its own stream.
*/
func peerFor(thesis *types.Thesis, self string) string {
	if thesis == nil {
		return ""
	}

	var peer string

	thesis.Symbols.Range(func(key, _ any) bool {
		symbolName, ok := key.(string)

		if !ok {
			return true
		}

		if symbolName != self {
			peer = symbolName

			return false
		}

		return true
	})

	return peer
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
