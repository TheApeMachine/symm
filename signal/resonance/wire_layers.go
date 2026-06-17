package resonance

import (
	"math"

	"github.com/theapemachine/nomagique/learning"
)

func wireLayersFromBatch(
	arch []int,
	input []float64,
	latent []float64,
	surprise float64,
) []learning.ResonanceLayerWire {
	layers := make([]learning.ResonanceLayerWire, len(arch))
	layerError := surprise / float64(len(arch))

	if layerError <= 0 || math.IsNaN(layerError) || math.IsInf(layerError, 0) {
		layerError = surprise
	}

	if len(arch) > 0 {
		state := append([]float64(nil), input...)
		prediction := make([]float64, len(state))
		layers[0] = learning.ResonanceLayerWire{
			State:      state,
			Prediction: prediction,
			ErrorNorm:  layerError,
		}
	}

	for layerIndex := 1; layerIndex < len(arch)-1; layerIndex++ {
		dimension := arch[layerIndex]
		layers[layerIndex] = learning.ResonanceLayerWire{
			State:      make([]float64, dimension),
			Prediction: make([]float64, dimension),
			ErrorNorm:  layerError,
		}
	}

	if len(arch) > 1 {
		topIndex := len(arch) - 1
		state := append([]float64(nil), latent...)
		prediction := make([]float64, len(state))
		layers[topIndex] = learning.ResonanceLayerWire{
			State:      state,
			Prediction: prediction,
			ErrorNorm:  layerError,
		}
	}

	return layers
}
