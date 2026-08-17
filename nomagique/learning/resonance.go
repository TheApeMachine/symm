package learning

import (
	"errors"
	"fmt"
	"math"
	"math/rand"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

type ResonanceConfig struct {
	MaxInferenceSteps  int
	MinInferenceSteps  int
	LrState            float64
	EarlyStopTol       float64
	EarlyStopPatience  int
	MonotoneStateSteps bool
	LineSearchHalvings int

	LrGenerative  float64
	LrTemporal    float64
	LrRecognition float64

	TemporalWeight  float64
	TopDownInitMix  float64
	TemporalNormMax float64

	UsePrecision  bool
	PrecisionBeta float64
	PrecisionMin  float64
	PrecisionMax  float64
	PrecisionEps  float64

	LatentDecay float64
	Sparsity    float64
	WeightDecay float64
	GradClip    float64
	StateClip   float64
}

/*
AdaptiveResonanceConfig derives every single hyperparameter dynamically
from the system-wide learning pace (alpha) and the physical depth of the network.

Two families of parameter live in here and they do not behave the same way under
a later SetAlpha. Pace terms (the learning rates, the precision tracking weight,
the regularizer strengths) are what alpha is for, and retuning them mid-stream is
the intended effect. Geometry terms (StateClip, TopDownInitMix, the inference
step counts) describe the shape of the state space the retained weights and
precisions were estimated in; moving those mid-stream changes the problem rather
than the pace at which it is solved. SetAlpha therefore re-derives only the pace
family. See ResonanceConfig.adoptPace.
*/
func AdaptiveResonanceConfig(
	alpha float64, arch []int,
) ResonanceConfig {
	depth := len(arch)
	depthFloat := float64(depth)

	topDownInitMix := (depthFloat - 1.0) / depthFloat
	temporalNormMax := 1.0 - 1.0/depthFloat
	temporalWeight := alpha / (alpha + 1.0/depthFloat)
	earlyStopPatience := int(math.Max(1, math.Ceil(math.Sqrt(depthFloat))))
	gradClip := alpha * depthFloat

	// StateClip bounds the latent magnitude. Deriving it as depth/alpha makes a
	// faster learning pace imply a tighter state space, which is backwards and
	// couples the clip to the controller: a pace that rose by 30x would shrink
	// the admissible latent range by the same factor and clip states that the
	// retained weights were fit against. The bound belongs to the activation
	// geometry instead. Every latent below the top is a tanh image bounded by 1,
	// and the merge in initializeLatents is a convex combination of two such
	// images, so a bound of depth admits the full reachable range with room for
	// the transient excursions inference makes on the way to a settled state,
	// and is invariant to pace.
	stateClip := depthFloat

	return ResonanceConfig{
		MaxInferenceSteps:  depth * 8,
		MinInferenceSteps:  depth * 2,
		LrState:            alpha * 10.0,
		EarlyStopTol:       1e-5,
		EarlyStopPatience:  earlyStopPatience,
		MonotoneStateSteps: true,
		LineSearchHalvings: 3,

		LrGenerative:  alpha * 1.0,
		LrTemporal:    alpha * 2.0,
		LrRecognition: alpha * 0.6,

		TemporalWeight:  temporalWeight,
		TopDownInitMix:  topDownInitMix,
		TemporalNormMax: temporalNormMax,

		UsePrecision:  true,
		PrecisionBeta: alpha,
		PrecisionMin:  0.10,
		PrecisionMax:  5.0,
		PrecisionEps:  1e-4,

		LatentDecay: alpha * 1e-1,
		Sparsity:    alpha * 1e-2,
		WeightDecay: alpha * 1e-3,
		GradClip:    gradClip,
		StateClip:   stateClip,
	}
}

type ResonanceManifold struct {
	cfg                    ResonanceConfig
	arch                   []int
	targetDim              int
	generativeWeights      []*mat.Dense
	recognitionWeights     []*mat.Dense
	temporalOperator       *mat.Dense
	taskWeights            *mat.Dense
	taskBias               *mat.VecDense
	taskLearners           []*RLS
	latentStates           []*mat.VecDense
	prevTop                *mat.VecDense
	errorVar               []*mat.VecDense
	precision              []*mat.VecDense
	temporalVar            *mat.VecDense
	temporalPrecision      *mat.VecDense
	temporalPrior          *mat.VecDense
	temporalPriorReady     bool
	settleAdvancedTemporal bool
	taskVar                *mat.VecDense
	taskScale              *mat.VecDense
	taskScaleReady         []bool
	taskPrecision          *mat.VecDense
	taskModelLoss          *mat.VecDense
	taskBaselineLoss       *mat.VecDense
	taskSkillReady         []bool
	taskSkill              *mat.VecDense
	workspace              *resonanceWorkspace
	streamLearn            bool
	streamAdvanceTemporal  bool
	lastInferenceSteps     int
	output                 float64
}

func NewResonanceManifold(
	arch []int, targetDim int, alpha float64,
) *ResonanceManifold {
	if len(arch) < 2 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: architecture must contain at least input and one latent layer",
			nil,
		))

		return nil
	}

	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: alpha must be finite and in (0, 1]",
			nil,
		))

		return nil
	}

	cfg := AdaptiveResonanceConfig(alpha, arch)
	rng := rand.New(rand.NewSource(42))
	numLinks := len(arch) - 1

	weights := make([]*mat.Dense, numLinks)
	recognition := make([]*mat.Dense, numLinks)
	errorVar := make([]*mat.VecDense, numLinks)
	precision := make([]*mat.VecDense, numLinks)

	for layerIndex := range numLinks {
		rowCount, colCount := arch[layerIndex], arch[layerIndex+1]
		scaleW := math.Sqrt(2.0 / float64(rowCount+colCount))
		dataW := make([]float64, rowCount*colCount)

		for index := range dataW {
			dataW[index] = rng.NormFloat64() * scaleW
		}

		weights[layerIndex] = mat.NewDense(rowCount, colCount, dataW)
		scaleR := math.Sqrt(2.0 / float64(rowCount+colCount))
		dataR := make([]float64, colCount*rowCount)

		for index := range dataR {
			dataR[index] = rng.NormFloat64() * scaleR
		}

		recognition[layerIndex] = mat.NewDense(colCount, rowCount, dataR)
		errorVar[layerIndex] = mat.NewVecDense(rowCount, nil)
		precision[layerIndex] = mat.NewVecDense(rowCount, nil)
		denseFill(errorVar[layerIndex], 1.0)
		denseFill(precision[layerIndex], 1.0)
	}

	topDim := arch[len(arch)-1]
	scaleA := math.Sqrt(1.0 / float64(topDim))
	dataA := make([]float64, topDim*topDim)

	for index := range dataA {
		dataA[index] = rng.NormFloat64() * scaleA * 0.30
	}

	temporalWeights := mat.NewDense(topDim, topDim, dataA)

	var taskWeights *mat.Dense
	var taskBias *mat.VecDense
	var taskLearners []*RLS
	var taskVar *mat.VecDense
	var taskScale *mat.VecDense
	var taskScaleReady []bool
	var taskPrecision *mat.VecDense
	var taskModelLoss *mat.VecDense
	var taskBaselineLoss *mat.VecDense
	var taskSkillReady []bool
	var taskSkill *mat.VecDense

	if targetDim > 0 {
		// The head is linear and fits a target on the caller's own scale, so it
		// starts at zero rather than at a random draw. A random head is a
		// nonzero forecast asserted before a single sample has been seen, and
		// with the small-magnitude targets this head is built for that noise
		// can exceed the signal it will eventually learn. Zero forecasts
		// nothing until the data says otherwise, and the top latent it reads
		// from is already randomly projected, so no symmetry needs breaking
		// here.
		taskWeights = mat.NewDense(targetDim, topDim, nil)
		taskBias = mat.NewVecDense(targetDim, nil)
		taskLearners = make([]*RLS, targetDim)
		taskVar = mat.NewVecDense(targetDim, nil)
		taskScale = mat.NewVecDense(targetDim, nil)
		taskScaleReady = make([]bool, targetDim)
		taskPrecision = mat.NewVecDense(targetDim, nil)
		taskModelLoss = mat.NewVecDense(targetDim, nil)
		taskBaselineLoss = mat.NewVecDense(targetDim, nil)
		taskSkillReady = make([]bool, targetDim)
		taskSkill = mat.NewVecDense(targetDim, nil)
		denseFill(taskVar, 1.0)
		denseFill(taskPrecision, 1.0)
		denseFill(taskSkill, 1.0)

		for rowIndex := range targetDim {
			learner, err := NewRLS(RLSConfig{
				Dimension:       topDim,
				InitialVariance: 1,
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"resonance: task RLS learner construction failed - "+err.Error(),
					err,
				))
			}

			taskLearners[rowIndex] = learner
		}
	}

	latents := make([]*mat.VecDense, len(arch))

	for layerIndex, layerDim := range arch {
		latents[layerIndex] = mat.NewVecDense(layerDim, nil)
	}

	temporalVar := mat.NewVecDense(topDim, nil)
	temporalPrecision := mat.NewVecDense(topDim, nil)
	denseFill(temporalVar, 1.0)
	denseFill(temporalPrecision, 1.0)

	manifold := &ResonanceManifold{
		cfg:                   cfg,
		arch:                  arch,
		targetDim:             targetDim,
		generativeWeights:     weights,
		recognitionWeights:    recognition,
		temporalOperator:      temporalWeights,
		taskWeights:           taskWeights,
		taskBias:              taskBias,
		taskLearners:          taskLearners,
		latentStates:          latents,
		errorVar:              errorVar,
		precision:             precision,
		temporalVar:           temporalVar,
		temporalPrecision:     temporalPrecision,
		temporalPrior:         mat.NewVecDense(topDim, nil),
		taskVar:               taskVar,
		taskScale:             taskScale,
		taskScaleReady:        taskScaleReady,
		taskPrecision:         taskPrecision,
		taskModelLoss:         taskModelLoss,
		taskBaselineLoss:      taskBaselineLoss,
		taskSkillReady:        taskSkillReady,
		taskSkill:             taskSkill,
		workspace:             newResonanceWorkspace(arch, targetDim),
		streamLearn:           true,
		streamAdvanceTemporal: true,
	}

	if err := manifold.projectTemporalOperatorNorm(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: constrain initial temporal weights: "+err.Error(),
			err,
		))
	}

	return manifold
}

