package derivatives

import (
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolBasis          = nomagique.MustIntern("derivatives/basis")
	SymbolBasisVelocity  = nomagique.MustIntern("derivatives/basis_velocity")
	SymbolOIAcceleration = nomagique.MustIntern("derivatives/oi_acceleration")
	SymbolIndexBasis     = nomagique.MustIntern("derivatives/index_basis")
	SymbolCVD            = nomagique.MustIntern("derivatives/cvd")
	SymbolLiqIntensity   = nomagique.MustIntern("derivatives/liq_intensity")
)

func basisPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Quotient,
		nomagique.Relay(calculus.SymbolResult, SymbolBasis),
	)
}

func basisVelocityPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(nomagique.SampleValue),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Pipe(
				nomagique.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolBasisVelocity),
			),
			nomagique.Identity,
		),
	)
}

func oiAccelerationPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(nomagique.SampleValue),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Pipe(
				nomagique.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolOIAcceleration),
			),
			nomagique.Identity,
		),
	)
}

func indexBasisPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Quotient,
		nomagique.Relay(calculus.SymbolResult, SymbolIndexBasis),
	)
}

func flowPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolDelta),
		calculus.Accumulate,
		nomagique.Relay(calculus.SymbolTotal, SymbolCVD),
		nomagique.Relay(calculus.SymbolDelta, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		statistic.ZScore,
	)
}

func liqPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Quotient,
		nomagique.Relay(calculus.SymbolResult, SymbolLiqIntensity),
		nomagique.Relay(SymbolLiqIntensity, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		statistic.Deviation,
	)
}

func eventFrame(at time.Time) types.Frame {
	input := types.Frame{}
	input.Put(nmtypes.EventTimeSec, float64(at.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(at.Nanosecond()))
	input.Put(statistic.SymbolUnixSec, float64(at.Unix()))
	input.Put(statistic.SymbolUnixNsec, float64(at.Nanosecond()))

	return input
}

func sampleFrame(at time.Time, value float64) types.Frame {
	input := eventFrame(at)
	input.Put(nomagique.SampleValue, value)

	return input
}
