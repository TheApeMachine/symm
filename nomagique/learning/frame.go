package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

const (
	MaxFrameFeatures = 32
	MaxFrameLayers   = 4
	MaxLayerDim      = 32
)

var (
	SymbolFeatureCount        = types.MustIntern("resonance/feature_count")
	SymbolInferenceSteps      = types.MustIntern("resonance/inference_steps")
	SymbolInvocation          = types.MustIntern("resonance/invocation")
	SymbolEnergy              = types.MustIntern("resonance/energy")
	SymbolSurprise            = types.MustIntern("resonance/surprise")
	SymbolReconstructionError = types.MustIntern("resonance/reconstruction_error")
	SymbolLatentCount         = types.MustIntern("resonance/latent_count")
	SymbolInnovationCount     = types.MustIntern("resonance/innovation_count")

	featureSymbols  [MaxFrameFeatures]types.Symbol
	layerLatentSyms [MaxFrameLayers][MaxLayerDim]types.Symbol
	layerInnoSyms   [MaxFrameLayers][MaxLayerDim]types.Symbol
)

func init() {
	for featureIndex := range MaxFrameFeatures {
		featureSymbols[featureIndex] = types.MustIntern(
			fmt.Sprintf("resonance/feature/%d", featureIndex),
		)
	}

	for layer := range MaxFrameLayers {
		for index := range MaxLayerDim {
			layerLatentSyms[layer][index] = types.MustIntern(
				fmt.Sprintf("resonance/layer/%d/latent/%d", layer+1, index),
			)
			layerInnoSyms[layer][index] = types.MustIntern(
				fmt.Sprintf("resonance/layer/%d/innovation/%d", layer, index),
			)
		}
	}
}

func FeatureSymbol(featureIndex int) types.Symbol {
	if featureIndex < 0 || featureIndex >= MaxFrameFeatures {
		panic(fmt.Sprintf("resonance: feature index %d is outside Frame capacity", featureIndex))
	}
	return featureSymbols[featureIndex]
}

func LatentSymbol(layerIndex int, index int) types.Symbol {
	if layerIndex < 0 || layerIndex >= MaxFrameLayers || index < 0 || index >= MaxLayerDim {
		panic("resonance: layer or latent index outside Frame capacity")
	}
	return layerLatentSyms[layerIndex][index]
}

func InnovationSymbol(layerIndex int, index int) types.Symbol {
	if layerIndex < 0 || layerIndex >= MaxFrameLayers || index < 0 || index >= MaxLayerDim {
		panic("resonance: layer or innovation index outside Frame capacity")
	}
	return layerInnoSyms[layerIndex][index]
}

/*
FramePrimitive adapts the multi-timescale, overcomplete predictive coding manifold
into types's universal Frame reducer interface.
*/
func FramePrimitive(manifold *ResonanceManifold, learn bool) types.Primitive {
	return func(input types.Frame) types.Frame {
		if manifold == nil {
			input.Err = fmt.Errorf("resonance: manifold required")
			return input
		}

		featureCountValue, hasFeatureCount := input.Get(SymbolFeatureCount)
		featureCount := manifold.arch[0]
		if hasFeatureCount {
			featureCount = int(featureCountValue)
		}

		if featureCount <= 0 || featureCount > MaxFrameFeatures || featureCount != manifold.arch[0] {
			input.Err = fmt.Errorf(
				"resonance: feature count %d does not match input width %d",
				featureCount, manifold.arch[0],
			)
			return input
		}

		var featureStorage [MaxFrameFeatures]float64
		for featureIndex := range featureCount {
			feature, found := input.Get(featureSymbols[featureIndex])
			if !found {
				input.Err = fmt.Errorf("resonance: feature %d required", featureIndex)
				return input
			}
			featureStorage[featureIndex] = feature
		}

		if err := manifold.Settle(featureStorage[:featureCount], !learn); err != nil {
			input.Err = err
			return input
		}

		if learn {
			if err := manifold.Learn(nil); err != nil {
				input.Err = err
				return input
			}
		}

		invocation, _ := input.Get(SymbolInvocation)
		input.Put(SymbolInvocation, invocation+1)

		input.Put(SymbolEnergy, manifold.Energy())
		input.Put(SymbolSurprise, manifold.ReconstructionError())
		input.Put(SymbolReconstructionError, manifold.ReconstructionError())
		input.Put(SymbolInferenceSteps, float64(manifold.lastInferenceSteps))

		// Export multi-layer dictionary latents
		totalLatents := 0
		for layer := 1; layer < len(manifold.latentStates) && layer-1 < MaxFrameLayers; layer++ {
			data := manifold.latentStates[layer].RawVector().Data
			count := min(len(data), MaxLayerDim)
			for i := range count {
				input.Put(layerLatentSyms[layer-1][i], data[i])
			}
			totalLatents += count
		}
		input.Put(SymbolLatentCount, float64(totalLatents))

		// Export multi-layer prediction error innovations
		_, layerErrors := manifold.predictAdjacentLayers()
		totalInnovations := 0
		for layer := range layerErrors {
			if layer >= MaxFrameLayers {
				break
			}
			data := layerErrors[layer].RawVector().Data
			count := min(len(data), MaxLayerDim)
			for i := range count {
				input.Put(layerInnoSyms[layer][i], data[i])
			}
			totalInnovations += count
		}
		input.Put(SymbolInnovationCount, float64(totalInnovations))

		return input
	}
}