func (rm *ResonanceManifold) ResetState(resetPrecision bool) {
	for _, latent := range rm.latentStates {
		latent.Zero()
	}
	rm.prevTop = nil
	rm.temporalPriorReady = false
	rm.settleAdvancedTemporal = false

	if resetPrecision {
		for layerIndex := 0; layerIndex < len(rm.generativeWeights); layerIndex++ {
			denseFill(rm.errorVar[layerIndex], 1.0)
			denseFill(rm.precision[layerIndex], 1.0)
		}
		denseFill(rm.temporalVar, 1.0)
		denseFill(rm.temporalPrecision, 1.0)

		if rm.targetDim > 0 {
			denseFill(rm.taskVar, 1.0)
			rm.taskScale.Zero()
			denseFill(rm.taskPrecision, 1.0)
			rm.taskModelLoss.Zero()
			rm.taskBaselineLoss.Zero()
			denseFill(rm.taskSkill, 1.0)
			clear(rm.taskScaleReady)
			clear(rm.taskSkillReady)
		}
	}
}

func (rm *ResonanceManifold) SetStreamLearn(enabled bool) {
	rm.streamLearn = enabled
}

func (rm *ResonanceManifold) SetStreamAdvanceTemporal(enabled bool) {
	rm.streamAdvanceTemporal = enabled
}

func (rm *ResonanceManifold) SettleFromBatch(input []float64, target []float64) (float64, error) {
	return rm.SettleFromBatchOptions(input, target, rm.streamLearn, rm.streamAdvanceTemporal)
}

func (rm *ResonanceManifold) SettleFromBatchOptions(
	input []float64,
	target []float64,
	learn bool,
	advanceTemporal bool,
) (float64, error) {
	if len(input) != rm.arch[0] {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: input dimension mismatch",
			errors.New("resonance: input dimension mismatch"),
		))
	}

	settleAdvanceTemporal := advanceTemporal && !learn
	err := rm.Settle(input, settleAdvanceTemporal)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: settle failed",
			err,
		))
	}

	if learn {
		if err := rm.Learn(target); err != nil {
			return 0, err
		}
	}

	reconstruction := rm.ReconstructionError()
	rm.output = reconstruction

	return reconstruction, nil
}

func (rm *ResonanceManifold) ReconstructionOutput() float64 {
	return rm.output
}

