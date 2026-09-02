package vector

import (
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Distribution represents the unpacked result of an AdaptiveClassifier evaluation.
*/
type Distribution struct {
	Ready         bool
	WinnerIndex   int
	WinnerLabel   string
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
	Probabilities map[string]float64
}

/*
Unpack extracts the classification distribution from an evaluated Frame.
*/
func Unpack(frame types.Frame, groups []Group) Distribution {
	ready, found := frame.Get(types.SampleReady)
	if !found || ready == 0 {
		return Distribution{Ready: false}
	}

	dist := Distribution{
		Ready:         true,
		Probabilities: make(map[string]float64, len(groups)),
	}

	if winner, ok := frame.Get(probability.SymbolWinner); ok {
		dist.WinnerIndex = int(winner)
		if dist.WinnerIndex >= 0 && dist.WinnerIndex < len(groups) {
			dist.WinnerLabel = groups[dist.WinnerIndex].Label
		}
	}

	if conf, ok := frame.Get(probability.SymbolConfidence); ok {
		dist.Confidence = conf
	}

	if amb, ok := frame.Get(probability.SymbolAmbiguity); ok {
		dist.Ambiguity = amb
		dist.Sharpness = 1 - amb
	}

	for i, g := range groups {
		if prob, ok := frame.Get(types.MustSampleSymbol(i)); ok {
			dist.Probabilities[g.Label] = prob
		}
	}

	return dist
}
