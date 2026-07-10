package logic

import (
	"math"

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
	thesis *strategy.Thesis
	pearl  *algorithm.Pearl
}

func NewCausal(thesis *strategy.Thesis) *Causal {
	// The causal row is latent[0..4], energy(5), surprise(6): the latent dimension
	// equals resonanceObservables (5). Evaluate the causal effect of the leading
	// latent feature (treatment) on surprise (target), controlling for the
	// remaining latent dimensions and energy. An empty config would default Target
	// and Treatment both to 0, measuring latent_0 on itself.
	causal := &Causal{
		thesis: thesis,
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
	snapshot, ok := causal.thesis.Evidence("resonance")

	if !ok {
		return causal.thesis
	}

	outcome, ok := snapshot.(ResonanceOutcome)

	if !ok {
		return causal.thesis
	}

	// The causal row is the resonance latent state plus its free-energy and
	// surprise scalars: the ladder evaluates their causal relationships over the
	// accumulated row history.
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

	causal.thesis.AddEvidence("causal", output)

	return causal.thesis
}