/*
Settle performs generative inference without supervised target contamination.
Supervised targets belong in Learn and only affect weight updates.
*/
func (rm *ResonanceManifold) Settle(input []float64, advanceTemporal bool) error {
	if len(input) != rm.arch[0] {
		return errors.New("resonance: input dimension mismatch")
	}

	rm.settleAdvancedTemporal = false
	rm.lastInferenceSteps = 0

	xCol := rm.workspace.xCol
	copy(xCol.RawVector().Data, input)

	rm.initializeLatents(xCol)
	rm.temporalPriorReady = rm.prevTop != nil

	if rm.temporalPriorReady {
		rm.temporalPrior.CopyVec(rm.workspace.topPrior)
	}

	settledEnergy := rm.Energy()
	stableSteps := 0

	for step := 0; step < rm.cfg.MaxInferenceSteps; step++ {
		rm.lastInferenceSteps = step + 1
		predictions, layerErrors := rm.predictAdjacentLayers()
		gradients := rm.stateGradients(predictions, layerErrors)

		rm.saveStates()
		accepted := false
		candidateEnergy := settledEnergy
		stepSize := rm.cfg.LrState

		halvings := 0
		if rm.cfg.MonotoneStateSteps {
			halvings = rm.cfg.LineSearchHalvings
		}

		for halvingIndex := 0; halvingIndex <= halvings; halvingIndex++ {
			rm.tryStateUpdate(gradients, stepSize)
			rm.latentStates[0].CopyVec(xCol)
			candidateEnergy = rm.Energy()

			if !rm.cfg.MonotoneStateSteps ||
				candidateEnergy <= math.Nextafter(settledEnergy, math.Inf(1)) {
				accepted = true
				break
			}

			rm.restoreStates()
			stepSize *= 0.5
		}

		if !accepted {
			rm.restoreStates()
			rm.latentStates[0].CopyVec(xCol)
			stableSteps = 0

			continue
		}

		deltaEnergy := math.Abs(settledEnergy - candidateEnergy)
		energyScale := math.Max(math.Abs(settledEnergy), rm.cfg.PrecisionEps)
		relativeDelta := deltaEnergy / energyScale
		settledEnergy = candidateEnergy

		if step+1 < rm.cfg.MinInferenceSteps || relativeDelta >= rm.cfg.EarlyStopTol {
			stableSteps = 0

			continue
		}

		stableSteps++

		if stableSteps >= rm.cfg.EarlyStopPatience {
			break
		}
	}

	if advanceTemporal {
		rm.advanceTemporalState()
		rm.settleAdvancedTemporal = true
	}

	return nil
}

func (rm *ResonanceManifold) Learn(target []float64) error {
	if rm.settleAdvancedTemporal {
		return errors.New("resonance: temporal state advanced before learning")
	}

	if target != nil && len(target) != rm.targetDim {
		return fmt.Errorf(
			"resonance: target dimension mismatch: expected %d, got %d",
			rm.targetDim,
			len(target),
		)
	}

	predictions, layerErrors := rm.predictAdjacentLayers()
	topIndex := len(rm.latentStates) - 1

	var targetCol *mat.VecDense
	if target != nil && rm.targetDim > 0 {
		targetCol = rm.workspace.yCol
		copy(targetCol.RawVector().Data, target)
	}

	for layerIndex, weightMatrix := range rm.generativeWeights {
		localSignal := rm.workspace.localSignal[layerIndex]
		denseApplyOneMinusSquareInto(localSignal, predictions[layerIndex])

		precision := rm.precisionFor(layerIndex)
		localSignal.MulElemVec(localSignal, layerErrors[layerIndex])
		localSignal.MulElemVec(localSignal, precision)

		update := rm.workspace.weightUpdate[layerIndex]
		denseOuterColsInto(update, localSignal, rm.latentStates[layerIndex+1], 1.0)

		scale := rm.cfg.LrGenerative
		if norm := mat.Norm(update, 2); norm > rm.cfg.GradClip {
			scale *= rm.cfg.GradClip / norm
		}

		denseScaleInPlace(update, scale)
		weightMatrix.Add(weightMatrix, update)

		if rm.cfg.WeightDecay > 0 {
			denseScaleInPlace(
				weightMatrix,
				1.0-rm.cfg.LrGenerative*rm.cfg.WeightDecay,
			)
		}
	}

	for layerIndex, recognitionMatrix := range rm.recognitionWeights {
		proposal := rm.workspace.recProposal[layerIndex]
		proposal.MulVec(recognitionMatrix, rm.latentStates[layerIndex])
		denseApplyTanhInPlace(proposal)

		recError := rm.workspace.recError[layerIndex]
		recError.SubVec(rm.latentStates[layerIndex+1], proposal)

		recSignal := rm.workspace.recSignal[layerIndex]
		denseApplyOneMinusSquareInto(recSignal, proposal)
		recSignal.MulElemVec(recSignal, recError)

		update := rm.workspace.recUpdate[layerIndex]
		denseOuterColsInto(update, recSignal, rm.latentStates[layerIndex], 1.0)

		scale := rm.cfg.LrRecognition
		if norm := mat.Norm(update, 2); norm > rm.cfg.GradClip {
			scale *= rm.cfg.GradClip / norm
		}

		denseScaleInPlace(update, scale)
		recognitionMatrix.Add(recognitionMatrix, update)

		if rm.cfg.WeightDecay > 0 {
			denseScaleInPlace(
				recognitionMatrix,
				1.0-rm.cfg.LrRecognition*rm.cfg.WeightDecay,
			)
		}
	}

	var taskError *mat.VecDense

	if targetCol != nil && rm.taskWeights != nil {
		taskPred := rm.workspace.taskPred
		rm.taskPredictionInto(taskPred)

		taskError = rm.workspace.taskError
		taskError.SubVec(targetCol, taskPred)

		/*
			The return head is a linear regression, so square-root recursive least
			squares is the exact online learner for its objective. Its gain follows
			from retained design covariance; sharing the manifold's scalar alpha
			with unrelated generative, recognition, temporal, precision, and
			regularization updates made the task fit move for reasons outside its
			own forecast error.

			Initial covariance is the identity because the top latent is a tanh
			image on a unit scale. Forgetting is one: no evidence is discarded
			without an observed regime-reset rule.
		*/
		topState := rm.latentStates[topIndex].RawVector().Data
		targetData := targetCol.RawVector().Data
		biasData := rm.taskBias.RawVector().Data

		for rowIndex, learner := range rm.taskLearners {
			_, err := learner.Observe(RLSSample{
				Features: topState,
				Target:   targetData[rowIndex],
			})

			if err != nil {
				return fmt.Errorf("resonance: task learner update: %w", err)
			}

			intercept, err := learner.copyCoefficients(rm.taskWeights.RawRowView(rowIndex))

			if err != nil {
				return fmt.Errorf("resonance: task learner coefficients: %w", err)
			}

			biasData[rowIndex] = intercept
		}
	}

	var temporalError *mat.VecDense

	if rm.temporalPriorReady {
		temporalError = rm.workspace.temporalError
		temporalError.SubVec(rm.latentStates[topIndex], rm.temporalPrior)

		temporalSignal := rm.workspace.temporalSignal
		denseApplyOneMinusSquareInto(temporalSignal, rm.temporalPrior)

		precision := rm.temporalPrecisionVec()
		temporalSignal.MulElemVec(temporalSignal, temporalError)
		temporalSignal.MulElemVec(temporalSignal, precision)
		temporalSignal.ScaleVec(rm.cfg.TemporalWeight, temporalSignal)

		update := rm.workspace.temporalUpdate
		denseOuterColsInto(update, temporalSignal, rm.prevTop, 1.0)

		scale := rm.cfg.LrTemporal
		if norm := mat.Norm(update, 2); norm > rm.cfg.GradClip {
			scale *= rm.cfg.GradClip / norm
		}

		denseScaleInPlace(update, scale)
		rm.temporalOperator.Add(rm.temporalOperator, update)

		if rm.cfg.WeightDecay > 0 {
			denseScaleInPlace(
				rm.temporalOperator,
				1.0-rm.cfg.LrTemporal*rm.cfg.WeightDecay,
			)
		}

		if err := rm.projectTemporalOperatorNorm(); err != nil {
			return fmt.Errorf("resonance: constrain learned temporal weights: %w", err)
		}
	}

	if err := rm.updatePrecision(layerErrors, temporalError, targetCol, taskError); err != nil {
		return err
	}

	rm.advanceTemporalState()
	return nil
}

