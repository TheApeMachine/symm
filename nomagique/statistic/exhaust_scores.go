package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolMechanical    = types.MustIntern("exhaust/mechanical")
	SymbolFragile       = types.MustIntern("exhaust/fragile")
	SymbolThermal       = types.MustIntern("exhaust/thermal")
	SymbolReversal      = types.MustIntern("exhaust/reversal")
	SymbolUrgency       = types.MustIntern("exhaust/urgency")
	SymbolVolume        = types.MustIntern("exhaust/volume")
	SymbolSpread        = types.MustIntern("exhaust/spread")
	SymbolPriceDelta    = types.MustIntern("exhaust/price_delta")
	SymbolAggressorSide = types.MustIntern("exhaust/aggressor_side")
)

var symbolPreviousFlow = types.MustIntern("state/prev_flow")

/*
ExhaustScores evaluates the four physical decay channels — mechanical depth
collapse, fragile spread expansion, thermal price rejection, and directional
reversal — and fuses them into an urgency margin. It is a primitive over the
baseline-adapted volume, spread, price delta, and aggressor side facts.
*/
func ExhaustScores(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	volume, _ := input.Get(SymbolVolume)
	spread, _ := input.Get(SymbolSpread)
	priceDelta, _ := input.Get(SymbolPriceDelta)
	aggressorSide, _ := input.Get(SymbolAggressorSide)
	baselineVolume, hasBaseline := input.Get(SymbolBaselineValue)

	if !hasBaseline {
		baselineVolume = volume
	}

	mechanical := 0.0

	if baselineVolume > 0 && volume < baselineVolume {
		mechanical = (baselineVolume - volume) / baselineVolume
	}

	fragile := 0.0

	if spread > 0 {
		fragile = spread / (spread + 1.0)
	}

	thermal := 0.0
	isAdverse := (aggressorSide > 0 && priceDelta < 0) || (aggressorSide < 0 && priceDelta > 0)

	if isAdverse {
		rejectionScale := math.Abs(priceDelta)
		pressureShare := volume / (volume + baselineVolume)
		thermal = (rejectionScale / (rejectionScale + 1.0)) * pressureShare
	}

	previousFlow, hasFlow := state.Get(symbolPreviousFlow)
	reversal := 0.0

	if hasFlow && previousFlow*aggressorSide < 0 {
		reversal = volume / (volume + baselineVolume)
	}

	nextState := state
	nextState.Put(symbolPreviousFlow, aggressorSide*volume)

	urgency := math.Max(mechanical, math.Max(fragile, math.Max(thermal, reversal)))

	output := input
	output.Put(SymbolMechanical, mechanical)
	output.Put(SymbolFragile, fragile)
	output.Put(SymbolThermal, thermal)
	output.Put(SymbolReversal, reversal)
	output.Put(SymbolUrgency, urgency)

	return nextState, output, nil
}
