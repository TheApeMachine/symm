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

// resonanceTargetDim activates the supervised task head (the third "V" head) with
// a single output: the adaptive-horizon forward-return prediction.
const resonanceTargetDim = 1

type cognitiveManifold struct {
	solver *resonance.BatchSolver
	target *resonanceTarget
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
		created, err := resonance.NewBatchSolver(arch, resonanceTargetDim, 1, alpha)
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
		target: newResonanceTarget(),
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
	symbol string,
	price float64,
	physical physicalEvidence,
) (predictiveEvidence, error) {
	if cognitive == nil || cognitive.solver == nil {
		return predictiveEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: solver is not initialized",
			nil,
		))
	}

	input := physicalVector(physical)

	if err := cognitive.solver.SetInput(0, input, nil); err != nil {
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

	// Task head: read the forward-return prediction (V·z) for this live latent
	// before any training replay perturbs the solver state.
	forecast, err := cognitive.forecast(latent)
	if err != nil {
		return predictiveEvidence{}, err
	}

	// Lagged supervision: record this settle and train the task head on any
	// samples whose adaptive-horizon forward window has now fully accrued.
	if cognitive.learn && cognitive.target != nil {
		matured := cognitive.target.Observe(symbol, input, price)

		if err := cognitive.trainTask(matured, input); err != nil {
			return predictiveEvidence{}, err
		}
	}

	return cognitive.evidence(latent, energy, surprise, forecast)
}

/*
forecast reads the supervised task head's forward-return prediction for the given
latent. Returns 0 when the head is disabled.
*/
func (cognitive *cognitiveManifold) forecast(latent []float64) (float64, error) {
	if cognitive.solver.TargetDim() <= 0 {
		return 0, nil
	}

	prediction, err := cognitive.solver.TaskPrediction(0, latent)
	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to read task prediction",
			err,
		))
	}

	if len(prediction) == 0 {
		return 0, nil
	}

	return prediction[0], nil
}

/*
trainTask replays each matured (input, forward-return) pair through the solver to
update the task head, then restores the live input so the caller's temporal state
is not corrupted by the replay. Settle is run without advancing temporal state so
the replay does not perturb the live sequence.
*/
func (cognitive *cognitiveManifold) trainTask(
	matured []maturedSample,
	liveInput []float64,
) error {
	if len(matured) == 0 {
		return nil
	}

	for _, sample := range matured {
		if err := cognitive.solver.SetInput(0, sample.input, []float64{sample.label}); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision cognitive: failed to stage task replay input",
				err,
			))
		}

		if err := cognitive.solver.Settle(false); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision cognitive: failed to settle task replay",
				err,
			))
		}

		if err := cognitive.solver.Learn(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision cognitive: failed to learn task replay",
				err,
			))
		}
	}

	// Restore the live latent so downstream reads (and the next tick's temporal
	// prior) reflect the current input, not the last replayed sample.
	if err := cognitive.solver.SetInput(0, liveInput, nil); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to restore live resonance input",
			err,
		))
	}

	if err := cognitive.solver.Settle(false); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision cognitive: failed to re-settle live resonance input",
			err,
		))
	}

	return nil
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
	forecast float64,
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
		forecast:   forecast,
		latent:     append([]float64(nil), latent...),
	}, nil
}

func latentMode(latent []float64, index int) float64 {
	if index >= len(latent) {
		return 0
	}

	return math.Abs(latent[index])
}