/*
Energy is the variational objective the inference line search minimizes. It is
the sum of precision-weighted prediction error and the regularizer penalties
that shape the latent state.

Do not report this as a measure of how well the network is predicting. The
regularizer terms are a function of the latent magnitudes and of alpha, not of
market surprise, so a pace change moves this number without any change in
prediction quality. PredictionEnergy isolates the part that is prediction error.
*/
func (rm *ResonanceManifold) Energy() float64 {
	energy := rm.PredictionEnergy()

	if rm.cfg.LatentDecay > 0 {
		for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
			norm := denseColNorm(rm.latentStates[layerIndex])
			energy += 0.5 * rm.cfg.LatentDecay * norm * norm
		}
	}

	if rm.cfg.Sparsity > 0 {
		for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
			energy += rm.cfg.Sparsity * floats.Norm(
				rm.latentStates[layerIndex].RawVector().Data,
				1,
			)
		}
	}

	return energy
}

/*
PredictionEnergy is the precision-weighted prediction error alone: the
generative residual at every link plus the temporal residual at the top,
excluding the latent-decay and sparsity penalties that Energy adds.

This is the quantity to report and to compare across ticks. It responds only to
how well the network predicts, so unlike Energy it does not move when the
learning pace is retuned.
*/
func (rm *ResonanceManifold) PredictionEnergy() float64 {
	_, layerErrors := rm.predictAdjacentLayers()
	energy := 0.0

	for layerIndex, layerError := range layerErrors {
		if rm.cfg.UsePrecision {
			weightedError := rm.workspace.weightedErr[layerIndex]
			weightedError.MulElemVec(rm.precisionFor(layerIndex), layerError)
			energy += 0.5 * denseColDot(weightedError, layerError)

			continue
		}

		energy += 0.5 * denseColDot(layerError, layerError)
	}

	if rm.temporalPriorReady {
		temporalError := rm.workspace.temporalError
		temporalError.SubVec(rm.latentStates[len(rm.latentStates)-1], rm.temporalPrior)

		if rm.cfg.UsePrecision {
			weightedError := rm.workspace.temporalWeightedErr
			weightedError.MulElemVec(rm.temporalPrecisionVec(), temporalError)
			energy += 0.5 * rm.cfg.TemporalWeight * denseColDot(weightedError, temporalError)
		} else {
			energy += 0.5 * rm.cfg.TemporalWeight * denseColDot(temporalError, temporalError)
		}
	}

	return energy
}

func (rm *ResonanceManifold) ReconstructionError() float64 {
	reconstruction := rm.workspace.reconPred
	reconstruction.MulVec(rm.generativeWeights[0], rm.latentStates[1])
	denseApplyTanhInPlace(reconstruction)

	diff := rm.workspace.reconDiff
	diff.SubVec(rm.latentStates[0], reconstruction)

	return denseColNorm(diff)
}

/*
taskPredictionInto evaluates the supervised head y = V * z into dst.

The head is deliberately linear, unlike every generative and recognition link in
the network, which are tanh. Those links reconstruct latent states that are
themselves bounded tanh images, so squashing them is what makes prediction and
target commensurate. The supervised target is not such a state: it is whatever
scale the caller's regression lives on. For a log return that scale is order
1e-4, which sits so deep inside tanh's linear region that the squash contributes
nothing but a systematically attenuated gradient, while capping the head at +/-1
would silently truncate any caller whose target is larger. A linear head fits the
target on its own scale and leaves saturation to the caller who chose it.
*/
func (rm *ResonanceManifold) taskPredictionInto(dst *mat.VecDense) {
	dst.MulVec(rm.taskWeights, rm.latentStates[len(rm.latentStates)-1])
	dst.AddVec(dst, rm.taskBias)
}

func (rm *ResonanceManifold) TaskPrediction() []float64 {
	if rm.taskWeights == nil || rm.targetDim <= 0 {
		return nil
	}

	taskPred := rm.workspace.taskPred
	rm.taskPredictionInto(taskPred)

	return append([]float64(nil), taskPred.RawVector().Data...)
}

/*
ObserveTask resolves one strict-prior task forecast against its later target.
Features and prediction must be the exact values retained when the forecast was
issued; the current manifold state may already describe a newer market tick.
*/
func (rm *ResonanceManifold) ObserveTask(
	features []float64,
	prediction []float64,
	target []float64,
) error {
	if rm.taskWeights == nil || rm.targetDim <= 0 {
		return errors.New("resonance: supervised task head required")
	}

	if len(features) != rm.arch[len(rm.arch)-1] {
		return fmt.Errorf(
			"resonance: expected %d task features, got %d",
			rm.arch[len(rm.arch)-1],
			len(features),
		)
	}

	if len(prediction) != rm.targetDim || len(target) != rm.targetDim {
		return fmt.Errorf(
			"resonance: expected %d task predictions and targets, got %d and %d",
			rm.targetDim,
			len(prediction),
			len(target),
		)
	}

	for index, feature := range features {
		if !finite(feature) {
			return fmt.Errorf("resonance: task feature %d must be finite", index)
		}
	}

	for index := range target {
		if !finite(prediction[index]) || !finite(target[index]) {
			return fmt.Errorf(
				"resonance: task prediction and target %d must be finite",
				index,
			)
		}
	}

	targetCol := rm.workspace.yCol
	predictionCol := rm.workspace.taskPred
	copy(targetCol.RawVector().Data, target)
	copy(predictionCol.RawVector().Data, prediction)

	taskError := rm.workspace.taskError
	taskError.SubVec(targetCol, predictionCol)
	biasData := rm.taskBias.RawVector().Data

	for rowIndex, learner := range rm.taskLearners {
		_, err := learner.Observe(RLSSample{
			Features: features,
			Target:   target[rowIndex],
		})

		if err != nil {
			return fmt.Errorf("resonance: task learner update: %w", err)
		}

		intercept, err := learner.copyCoefficients(rm.taskWeights.RawRowView(rowIndex))

		if err != nil {
			return fmt.Errorf("resonance: task learner coefficients: %w", err)
		}

		biasData[rowIndex] = intercept
	}

	return rm.updateTaskReliability(targetCol, taskError)
}

func (rm *ResonanceManifold) LatentState() []float64 {
	if len(rm.latentStates) == 0 {
		return nil
	}

	return append([]float64(nil), rm.latentStates[len(rm.latentStates)-1].RawVector().Data...)
}

/*
ResonanceLayerWire exports one settled layer for UI x-ray visualization.

Temporal reports whether ErrorNorm is a temporal mismatch rather than a
generative one. Only the top layer carries a temporal error, because it is the
only layer no other layer predicts top-down.
*/
type ResonanceLayerWire struct {
	State      []float64 `json:"state"`
	Prediction []float64 `json:"prediction"`
	ErrorNorm  float64   `json:"errorNorm"`
	Temporal   bool      `json:"temporal"`
}

