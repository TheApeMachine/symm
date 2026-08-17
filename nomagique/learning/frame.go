package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

const MaxFrameFeatures = 64

var (
	SymbolFeatureCount   = nomagique.MustIntern("resonance/feature_count")
	SymbolInferenceSteps = nomagique.MustIntern("resonance/inference_steps")
	SymbolInvocation     = nomagique.MustIntern("resonance/invocation")
	SymbolEnergy         = nomagique.MustIntern("resonance/energy")
	SymbolSurprise       = nomagique.MustIntern("resonance/surprise")
	SymbolLatentCount    = nomagique.MustIntern("resonance/latent_count")
	featureSymbols       [MaxFrameFeatures]nomagique.Symbol
	latentSymbols        [MaxFrameFeatures]nomagique.Symbol
)

func init() {
	for featureIndex := range MaxFrameFeatures {
		featureSymbols[featureIndex] = nomagique.MustIntern(
			fmt.Sprintf("resonance/feature/%d", featureIndex),
		)
		latentSymbols[featureIndex] = nomagique.MustIntern(
			fmt.Sprintf("resonance/latent/%d", featureIndex),
		)
	}
}

/*
FeatureSymbol returns the interned Frame slot used for one ordered resonance
feature. Feature order remains the caller-owned schema contract.
*/
func FeatureSymbol(featureIndex int) nomagique.Symbol {
	if featureIndex < 0 || featureIndex >= MaxFrameFeatures {
		panic(fmt.Sprintf(
			"resonance: feature index %d is outside Frame capacity",
			featureIndex,
		))
	}

	return featureSymbols[featureIndex]
}

/*
LatentSymbol returns the interned Frame slot used for one settled top-layer
coordinate.
*/
func LatentSymbol(latentIndex int) nomagique.Symbol {
	if latentIndex < 0 || latentIndex >= MaxFrameFeatures {
		panic(fmt.Sprintf(
			"resonance: latent index %d is outside Frame capacity",
			latentIndex,
		))
	}

	return latentSymbols[latentIndex]
}

/*
FramePrimitive adapts the dense model to nomagique's universal Frame boundary.
The manifold retains its preallocated matrices and workspace; the reducer state
only records successful invocations. This keeps model geometry out of transport
payloads while removing string maps and per-step feature allocation at the
boundary.
*/
func FramePrimitive(
	manifold *ResonanceManifold,
	learn bool,
) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		if manifold == nil {
			return state, nomagique.Frame{}, fmt.Errorf(
				"resonance: manifold required",
			)
		}

		featureCountValue, hasFeatureCount := input.Get(SymbolFeatureCount)

		if !hasFeatureCount {
			return state, nomagique.Frame{}, fmt.Errorf(
				"resonance: feature count required",
			)
		}

		featureCount := int(featureCountValue)

		if featureCount <= 0 || featureCount > MaxFrameFeatures ||
			featureCount != manifold.arch[0] {
			return state, nomagique.Frame{}, fmt.Errorf(
				"resonance: feature count %d does not match input width %d",
				featureCount,
				manifold.arch[0],
			)
		}

		var featureStorage [MaxFrameFeatures]float64

		for featureIndex := range featureCount {
			feature, found := input.Get(featureSymbols[featureIndex])

			if !found {
				return state, nomagique.Frame{}, fmt.Errorf(
					"resonance: feature %d required",
					featureIndex,
				)
			}

			featureStorage[featureIndex] = feature
		}

		_, err := manifold.SettleFromBatchOptions(
			featureStorage[:featureCount],
			nil,
			learn,
			false,
		)

		if err != nil {
			return state, nomagique.Frame{}, err
		}

		invocation, _ := state.Get(SymbolInvocation)
		nextState := state
		nextState.Put(SymbolInvocation, invocation+1)

		output := nomagique.Frame{}
		output.Put(SymbolEnergy, manifold.Energy())
		output.Put(SymbolSurprise, manifold.ReconstructionError())
		output.Put(SymbolInferenceSteps, float64(manifold.lastInferenceSteps))

		topLayer := manifold.latentStates[len(manifold.latentStates)-1].RawVector().Data
		latentCount := min(len(topLayer), MaxFrameFeatures)
		output.Put(SymbolLatentCount, float64(latentCount))

		for latentIndex := range latentCount {
			output.Put(latentSymbols[latentIndex], topLayer[latentIndex])
		}

		return nextState, output, nil
	}
}
