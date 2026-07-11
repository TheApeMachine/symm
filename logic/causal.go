package logic

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
causalKey is the fixed sample key: the causal ladder accumulates one aligned row
history for the resonance signal.
*/
const causalKey = "resonance"

type Causal struct {
	symbol string
	thesis *strategy.Thesis
	ui     chan []byte
	pearl  *algorithm.Pearl
}

func NewCausal(symbol string, thesis *strategy.Thesis, ui chan []byte) *Causal {
	causal := &Causal{
		symbol: symbol,
		thesis: thesis,
		ui:     ui,
		pearl: algorithm.NewPearl(
			algorithm.PearlConfig{
				Target:    6,
				Treatment: 0,
				Controls:  []int{1, 2, 3, 4, 5},
				CategoryIndexes: []float64{
					float64(types.CategoryIndex(types.SystemicBeta)),    // association
					float64(types.CategoryIndex(types.LiquidityShock)),  // intervention
					float64(types.CategoryIndex(types.EndogenousAlpha)), // counterfactual
					float64(types.CategoryIndex(types.CausalNoise)),     // residual
				},
			},
		),
	}

	return causal
}

func (causal *Causal) Update() *strategy.Thesis {
	snapshot, ok := causal.thesis.Evidence(causal.symbol, "resonance")

	if !ok {
		return causal.thesis
	}

	outcome, ok := snapshot.(ResonanceOutcome)

	if !ok {
		return causal.thesis
	}

	row := append(append([]float64(nil), outcome.Latent...), outcome.Energy, outcome.Surprise)

	for _, value := range row {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic causal: non-finite causal row value",
				nil,
			))

			return causal.thesis
		}
	}

	output, ready, err := causal.pearl.Measure(algorithm.PearlInput{
		Key: causalKey,
		Row: row,
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic causal: failed to measure causal ladder",
			err,
		))

		return causal.thesis
	}

	if !ready {
		return causal.thesis
	}

	causal.thesis.AddEvidence(causal.symbol, "causal", output)

	if causal.ui != nil {
		frame := datura.Map[any]{"causal": output}

		if symbol, ok := causal.thesis.Evidence(causal.symbol, "symbol"); ok {
			frame["symbol"] = symbol
		}

		select {
		case causal.ui <- frame.Marshal():
		default:
		}
	}

	return causal.thesis
}