/*
TemporalError reports the norm of the top layer's temporal prediction residual,
z_top - tanh(A * z_prev), and whether that residual is defined at all.

The top latent has no generative error term: layerErrors is indexed by link, of
which there are len(arch)-1, so the top layer is not predicted from above by any
weight matrix. Its prediction error is temporal, carried by A across ticks, and
it is undefined on the very first settle because no prior top state exists yet.
Callers driving a controller off this must honour the ok flag, because a zero
returned as if it were a measurement reads as perfect temporal prediction and
inverts whatever ratio it feeds.
*/
func (rm *ResonanceManifold) TemporalError() (float64, bool) {
	if !rm.temporalPriorReady {
		return 0, false
	}

	temporalError := rm.workspace.temporalError
	temporalError.SubVec(rm.latentStates[len(rm.latentStates)-1], rm.temporalPrior)

	return denseColNorm(temporalError), true
}

/*
WireSnapshot exports settled states, top-down predictions, and layer errors.
Its scalar diagnostics are the root-mean-square reconstruction error and mean
prediction energy, so architecture width cannot inflate either reading.
*/
func (rm *ResonanceManifold) WireSnapshot() (
	layers []ResonanceLayerWire,
	surprise float64,
	energy float64,
) {
	predictions, layerErrors := rm.predictAdjacentLayers()
	layers = make([]ResonanceLayerWire, len(rm.latentStates))
	topIndex := len(rm.latentStates) - 1
	temporalNorm, hasTemporal := rm.TemporalError()

	for layerIndex := range rm.latentStates {
		stateMatrix := rm.latentStates[layerIndex]
		rowCount, _ := stateMatrix.Dims()
		state := append([]float64(nil), stateMatrix.RawVector().Data...)
		prediction := make([]float64, rowCount)

		if layerIndex < len(predictions) {
			copy(prediction, predictions[layerIndex].RawVector().Data)
		}

		if layerIndex == topIndex && rm.temporalPriorReady {
			copy(prediction, rm.temporalPrior.RawVector().Data)
		}

		errorNorm := 0.0
		temporal := false

		switch {
		case layerIndex < len(layerErrors):
			errorNorm = denseColNorm(layerErrors[layerIndex])
		case layerIndex == topIndex && hasTemporal:
			// The top layer's only residual is the temporal one. Reporting it
			// here is what makes the wire's last row a measurement rather than
			// a structural zero.
			errorNorm = temporalNorm
			temporal = true
		}

		layers[layerIndex] = ResonanceLayerWire{
			State:      state,
			Prediction: prediction,
			ErrorNorm:  errorNorm,
			Temporal:   temporal,
		}
	}

	reconstructionDimensions := float64(rm.arch[0])
	predictionDimensions := 0

	for _, layerError := range layerErrors {
		predictionDimensions += layerError.Len()
	}

	if hasTemporal {
		predictionDimensions += rm.arch[topIndex]
	}

	return layers,
		rm.ReconstructionError() / math.Sqrt(reconstructionDimensions),
		rm.PredictionEnergy() / float64(predictionDimensions)
}

func (rm *ResonanceManifold) advanceTemporalState() {
	topIndex := len(rm.latentStates) - 1

	if rm.prevTop == nil {
		rm.prevTop = mat.NewVecDense(rm.arch[topIndex], nil)
	}

	rm.prevTop.CopyVec(rm.latentStates[topIndex])
}

func (rm *ResonanceManifold) precisionFor(layerIndex int) *mat.VecDense {
	return rm.precision[layerIndex]
}

func (rm *ResonanceManifold) temporalPrecisionVec() *mat.VecDense {
	return rm.temporalPrecision
}

/*
TaskPrecision reports how reliable the supervised head currently is, relative to
its own retained residual scale, and whether it has seen enough to say.

The value is scale-free by construction: one means the head is predicting at its
typical accuracy, above one means it is currently doing better than its own
history, below one means worse. That makes it the quantity a caller should use
to decide how far ahead to trust the head, because it means the same thing
whatever scale the caller's target is on and whatever the market is doing.

ok is false before any supervised sample has been resolved, when the head has no
basis for a claim at all.
*/
func (rm *ResonanceManifold) TaskPrecision() (float64, bool) {
	if rm.taskWeights == nil || rm.targetDim <= 0 {
		return 0, false
	}

	for rowIndex := range rm.targetDim {
		if !rm.taskScaleReady[rowIndex] {
			return 0, false
		}
	}

	return floats.Sum(rm.taskPrecision.RawVector().Data) /
		float64(rm.targetDim), true
}

/*
TaskSkill reports the supervised head's scale-free prequential skill against a
zero-change baseline. Values above one mean the head has lower retained squared
error than forecasting no change; values below one mean the baseline is better.
*/
func (rm *ResonanceManifold) TaskSkill() (float64, bool) {
	if rm.taskWeights == nil || rm.targetDim <= 0 {
		return 0, false
	}

	for rowIndex := range rm.targetDim {
		if !rm.taskSkillReady[rowIndex] {
			return 0, false
		}
	}

	return floats.Sum(rm.taskSkill.RawVector().Data) /
		float64(rm.targetDim), true
}

func (rm *ResonanceManifold) predictAdjacentLayers() (
	[]*mat.VecDense,
	[]*mat.VecDense,
) {
	for layerIndex := 0; layerIndex < len(rm.generativeWeights); layerIndex++ {
		prediction := rm.workspace.predictions[layerIndex]
		prediction.MulVec(rm.generativeWeights[layerIndex], rm.latentStates[layerIndex+1])
		denseApplyTanhInPlace(prediction)

		layerError := rm.workspace.errors[layerIndex]
		layerError.SubVec(rm.latentStates[layerIndex], prediction)
	}

	return rm.workspace.predictions, rm.workspace.errors
}

func (rm *ResonanceManifold) initializeLatents(xCol *mat.VecDense) {
	bottomUp := rm.workspace.bottomUp
	bottomUp[0].CopyVec(xCol)

	for layerIndex := 0; layerIndex < len(rm.recognitionWeights); layerIndex++ {
		proposal := bottomUp[layerIndex+1]
		proposal.MulVec(rm.recognitionWeights[layerIndex], bottomUp[layerIndex])
		denseApplyTanhInPlace(proposal)
	}

	rm.latentStates[0].CopyVec(xCol)

	if rm.prevTop == nil {
		for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
			rm.latentStates[layerIndex].CopyVec(bottomUp[layerIndex])
		}

		return
	}

	topPrior := rm.workspace.topPrior
	topPrior.MulVec(rm.temporalOperator, rm.prevTop)

	denseApplyTanhInPlace(topPrior)

	topDown := rm.workspace.topDown
	topDown[len(topDown)-1].CopyVec(topPrior)

	for layerIndex := len(rm.generativeWeights) - 1; layerIndex > 0; layerIndex-- {
		proposal := topDown[layerIndex]
		proposal.MulVec(rm.generativeWeights[layerIndex], topDown[layerIndex+1])
		denseApplyTanhInPlace(proposal)
	}

	initMix := rm.cfg.TopDownInitMix

	for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
		merged := rm.latentStates[layerIndex]
		merged.ScaleVec(initMix, topDown[layerIndex])
		floats.AddScaled(
			merged.RawVector().Data,
			1.0-initMix,
			bottomUp[layerIndex].RawVector().Data,
		)
		denseClipColInPlace(merged, rm.cfg.StateClip)
	}
}

