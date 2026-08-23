package derivatives

import (
	"time"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolBasis          = nmtypes.MustIntern("derivatives/basis")
	SymbolBasisVelocity  = nmtypes.MustIntern("derivatives/basis_velocity")
	SymbolOIAcceleration = nmtypes.MustIntern("derivatives/oi_acceleration")
	SymbolIndexBasis     = nmtypes.MustIntern("derivatives/index_basis")
	SymbolCVD            = nmtypes.MustIntern("derivatives/cvd")
	SymbolLiqIntensity   = nmtypes.MustIntern("derivatives/liq_intensity")
)

func basisPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Difference,
		nmtypes.Relay(calculus.SymbolResult, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Quotient,
		nmtypes.Relay(calculus.SymbolResult, SymbolBasis),
	)
}

func basisVelocityPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		temporal.Observer(nmtypes.SampleValue),
		logic.If(
			nmtypes.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nmtypes.Pipe(
				nmtypes.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
				nmtypes.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Difference,
				nmtypes.Relay(calculus.SymbolResult, SymbolBasisVelocity),
			),
			nmtypes.Identity,
		),
	)
}

func oiAccelerationPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		temporal.Observer(nmtypes.SampleValue),
		logic.If(
			nmtypes.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nmtypes.Pipe(
				nmtypes.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
				nmtypes.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Difference,
				nmtypes.Relay(calculus.SymbolResult, SymbolOIAcceleration),
			),
			nmtypes.Identity,
		),
	)
}

func indexBasisPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Difference,
		nmtypes.Relay(calculus.SymbolResult, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Quotient,
		nmtypes.Relay(calculus.SymbolResult, SymbolIndexBasis),
	)
}

func flowPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		calculus.Difference,
		nmtypes.Relay(calculus.SymbolResult, calculus.SymbolDelta),
		calculus.Accumulate,
		nmtypes.Relay(calculus.SymbolTotal, SymbolCVD),
		nmtypes.Relay(calculus.SymbolDelta, nmtypes.SampleValue),
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		statistic.ZScore,
	)
}

func liqPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		calculus.Quotient,
		nmtypes.Relay(calculus.SymbolResult, SymbolLiqIntensity),
		nmtypes.Relay(SymbolLiqIntensity, nmtypes.SampleValue),
		nmtypes.Configure(
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
	input.Put(nmtypes.SampleValue, value)

	return input
}
