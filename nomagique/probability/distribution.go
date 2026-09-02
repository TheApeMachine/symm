package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Distribution represents the unpacked result of a schema classification.
*/
type Distribution struct {
	Ready         bool
	WinnerIndex   int
	WinnerLabel   string
	Confidence    float64
	Ambiguity     float64
	Probabilities map[string]float64
}

/*
ProjectDistribution unpacks the classification distribution from an evaluated Frame.
*/
func ProjectDistribution(frame types.Frame, labels []string) Distribution {
	ready, found := frame.Get(types.SampleReady)
	if !found || ready == 0 {
		return Distribution{Ready: false}
	}

	dist := Distribution{
		Ready:         true,
		Probabilities: make(map[string]float64, len(labels)),
	}

	if winner, ok := frame.Get(SymbolWinner); ok {
		dist.WinnerIndex = int(winner)
		if dist.WinnerIndex >= 0 && dist.WinnerIndex < len(labels) {
			dist.WinnerLabel = labels[dist.WinnerIndex]
		}
	}

	if conf, ok := frame.Get(SymbolConfidence); ok {
		dist.Confidence = conf
	}

	if amb, ok := frame.Get(SymbolAmbiguity); ok {
		dist.Ambiguity = amb
	}

	for i, label := range labels {
		if prob, ok := frame.Get(types.MustSampleSymbol(i)); ok {
			dist.Probabilities[label] = prob
		}
	}

	return dist
}