func (rm *ResonanceManifold) stateGradients(
	predictions []*mat.VecDense,
	layerErrors []*mat.VecDense,
) []*mat.VecDense {
	topIndex := len(rm.latentStates) - 1

	for layerIndex := 1; layerIndex <= topIndex; layerIndex++ {
		gradient := rm.workspace.grads[layerIndex]
		gradient.Zero()

		if layerIndex < topIndex {
			if rm.cfg.UsePrecision {
				weightedError := rm.workspace.weightedErr[layerIndex]
				weightedError.MulElemVec(
					rm.precisionFor(layerIndex),
					layerErrors[layerIndex],
				)
				gradient.AddVec(gradient, weightedError)
			}

			if !rm.cfg.UsePrecision {
				gradient.AddVec(gradient, layerErrors[layerIndex])
			}
		}

		belowSignal := rm.workspace.belowSignal[layerIndex-1]
		denseApplyOneMinusSquareInto(belowSignal, predictions[layerIndex-1])

		if rm.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
			belowSignal.MulElemVec(
				belowSignal,
				rm.precisionFor(layerIndex-1),
			)
		}

		if !rm.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
		}

		correction := rm.workspace.correction[layerIndex]
		denseMulWeightTransposeInto(correction, rm.generativeWeights[layerIndex-1], belowSignal)
		gradient.SubVec(gradient, correction)

		if layerIndex == topIndex && rm.temporalPriorReady {
			temporalError := rm.workspace.temporalError
			temporalError.SubVec(rm.latentStates[topIndex], rm.temporalPrior)

			if rm.cfg.UsePrecision {
				temporalError.MulElemVec(
					temporalError,
					rm.temporalPrecisionVec(),
				)
			}

			temporalError.ScaleVec(rm.cfg.TemporalWeight, temporalError)
			gradient.AddVec(gradient, temporalError)
		}

		if rm.cfg.LatentDecay > 0 {
			floats.AddScaled(
				gradient.RawVector().Data,
				rm.cfg.LatentDecay,
				rm.latentStates[layerIndex].RawVector().Data,
			)
		}

		if rm.cfg.Sparsity > 0 {
			gradientData := gradient.RawVector().Data
			latentData := rm.latentStates[layerIndex].RawVector().Data

			for index, latentValue := range latentData {
				switch {
				case latentValue > 0:
					gradientData[index] += rm.cfg.Sparsity
				case latentValue < 0:
					gradientData[index] -= rm.cfg.Sparsity
				}
			}
		}

		gradientNorm := denseColNorm(gradient)

		if gradientNorm > rm.cfg.GradClip {
			gradient.ScaleVec(rm.cfg.GradClip/gradientNorm, gradient)
		}
	}

	return rm.workspace.grads
}

func (rm *ResonanceManifold) saveStates() {
	for layerIndex, latent := range rm.latentStates {
		rm.workspace.savedStates[layerIndex].CopyVec(latent)
	}
}

func (rm *ResonanceManifold) restoreStates() {
	for layerIndex, latent := range rm.latentStates {
		latent.CopyVec(rm.workspace.savedStates[layerIndex])
	}
}

func (rm *ResonanceManifold) tryStateUpdate(
	gradients []*mat.VecDense,
	stepSize float64,
) {
	for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
		step := rm.workspace.stepBuf[layerIndex]
		step.ScaleVec(stepSize, gradients[layerIndex])

		nextState := rm.latentStates[layerIndex]
		nextState.SubVec(rm.workspace.savedStates[layerIndex], step)
		denseClipColInPlace(nextState, rm.cfg.StateClip)
	}
}

func (rm *ResonanceManifold) updatePrecision(
	layerErrors []*mat.VecDense,
	temporalError *mat.VecDense,
	targetCol *mat.VecDense,
	taskError *mat.VecDense,
) error {
	if !rm.cfg.UsePrecision {
		return nil
	}

	beta := rm.cfg.PrecisionBeta

	for layerIndex, layerError := range layerErrors {
		variance := rm.errorVar[layerIndex]
		denseVarianceEMAInto(
			variance,
			layerError,
			beta,
			rm.cfg.PrecisionEps,
		)

		varianceData := variance.RawVector().Data

		if !(floats.Min(varianceData) > 0) ||
			!finite(floats.Norm(varianceData, 2)) {
			return errnie.Err(
				errnie.Validation,
				"resonance: precision variance must be finite and strictly positive",
				nil,
			)
		}

		densePrecisionFromVarianceInto(
			rm.precision[layerIndex],
			variance,
			rm.cfg.PrecisionMin,
			rm.cfg.PrecisionMax,
		)
	}

	if temporalError != nil {
		denseVarianceEMAInto(
			rm.temporalVar,
			temporalError,
			beta,
			rm.cfg.PrecisionEps,
		)

		temporalVarianceData := rm.temporalVar.RawVector().Data

		if !(floats.Min(temporalVarianceData) > 0) ||
			!finite(floats.Norm(temporalVarianceData, 2)) {
			return errnie.Err(
				errnie.Validation,
				"resonance: temporal precision variance must be finite and strictly positive",
				nil,
			)
		}

		densePrecisionFromVarianceInto(
			rm.temporalPrecision,
			rm.temporalVar,
			rm.cfg.PrecisionMin,
			rm.cfg.PrecisionMax,
		)
	}

	if targetCol != nil && taskError != nil && rm.taskWeights != nil {
		return rm.updateTaskReliability(targetCol, taskError)
	}

	return nil
}

