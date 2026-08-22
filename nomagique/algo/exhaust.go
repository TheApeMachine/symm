package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolMechanical    = nomagique.MustIntern("exhaust/mechanical")
	SymbolFragile       = nomagique.MustIntern("exhaust/fragile")
	SymbolThermal       = nomagique.MustIntern("exhaust/thermal")
	SymbolReversal      = nomagique.MustIntern("exhaust/reversal")
	SymbolUrgency       = nomagique.MustIntern("exhaust/urgency")
	SymbolVolume        = nomagique.MustIntern("exhaust/volume")
	SymbolSpread        = nomagique.MustIntern("exhaust/spread")
	SymbolPriceDelta    = nomagique.MustIntern("exhaust/price_delta")
	SymbolAggressorSide = nomagique.MustIntern("exhaust/aggressor_side")
)

/*
Exhaust calculates the four physical decay channels (mechanical depth collapse,
fragile spread expansion, thermal price rejection, and directional reversal)
and fuses them into an urgency margin.
*/
func Exhaust() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolVolume, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		exhaustEvaluator,
	)
}

func exhaustEvaluator(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
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
	prevFlow, hasFlow := state.Get(nomagique.MustIntern("state/prev_flow"))
	reversal := 0.0

	if hasFlow && prevFlow*aggressorSide < 0 {
		reversal = volume / (volume + baselineVolume)
	}

	nextState := state
	nextState.Put(nomagique.MustIntern("state/prev_flow"), aggressorSide*volume)

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
