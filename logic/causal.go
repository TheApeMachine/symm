package logic

import (
	"math"

	"github.com/theapemachine/errnie"
	pearl "github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/types"
)

type causalCounterfactual struct {
	pearl *pearl.Pearl
}

func newCausalCounterfactual() *causalCounterfactual {
	return &causalCounterfactual{
		pearl: pearl.NewPearl(pearl.PearlConfig{
			Target:                  0,
			Treatment:               14,
			Controls:                []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13},
			NonlinearCounterfactual: true,
			CategoryIndexes: []float64{
				float64(types.CategoryIndex(types.CategoryEndogenousAlpha)),
				float64(types.CategoryIndex(types.CategorySystemicBeta)),
				float64(types.CategoryIndex(types.CategoryLiquidityShock)),
				float64(types.CategoryIndex(types.CategoryCausalNoise)),
			},
		}),
	}
}

func (counterfactual *causalCounterfactual) Evaluate(
	symbol string,
	frame boundaryFrame,
	physical physicalEvidence,
	intervened physicalEvidence,
	predictive predictiveEvidence,
) (counterfactualEvidence, bool, error) {
	interventionFlow := frame.Intervene().netMomentum()

	output, ready, err := counterfactual.pearl.Measure(pearl.PearlInput{
		Key: symbol,
		Row: []float64{
			physical.rho.gradient,
			physical.rho.mass,
			physical.rho.entropy,
			physical.rho.spreadX,
			physical.rho.spreadZ,
			physical.oscillators.coherence,
			physical.oscillators.kinetic,
			physical.oscillators.thermal,
			predictive.flow,
			predictive.stress,
			predictive.coupling,
			intervened.rho.gradient,
			intervened.oscillators.coherence,
			predictive.surprise,
			frame.netMomentum(),
			frame.netPressure(),
		},
		Intervention: interventionFlow,
		Condition:    predictive.surprise,
		Contagion:    intervened.rho.gradient - physical.rho.gradient,
	})

	if err != nil {
		return counterfactualEvidence{}, false, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return counterfactualEvidence{}, false, nil
	}

	category := types.CategoryByIndex(int(math.Round(output.Category)))

	if category == types.CategoryTypeNone {
		return counterfactualEvidence{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision causal: Pearl category required",
			nil,
		))
	}

	if output.EntryBaseline <= 0 {
		return counterfactualEvidence{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision causal: Pearl entry baseline required",
			nil,
		))
	}

	return counterfactualEvidence{
		category:     category,
		confidence:   output.Confidence,
		strength:     output.Strength,
		baseline:     output.EntryBaseline,
		uplift:       output.Uplift,
		intervention: output.InterventionScore,
		beta:         output.AssociationScore,
		panic:        math.Abs(output.Condition) + math.Abs(output.Contagion),
		residual:     output.Residual(),
	}, true, nil
}