func (rm *ResonanceManifold) updateTaskReliability(
	targetCol *mat.VecDense,
	taskError *mat.VecDense,
) error {
	beta := rm.cfg.PrecisionBeta
	squaredError := rm.workspace.taskSignal
	squaredError.MulElemVec(taskError, taskError)
	candidateVariance := taskError
	candidateVariance.ScaleVec(1.0-beta, rm.taskVar)

	varianceFloor := rm.workspace.taskPred
	varianceFloor.ScaleVec(beta, squaredError)
	candidateVariance.AddVec(candidateVariance, varianceFloor)

	squaredErrorData := squaredError.RawVector().Data
	candidateVarianceData := candidateVariance.RawVector().Data
	varianceFloorData := varianceFloor.RawVector().Data
	taskVarianceData := rm.taskVar.RawVector().Data
	taskScaleData := rm.taskScale.RawVector().Data

	for rowIndex, logScale := range taskScaleData {
		varianceFloorData[rowIndex] = rm.cfg.PrecisionEps * math.Exp(logScale)
	}

	for rowIndex := range taskVarianceData {
		if !rm.taskScaleReady[rowIndex] {
			if squaredErrorData[rowIndex] > 0 {
				taskVarianceData[rowIndex] = squaredErrorData[rowIndex]
			}

			continue
		}

		taskVarianceData[rowIndex] = math.Max(
			candidateVarianceData[rowIndex],
			varianceFloorData[rowIndex],
		)
	}

	for rowIndex, logScale := range taskScaleData {
		if !rm.taskScaleReady[rowIndex] {
			if squaredErrorData[rowIndex] > 0 {
				taskScaleData[rowIndex] = math.Log(squaredErrorData[rowIndex])
			}

			continue
		}

		if taskVarianceData[rowIndex] <= varianceFloorData[rowIndex] {
			continue
		}

		taskScaleData[rowIndex] = (1.0-beta)*logScale +
			beta*math.Log(taskVarianceData[rowIndex])
	}

	for rowIndex, ready := range rm.taskScaleReady {
		if !ready && squaredErrorData[rowIndex] > 0 {
			rm.taskScaleReady[rowIndex] = true
		}
	}

	if !(floats.Min(taskVarianceData) > 0) ||
		!finite(floats.Norm(taskVarianceData, 2)) {
		return errnie.Err(
			errnie.Validation,
			"resonance: task precision variance must be finite and strictly positive",
			nil,
		)
	}

	for rowIndex, logScale := range taskScaleData {
		varianceFloorData[rowIndex] = math.Exp(logScale)
	}

	rm.taskPrecision.DivElemVec(varianceFloor, rm.taskVar)
	taskPrecisionData := rm.taskPrecision.RawVector().Data

	for rowIndex, value := range taskPrecisionData {
		if !rm.taskScaleReady[rowIndex] {
			taskPrecisionData[rowIndex] = 1.0

			continue
		}

		taskPrecisionData[rowIndex] = math.Min(
			rm.cfg.PrecisionMax,
			math.Max(rm.cfg.PrecisionMin, value),
		)
	}

	rm.updateTaskSkill(targetCol, squaredError)

	return nil
}

func (rm *ResonanceManifold) updateTaskSkill(
	targetCol *mat.VecDense,
	modelSquaredError *mat.VecDense,
) {
	baselineSquaredError := rm.workspace.taskPred
	baselineSquaredError.MulElemVec(targetCol, targetCol)
	hasRetainedSkill := rm.taskSkillReady[0]

	if !hasRetainedSkill {
		rm.taskModelLoss.CopyVec(modelSquaredError)
		rm.taskBaselineLoss.CopyVec(baselineSquaredError)

		for rowIndex := range rm.taskSkillReady {
			rm.taskSkillReady[rowIndex] = true
		}
	}

	if hasRetainedSkill {
		beta := rm.cfg.PrecisionBeta
		rm.taskModelLoss.ScaleVec(1.0-beta, rm.taskModelLoss)
		modelSquaredError.ScaleVec(beta, modelSquaredError)
		rm.taskModelLoss.AddVec(rm.taskModelLoss, modelSquaredError)

		rm.taskBaselineLoss.ScaleVec(1.0-beta, rm.taskBaselineLoss)
		baselineSquaredError.ScaleVec(beta, baselineSquaredError)
		rm.taskBaselineLoss.AddVec(
			rm.taskBaselineLoss,
			baselineSquaredError,
		)
	}

	lossScale := rm.workspace.taskError
	lossScale.SubVec(rm.taskModelLoss, rm.taskBaselineLoss)
	lossScaleData := lossScale.RawVector().Data

	for rowIndex, value := range lossScaleData {
		lossScaleData[rowIndex] = math.Abs(value)
	}

	lossScale.AddVec(lossScale, rm.taskModelLoss)
	lossScale.AddVec(lossScale, rm.taskBaselineLoss)
	lossScale.ScaleVec(0.5, lossScale)

	numerator := modelSquaredError
	numerator.ScaleVec(rm.cfg.PrecisionEps, lossScale)
	numerator.AddVec(numerator, rm.taskBaselineLoss)

	denominator := baselineSquaredError
	denominator.ScaleVec(rm.cfg.PrecisionEps, lossScale)
	denominator.AddVec(denominator, rm.taskModelLoss)

	rm.taskSkill.DivElemVec(numerator, denominator)
	taskSkillData := rm.taskSkill.RawVector().Data

	for rowIndex, value := range taskSkillData {
		if lossScaleData[rowIndex] == 0 {
			taskSkillData[rowIndex] = 1.0

			continue
		}

		taskSkillData[rowIndex] = math.Min(
			rm.cfg.PrecisionMax,
			math.Max(rm.cfg.PrecisionMin, value),
		)
	}
}

/*
adoptPace copies the learning-pace family of another config over this one and
leaves the geometry family untouched.

The split exists because the retained weights, error variances and precisions
were all estimated under one state geometry. Re-deriving StateClip,
TopDownInitMix or the inference step counts mid-stream would move the state
space those estimates describe, so a pace change would silently invalidate
learned state instead of merely speeding it up or slowing it down.
*/
func (cfg *ResonanceConfig) adoptPace(pace ResonanceConfig) {
	cfg.LrState = pace.LrState
	cfg.LrGenerative = pace.LrGenerative
	cfg.LrTemporal = pace.LrTemporal
	cfg.LrRecognition = pace.LrRecognition
	cfg.PrecisionBeta = pace.PrecisionBeta

	// TemporalWeight is deliberately absent. It scales the temporal term of
	// the variational objective relative to the generative terms, so it
	// describes the shape of the energy landscape rather than the rate at
	// which the landscape is descended. Moving it with the pace would change
	// what the network is minimizing, and would make the reported prediction
	// energy move whenever the controller retuned alpha with no change in how
	// well the network predicts.
	cfg.LatentDecay = pace.LatentDecay
	cfg.Sparsity = pace.Sparsity
	cfg.WeightDecay = pace.WeightDecay
	cfg.GradClip = pace.GradClip
}

func (rm *ResonanceManifold) projectTemporalOperatorNorm() error {
	if !(rm.cfg.TemporalNormMax > 0) || rm.cfg.TemporalNormMax >= 1 {
		return errors.New("resonance: temporal operator-norm limit must be in (0, 1)")
	}

	decomposition := &rm.workspace.temporalSVD

	if ok := decomposition.Factorize(rm.temporalOperator, mat.SVDNone); !ok {
		return errors.New("resonance: temporal singular-value decomposition failed")
	}

	singularValues := decomposition.Values(rm.workspace.svdValues)

	if len(singularValues) == 0 || math.IsNaN(singularValues[0]) ||
		math.IsInf(singularValues[0], 0) {
		return errors.New("resonance: temporal operator norm must be finite")
	}

	operatorNorm := singularValues[0]

	if operatorNorm <= rm.cfg.TemporalNormMax {
		return nil
	}

	denseScaleInPlace(rm.temporalOperator, rm.cfg.TemporalNormMax/operatorNorm)

	return nil
}

