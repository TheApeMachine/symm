package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolMechanical    = nmtypes.MustIntern("exhaust/mechanical")
	SymbolFragile       = nmtypes.MustIntern("exhaust/fragile")
	SymbolThermal       = nmtypes.MustIntern("exhaust/thermal")
	SymbolReversal      = nmtypes.MustIntern("exhaust/reversal")
	SymbolUrgency       = nmtypes.MustIntern("exhaust/urgency")
	SymbolVolume        = nmtypes.MustIntern("exhaust/volume")
	SymbolSpread        = nmtypes.MustIntern("exhaust/spread")
	SymbolPriceDelta    = nmtypes.MustIntern("exhaust/price_delta")
	SymbolAggressorSide = nmtypes.MustIntern("exhaust/aggressor_side")
)

/*
Exhaust calculates the four physical decay channels (mechanical depth collapse,
fragile spread expansion, thermal price rejection, and directional reversal)
and fuses them into an urgency margin.
*/
func Exhaust() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(SymbolVolume, nmtypes.SampleValue),
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		exhaustEvaluator,
	)
}

func exhaustEvaluator(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	volume, _ := input.Get(SymbolVolume)
	spread, _ := input.Get(SymbolSpread)
	priceDelta, _ := input.Get(SymbolPriceDelta)
	aggressorSide, _ := input.Get(SymbolAggressorSide)
	baselineVolume, hasBaseline := input.Get(statistic.SymbolBaselineValue)

	if !hasBaseline {
		baselineVolume = volume
	}

	// 1. Mechanical: volume / participation dropping below its baseline
	mechanical := 0.0

	if baselineVolume > 0 && volume < baselineVolume {
		mechanical = (baselineVolume - volume) / baselineVolume
	}

	// 2. Fragile: spread widening relative to baseline
	fragile := 0.0

	if spread > 0 {
		fragile = spread / (spread + 1.0)
	}

	// 3. Thermal: adverse price movement against aggressor pressure
	thermal := 0.0
	isAdverse := (aggressorSide > 0 && priceDelta < 0) || (aggressorSide < 0 && priceDelta > 0)

	if isAdverse {
		rejectionScale := math.Abs(priceDelta)
		pressureShare := volume / (volume + baselineVolume)
		thermal = (rejectionScale / (rejectionScale + 1.0)) * pressureShare
	}

	// 4. Reversal: sign flip opposing recent directional pressure
	prevFlow, hasFlow := state.Get(nmtypes.MustIntern("state/prev_flow"))
	reversal := 0.0

	if hasFlow && prevFlow*aggressorSide < 0 {
		reversal = volume / (volume + baselineVolume)
	}

	nextState := state
	nextState.Put(nmtypes.MustIntern("state/prev_flow"), aggressorSide*volume)

	// 5. Urgency: fused maximum over the four decay channels
	urgency := math.Max(mechanical, math.Max(fragile, math.Max(thermal, reversal)))

	output := input
	output.Put(SymbolMechanical, mechanical)
	output.Put(SymbolFragile, fragile)
	output.Put(SymbolThermal, thermal)
	output.Put(SymbolReversal, reversal)
	output.Put(SymbolUrgency, urgency)

	return nextState, output, nil
}
