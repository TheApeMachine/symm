package logic

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	resonance "github.com/theapemachine/nomagique/learning/manifold"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
)

var predictiveModes = []types.CategoryType{
	types.CategoryLaminarResonance,
	types.CategoryTurbulentResonance,
	types.CategoryEquilibrium,
}

type cognitiveManifold struct {
	solver *resonance.BatchSolver
	learn  bool
}

func newCognitiveManifold() (*cognitiveManifold, error) {
	inputWidth := len(physicalVector(physicalEvidence{}))
	arch := []int{
		inputWidth,
		inputWidth * 2,
		inputWidth,
		len(predictiveModes),
	}
	alpha := 1 / float64(inputWidth)

	var solver *resonance.BatchSolver
	err := compute.WithMetalInit(func() error {
		created, err := resonance.NewBatchSolver(arch, 0, 1, alpha)
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision cognitive: failed to create resonance solver",
				err,
			))
		}

		solver = created
		return nil
	})
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to create resonance solver",
			err,
		))
	}

	return &cognitiveManifold{
		solver: solver,
		learn:  viper.GetBool("logic.resonance.learn"),
	}, nil
}

func (cognitive *cognitiveManifold) Close() {
	if cognitive == nil || cognitive.solver == nil {
		return
	}

	cognitive.solver.Close()
	cognitive.solver = nil
}

func (cognitive *cognitiveManifold) Settle(
	physical physicalEvidence,
) (predictiveEvidence, error) {
	if cognitive == nil || cognitive.solver == nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: solver is not initialized",
			nil,
		))
	}

	if err := cognitive.solver.SetInput(0, physicalVector(physical), nil); err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to set resonance input",
			err,
		))
	}

	if err := cognitive.solver.Settle(true); err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to settle resonance",
			err,
		))
	}

	if cognitive.learn {
		if err := cognitive.solver.Learn(); err != nil {
			return predictiveEvidence{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"decision cognitive: failed to learn resonance",
				err,
			))
		}
	}

	if err := cognitive.solver.ReadOutcomes(); err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to read resonance outcomes",
			err,
		))
	}

	latent, energy, surprise, err := cognitive.solver.OutcomeSlot(0)
	if err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to read resonance outcome slot",
			err,
		))
	}

	return cognitive.evidence(latent, energy, surprise)
}

func physicalVector(physical physicalEvidence) []float64 {
	reading := physical.reading
	projection := physical.projection

	return []float64{
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
		projection.PressureGradNorm,
		physical.rho.mass,
		physical.rho.peak,
		physical.rho.entropy,
		physical.rho.gradient,
		physical.oscillators.coherence,
		physical.oscillators.kinetic,
	}
}

func (cognitive *cognitiveManifold) evidence(
	latent []float64,
	energy float64,
	surprise float64,
) (predictiveEvidence, error) {
	flow := latentMode(latent, 0)
	stress := latentMode(latent, 1)
	coupling := latentMode(latent, 2)
	scores := []float64{flow, stress, coupling}
	category := types.CategoryLaminarResonance
	categoryIndex := 1

	if stress >= flow {
		category = types.CategoryTurbulentResonance
		categoryIndex = 2
	}

	if coupling >= flow && coupling >= stress {
		category = types.CategoryEquilibrium
		categoryIndex = 3
	}

	confidence, err := probability.CategoryShareConfidence(scores, categoryIndex)
	if err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to calculate confidence",
			err,
		))
	}

	_, baseline, _, err := probability.CategoryEvidenceBaselines(scores, categoryIndex)
	if err != nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to calculate baseline",
			err,
		))
	}

	return predictiveEvidence{
		category:   category,
		confidence: confidence,
		flow:       flow,
		stress:     stress,
		coupling:   coupling,
		baseline:   baseline,
		energy:     energy,
		surprise:   surprise,
		latent:     append([]float64(nil), latent...),
	}, nil
}

func latentMode(latent []float64, index int) float64 {
	if index >= len(latent) {
		return 0
	}

	return math.Abs(latent[index])
}