/*
SetAlpha retunes the learning pace of the manifold dynamically.

Only the pace family moves. The state geometry the retained weights and
precisions were fit in stays fixed, so a controller may drive alpha across its
whole range without invalidating what the network has already learned.
*/
func (rm *ResonanceManifold) SetAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		return errors.New("resonance: alpha must be finite and in (0, 1]")
	}

	rm.cfg.adoptPace(AdaptiveResonanceConfig(alpha, rm.arch))

	return nil
}

/*
RolloutRetention reports how much of the initial latent magnitude survives at
each step of a rollout, as a fraction in (0, 1].

The temporal recursion z <- tanh(A * z) is a contraction: tanh is 1-Lipschitz
and every temporal update projects A back inside TemporalNormMax in induced
Euclidean norm, so the trajectory relaxes toward the origin and every task
reading taken along it shrinks with it.
That relaxation is genuine learned dynamics, not an artifact, but it means a
k-step curve is not k equally informative forecasts. Past the point where
retention has decayed, the curve carries the decay envelope rather than any
statement about the market, and a caller that averages or sums across the whole
curve is mostly averaging the envelope.

Retention makes that envelope explicit so callers can weight, truncate, or
simply read only as far as the dynamics still support.
*/
func (rm *ResonanceManifold) RolloutRetention(steps int) []float64 {
	if rm.temporalOperator == nil || steps < 1 {
		return nil
	}

	topDim := rm.arch[len(rm.arch)-1]
	currentState := mat.VecDenseCopyOf(rm.latentStates[len(rm.latentStates)-1])
	nextState := mat.NewVecDense(topDim, nil)

	initialNorm := denseColNorm(currentState)
	retention := make([]float64, steps)

	for step := range steps {
		if step == 0 {
			retention[step] = 1
		}

		if step > 0 && initialNorm > 0 {
			retention[step] = denseColNorm(currentState) / initialNorm
		}

		if step+1 < steps {
			nextState.MulVec(rm.temporalOperator, currentState)
			denseApplyTanhInPlace(nextState)
			currentState, nextState = nextState, currentState
		}
	}

	return retention
}

/*
RolloutTaskForecast returns the posterior predictive task distribution at every
supported step. Step zero evaluates the currently settled state because that is
the state the supervised head learned against for the next realized target. Only
later steps advance through the temporal prior.
*/
func (rm *ResonanceManifold) RolloutTaskForecast(steps int) ([]RLSOutput, error) {
	if rm.taskWeights == nil || rm.temporalOperator == nil || rm.targetDim <= 0 || steps < 1 {
		return nil, nil
	}

	topDim := rm.arch[len(rm.arch)-1]
	currentState := mat.VecDenseCopyOf(rm.latentStates[len(rm.latentStates)-1])
	nextState := mat.NewVecDense(topDim, nil)
	forecast := make([]RLSOutput, steps*rm.targetDim)

	for step := range steps {
		features := currentState.RawVector().Data

		for rowIndex, learner := range rm.taskLearners {
			output, err := learner.Predict(features)

			if err != nil {
				return nil, fmt.Errorf("resonance: task forecast: %w", err)
			}

			forecast[step*rm.targetDim+rowIndex] = output
		}

		if step+1 < steps {
			nextState.MulVec(rm.temporalOperator, currentState)
			denseApplyTanhInPlace(nextState)
			currentState, nextState = nextState, currentState
		}
	}

	return forecast, nil
}

/*
RolloutTaskAggregateForecast returns the posterior predictive distribution of
the cumulative target from t+1 through t+steps. Every rollout row shares the
same task-head coefficient posterior, so RLS.PredictSum retains their covariance.
*/
func (rm *ResonanceManifold) RolloutTaskAggregateForecast(
	steps int,
) ([]RLSOutput, error) {
	if rm.taskWeights == nil || rm.temporalOperator == nil || rm.targetDim <= 0 || steps < 1 {
		return nil, nil
	}

	topDim := rm.arch[len(rm.arch)-1]
	currentState := mat.VecDenseCopyOf(rm.latentStates[len(rm.latentStates)-1])
	nextState := mat.NewVecDense(topDim, nil)
	featureRows := make([][]float64, steps)

	for step := range steps {
		featureRows[step] = append(
			[]float64(nil),
			currentState.RawVector().Data...,
		)

		if step+1 < steps {
			nextState.MulVec(rm.temporalOperator, currentState)
			denseApplyTanhInPlace(nextState)
			currentState, nextState = nextState, currentState
		}
	}

	forecast := make([]RLSOutput, rm.targetDim)

	for rowIndex, learner := range rm.taskLearners {
		output, err := learner.PredictSum(featureRows)

		if err != nil {
			return nil, fmt.Errorf("resonance: aggregate task forecast: %w", err)
		}

		forecast[rowIndex] = output
	}

	return forecast, nil
}

/*
RolloutTaskPrediction projects the top latent state forward k steps into the future
using the temporal prior matrix A, evaluating task head V at each step.
Returns a slice of return predictions [y_t+1, y_t+2, ..., y_t+k].

The latent recursion keeps its tanh, which is what bounds the trajectory and
makes the dynamics stable. The task head does not, for the reason given on
taskPredictionInto: squashing a small-magnitude target only attenuates it.

Callers reading more than the first step should pair this with RolloutRetention,
which reports how much of the latent magnitude still survives at each step and
therefore how much of the curve is forecast rather than relaxation.
*/
func (rm *ResonanceManifold) RolloutTaskPrediction(steps int) []float64 {
	// Guard checks (including len(rm.latentStates) == 0 to prevent index-out-of-range panics)
	if rm.taskWeights == nil || rm.temporalOperator == nil || rm.targetDim <= 0 || steps < 1 || len(rm.latentStates) == 0 {
		return nil
	}

	// Pre-allocate working state buffers once
	currentState := mat.VecDenseCopyOf(rm.latentStates[len(rm.latentStates)-1])
	nextState := mat.NewVecDense(currentState.Len(), nil)
	taskPred := mat.NewVecDense(rm.targetDim, nil)

	// Cache slice header to avoid calling RawVector repeatedly in the loop.
	taskPredData := taskPred.RawVector().Data

	curve := make([]float64, steps*rm.targetDim)

	for step := range steps {
		// Predict return for current state
		taskPred.MulVec(rm.taskWeights, currentState)

		if rm.taskBias != nil {
			taskPred.AddVec(taskPred, rm.taskBias)
		}

		start := step * rm.targetDim
		copy(curve[start:start+rm.targetDim], taskPredData)

		if step+1 < steps {
			nextState.MulVec(rm.temporalOperator, currentState)
			denseApplyTanhInPlace(nextState)

			// Zero-copy swap of matrix pointers instead of copying memory
			currentState, nextState = nextState, currentState
		}
	}

	return curve
}
