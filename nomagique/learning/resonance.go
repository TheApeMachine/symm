/*
Package learning provides streaming learning primitives: recursive least
squares, an adaptive resonance manifold for hierarchical predictive coding, a
temporal ledger for delayed target supervision, and a predictive coder that
orchestrates them.

Everything is a streaming Primitive over an unsafe.Pointer wire. Multi-operation
owners (the manifold, the ledger, the coder) receive fixed-shape float rows
whose first control value selects the operation, and yield fixed-shape rows.
*/
package learning

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"math/rand"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

/*
ReadoutMode defines which representation components are harvested into the
downstream task readout and feature extraction vector.
*/
type ReadoutMode uint8

const (
	// ReadoutAll concatenates [z_1, ..., z_L, e_0, ..., e_{L-1}].
	ReadoutAll ReadoutMode = iota
	// ReadoutLatents concatenates only the settled latents [z_1, ..., z_L].
	ReadoutLatents
	// ReadoutInnovations concatenates only the prediction error residuals [e_0, ..., e_{L-1}].
	ReadoutInnovations
)

/*
resonanceConfig configures multi-timescale, overcomplete predictive coding. It
is derived from the architecture and pace by adaptiveResonanceConfig, never
hand-tuned at a call site.
*/
type resonanceConfig struct {
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

	TemporalWeights []float64 // Layer-dependent temporal weights (fast -> slow timescales)
	TopDownInitMix  float64
	TemporalNormMax float64

	UsePrecision  bool
	PrecisionBeta float64
	PrecisionMin  float64
	PrecisionMax  float64
	PrecisionEps  float64

	LatentDecay []float64 // Per-layer L2 regularization
	Sparsity    []float64 // Per-layer L1 sparsity (dictionary learning)
	WeightDecay float64
	GradClip    float64
	StateClip   float64
	LambdaRLS   float64

	ReadoutMode ReadoutMode
}

/*
adaptiveResonanceConfig derives hyperparameters dynamically from the learning
pace, physical depth, and layer dimensions, automatically configuring
dictionary sparsity when expanding overcomplete layers are detected.
*/
func adaptiveResonanceConfig(alpha float64, arch []int) resonanceConfig {
	depth := len(arch)
	depthFloat := float64(depth)
	numLatents := depth - 1

	topDownInitMix := (depthFloat - 1.0) / depthFloat
	temporalNormMax := 1.0 - 1.0/depthFloat
	earlyStopPatience := int(math.Max(1, math.Ceil(math.Sqrt(depthFloat))))
	gradClip := alpha * depthFloat
	stateClip := depthFloat

	temporalWeights := make([]float64, numLatents)
	latentDecay := make([]float64, numLatents)
	sparsity := make([]float64, numLatents)

	for latentIndex := range numLatents {
		layerIndex := latentIndex + 1
		layerDim := arch[layerIndex]
		inputDim := arch[0]

		timescaleProgression := float64(layerIndex) / depthFloat
		temporalWeights[latentIndex] = alpha * timescaleProgression / (alpha + 1.0/depthFloat)

		latentDecay[latentIndex] = alpha * 1e-1

		sparsity[latentIndex] = alpha * 1e-2

		if layerDim > inputDim {
			expansionRatio := float64(layerDim) / float64(inputDim)
			sparsity[latentIndex] = alpha * 5e-2 * math.Sqrt(expansionRatio)
		}
	}

	return resonanceConfig{
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

		TemporalWeights: temporalWeights,
		TopDownInitMix:  topDownInitMix,
		TemporalNormMax: temporalNormMax,

		UsePrecision:  true,
		PrecisionBeta: alpha,
		PrecisionMin:  0.10,
		PrecisionMax:  5.0,
		PrecisionEps:  1e-4,

		LatentDecay: latentDecay,
		Sparsity:    sparsity,
		WeightDecay: alpha * 1e-3,
		GradClip:    gradClip,
		StateClip:   stateClip,
		LambdaRLS:   0.99,
		ReadoutMode: ReadoutAll,
	}
}

/*
Manifold operations. Each arrival on a ResonanceManifold is *[3][]float64
{control, primary, secondary}; control[0] selects the operation and the rest
of control carries its scalar arguments:

	ManifoldSettle    {op, advanceTemporal}          primary = input
	ManifoldLearn     {op}                           primary = target (empty: no task target)
	ManifoldBatch     {op, learn, advanceTemporal}   primary = input, secondary = target
	ManifoldTask      {op, horizon, prediction, target} primary = readout features
	ManifoldReset     {op, precision}
	ManifoldAlpha     {op, alpha}
	ManifoldReading   {op}
	ManifoldRetention {op, steps}
	ManifoldForecast  {op, steps}
*/
const (
	ManifoldSettle = iota
	ManifoldLearn
	ManifoldBatch
	ManifoldTask
	ManifoldReset
	ManifoldAlpha
	ManifoldReading
	ManifoldRetention
	ManifoldForecast
)

/*
ResonanceManifold executes hierarchical predictive coding with multi-timescale
operators, sparse overcomplete dictionaries, and innovation feature
harvesting. It owns every recurrence and answers only through its wire.

Every operation yields *[10][]float64. Mutating operations and
ManifoldReading answer with the full settled reading; ManifoldRetention fills
only [9] and ManifoldForecast fills only [8]:

	[0] summary {reconstruction, energy, predictionEnergy, reconstructionError,
	    energyDensity, surprise, temporalError, hasTemporalError,
	    readoutDimension, skillAverage, skillReadyAvg, precisionAverage,
	    precisionReadyAvg, scaleAverage, scaleReadyAvg}
	[1] readout
	[2] latent (top layer)
	[3] task prediction, one per horizon row
	[4] skill, one per horizon row
	[5] skill ready (0/1), one per horizon row
	[6] precision ready (0/1), one per horizon row
	[7] layers, per layer {errorNorm, temporal, state..., prediction...}
	    where state and prediction are each arch[layer] wide
	[8] forecast, per horizon row {value, scale, dof, ready, innovation, reset}
	[9] retention, one per rollout step
*/
type ResonanceManifold struct {
	*core.PrimitiveError
	cfg                    resonanceConfig
	arch                   []int
	targetDim              int
	taskRows               int
	readoutDim             int
	generativeWeights      []*mat.Dense
	recognitionWeights     []*mat.Dense
	temporalOperators      []*mat.Dense
	taskWeights            *mat.Dense
	taskBias               *mat.VecDense
	taskLearners           []core.Primitive
	latentStates           []*mat.VecDense
	errorVar               []*mat.VecDense
	precision              []*mat.VecDense
	temporalVar            []*mat.VecDense
	temporalPrecision      []*mat.VecDense
	temporalPriorsReady    bool
	settleAdvancedTemporal bool

	taskVar          *mat.VecDense
	taskScale        *mat.VecDense
	taskScaleReady   []bool
	taskPrecision    *mat.VecDense
	taskModelLoss    *mat.VecDense
	taskBaselineLoss *mat.VecDense
	taskSkillReady   []bool
	taskSkill        *mat.VecDense

	workspace          *resonanceWorkspace
	lastInferenceSteps int
	output             float64
	out                [10][]float64
}

/*
NewResonanceManifold constructs a multi-layer predictive coding manifold whose
supervised task head holds one row per forward horizon, so the head can be
trained on nested cumulative targets (row h predicts the move over the next h
ticks) and harvests the named readout.

The readout width sets the size of every horizon's covariance matrix, and that
matrix is quadratic in the width. ReadoutAll concatenates latents and
innovations, so it is twice as wide as either alone and therefore four times
the memory per horizon — a real cost when the head holds hundreds of horizons.

A rejected architecture or pace is recorded as the primitive's error state and
every stream over it yields nothing.
*/
func NewResonanceManifold(
	arch []int,
	targetDim int,
	maxHorizon int,
	alpha float64,
	readout ReadoutMode,
) core.Primitive {
	if len(arch) < 2 {
		rejected := &ResonanceManifold{PrimitiveError: core.NewPrimitiveError()}
		rejected.Error(fmt.Errorf(
			"%w: resonance: architecture must contain at least input and one latent layer",
			core.ErrShape,
		))
		return rejected
	}

	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		rejected := &ResonanceManifold{PrimitiveError: core.NewPrimitiveError()}
		rejected.Error(fmt.Errorf(
			"%w: resonance: alpha must be finite and in (0, 1]",
			core.ErrDomain,
		))
		return rejected
	}

	taskRows := max(maxHorizon, 1)
	cfg := adaptiveResonanceConfig(alpha, arch)
	cfg.ReadoutMode = readout
	rng := rand.New(rand.NewSource(42))
	numLinks := len(arch) - 1
	numLatents := len(arch) - 1

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

	temporalOperators := make([]*mat.Dense, numLatents)
	temporalVar := make([]*mat.VecDense, numLatents)
	temporalPrecision := make([]*mat.VecDense, numLatents)

	for latentIndex := range numLatents {
		dim := arch[latentIndex+1]
		scaleA := math.Sqrt(1.0 / float64(dim))
		dataA := make([]float64, dim*dim)
		for index := range dataA {
			dataA[index] = rng.NormFloat64() * scaleA * 0.30
		}
		temporalOperators[latentIndex] = mat.NewDense(dim, dim, dataA)

		temporalVar[latentIndex] = mat.NewVecDense(dim, nil)
		temporalPrecision[latentIndex] = mat.NewVecDense(dim, nil)
		denseFill(temporalVar[latentIndex], 1.0)
		denseFill(temporalPrecision[latentIndex], 1.0)
	}

	latents := make([]*mat.VecDense, len(arch))
	for layerIndex, layerDim := range arch {
		latents[layerIndex] = mat.NewVecDense(layerDim, nil)
	}

	totalLatentDim := 0
	for _, dim := range arch[1:] {
		totalLatentDim += dim
	}
	totalErrorDim := 0
	for _, dim := range arch[:len(arch)-1] {
		totalErrorDim += dim
	}

	readoutDim := 0
	switch cfg.ReadoutMode {
	case ReadoutAll:
		readoutDim = totalLatentDim + totalErrorDim
	case ReadoutLatents:
		readoutDim = totalLatentDim
	case ReadoutInnovations:
		readoutDim = totalErrorDim
	}

	manifold := &ResonanceManifold{
		PrimitiveError:     core.NewPrimitiveError(),
		cfg:                cfg,
		arch:               arch,
		targetDim:          targetDim,
		taskRows:           taskRows,
		readoutDim:         readoutDim,
		generativeWeights:  weights,
		recognitionWeights: recognition,
		temporalOperators:  temporalOperators,
		latentStates:       latents,
		errorVar:           errorVar,
		precision:          precision,
		temporalVar:        temporalVar,
		temporalPrecision:  temporalPrecision,
		workspace:          newResonanceWorkspace(arch, taskRows, cfg.ReadoutMode),
	}

	if targetDim > 0 {
		manifold.taskWeights = mat.NewDense(taskRows, readoutDim, nil)
		manifold.taskBias = mat.NewVecDense(taskRows, nil)
		manifold.taskLearners = make([]core.Primitive, taskRows)
		manifold.taskVar = mat.NewVecDense(taskRows, nil)
		manifold.taskScale = mat.NewVecDense(taskRows, nil)
		manifold.taskScaleReady = make([]bool, taskRows)
		manifold.taskPrecision = mat.NewVecDense(taskRows, nil)
		manifold.taskModelLoss = mat.NewVecDense(taskRows, nil)
		manifold.taskBaselineLoss = mat.NewVecDense(taskRows, nil)
		manifold.taskSkillReady = make([]bool, taskRows)
		manifold.taskSkill = mat.NewVecDense(taskRows, nil)
		denseFill(manifold.taskVar, 1.0)
		denseFill(manifold.taskPrecision, 1.0)
		denseFill(manifold.taskSkill, 1.0)

		for rowIndex := range taskRows {
			manifold.taskLearners[rowIndex] = NewRLS(readoutDim, 1.0, cfg.LambdaRLS)
		}
	}

	if targetDim <= 0 {
		manifold.taskRows = 0
	}

	for latentIndex := range numLatents {
		if err := manifold.projectTemporalOperatorNorm(latentIndex); err != nil {
			manifold.Error(fmt.Errorf(
				"resonance: constrain initial temporal weights: %w", err,
			))
		}
	}

	return manifold
}

func (rm *ResonanceManifold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if rm.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				rm.Error(core.ErrShape)
				return
			}

			command := (*[3][]float64)(arriving)
			control, primary, secondary := command[0], command[1], command[2]

			if len(control) == 0 {
				rm.Error(fmt.Errorf("%w: resonance: command requires an operation", core.ErrShape))
				return
			}

			var args [3]float64
			copy(args[:], control[1:])
			rm.out = [10][]float64{}

			switch control[0] {
			case ManifoldSettle:
				if err := rm.settle(primary, args[0] != 0); err != nil {
					rm.Error(fmt.Errorf("resonance: settle failed: %w", err))
					return
				}
			case ManifoldLearn:
				if len(primary) == 0 {
					primary = nil
				}

				if err := rm.learn(primary); err != nil {
					rm.Error(err)
					return
				}
			case ManifoldBatch:
				if len(primary) != rm.arch[0] {
					rm.Error(fmt.Errorf("%w: resonance: input dimension mismatch", core.ErrShape))
					return
				}

				learn := args[0] != 0

				if err := rm.settle(primary, args[1] != 0 && !learn); err != nil {
					rm.Error(fmt.Errorf("resonance: settle failed: %w", err))
					return
				}

				if len(secondary) == 0 {
					secondary = nil
				}

				if learn {
					if err := rm.learn(secondary); err != nil {
						rm.Error(err)
						return
					}
				}

				rm.output = rm.reconstructionError()
			case ManifoldTask:
				if err := rm.observeTask(int(args[0]), primary, args[1], args[2]); err != nil {
					rm.Error(err)
					return
				}
			case ManifoldReset:
				rm.resetState(args[0] != 0)
			case ManifoldAlpha:
				if err := rm.setAlpha(args[0]); err != nil {
					rm.Error(err)
					return
				}
			case ManifoldReading:
			case ManifoldRetention, ManifoldForecast:
				steps := int(args[0])

				if steps < 1 {
					rm.Error(fmt.Errorf(
						"%w: resonance: rollout requires a positive step count",
						core.ErrDomain,
					))
					return
				}

				if control[0] == ManifoldRetention {
					rm.out[9] = rm.rolloutRetention(steps)
				}

				if control[0] == ManifoldForecast {
					forecast, err := rm.rolloutTaskForecast(steps)

					if err != nil {
						rm.Error(err)
						return
					}

					rm.out[8] = forecast
				}

				if !yield(unsafe.Pointer(&rm.out)) {
					return
				}

				continue
			default:
				rm.Error(fmt.Errorf("%w: resonance: unknown operation %v", core.ErrShape, control[0]))
				return
			}

			summary := make([]float64, 15)
			summary[0] = rm.output
			summary[3] = rm.reconstructionError()
			summary[8] = float64(rm.readoutDim)
			rm.out[1] = rm.readoutVector()
			rm.out[2] = rm.latentState()
			rm.out[3] = rm.taskPrediction()
			summary[1] = rm.energy()
			summary[2] = rm.predictionEnergy()
			temporalNorm, hasTemporal := rm.temporalError()
			summary[6] = temporalNorm

			if hasTemporal {
				summary[7] = 1
			}

			predictions, layerErrors := rm.predictAdjacentLayers()
			topIndex := len(rm.latentStates) - 1
			layers := make([]float64, 0)

			for layerIndex, stateMatrix := range rm.latentStates {
				rowCount, _ := stateMatrix.Dims()
				errorNorm, temporal := 0.0, 0.0

				switch {
				case layerIndex < len(layerErrors):
					errorNorm = denseColNorm(layerErrors[layerIndex])
				case layerIndex == topIndex && hasTemporal:
					errorNorm = temporalNorm
					temporal = 1
				}

				layers = append(layers, errorNorm, temporal)
				layers = append(layers, stateMatrix.RawVector().Data...)
				prediction := make([]float64, rowCount)

				if layerIndex < len(predictions) {
					copy(prediction, predictions[layerIndex].RawVector().Data)
				}

				if layerIndex == topIndex && rm.temporalPriorsReady {
					copy(prediction, rm.workspace.temporalPriors[len(rm.temporalOperators)-1].RawVector().Data)
				}

				layers = append(layers, prediction...)
			}

			rm.out[7] = layers
			predictionDimensions := 0

			for _, layerError := range layerErrors {
				predictionDimensions += layerError.Len()
			}

			if hasTemporal {
				predictionDimensions += rm.arch[topIndex]
			}

			summary[5] = rm.reconstructionError() / math.Sqrt(float64(rm.arch[0]))
			summary[4] = rm.predictionEnergy() / float64(predictionDimensions)

			if rm.taskRows > 0 {
				rm.out[4] = append([]float64(nil), rm.taskSkill.RawVector().Data...)
				rm.out[5] = make([]float64, rm.taskRows)
				rm.out[6] = make([]float64, rm.taskRows)

				for rowIndex, ready := range rm.taskScaleReady {
					if ready {
						rm.out[5][rowIndex] = 1
						rm.out[6][rowIndex] = 1
					}
				}

				readiness := [3][]bool{rm.taskSkillReady, rm.taskScaleReady, rm.taskScaleReady}

				for slot, values := range [3]*mat.VecDense{rm.taskSkill, rm.taskPrecision, rm.taskScale} {
					sum, readyCount := 0.0, 0.0

					for rowIndex := range rm.taskRows {
						if readiness[slot][rowIndex] {
							sum += values.RawVector().Data[rowIndex]
							readyCount++
						}
					}

					if readyCount > 0 {
						summary[9+slot*2] = sum / readyCount
						summary[10+slot*2] = 1
					}
				}
			}

			rm.out[0] = summary

			if !yield(unsafe.Pointer(&rm.out)) {
				return
			}
		}
	}
}

func (rm *ResonanceManifold) resetState(resetPrecision bool) {
	for _, latent := range rm.latentStates {
		latent.Zero()
	}
	for latentIndex := range rm.temporalOperators {
		rm.workspace.prevLatents[latentIndex].Zero()
	}
	rm.temporalPriorsReady = false
	rm.settleAdvancedTemporal = false

	if resetPrecision {
		for layerIndex := 0; layerIndex < len(rm.generativeWeights); layerIndex++ {
			denseFill(rm.errorVar[layerIndex], 1.0)
			denseFill(rm.precision[layerIndex], 1.0)
		}
		for latentIndex := range rm.temporalOperators {
			denseFill(rm.temporalVar[latentIndex], 1.0)
			denseFill(rm.temporalPrecision[latentIndex], 1.0)
		}

		if rm.taskRows > 0 {
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

/*
settle performs generative inference by minimizing precision-weighted prediction
error, multi-timescale temporal priors, and overcomplete dictionary sparsity.
*/
func (rm *ResonanceManifold) settle(input []float64, advanceTemporal bool) error {
	if len(input) != rm.arch[0] {
		return fmt.Errorf(
			"%w: resonance: input dimension mismatch",
			core.ErrShape,
		)
	}

	rm.settleAdvancedTemporal = false
	rm.lastInferenceSteps = 0

	xCol := rm.workspace.xCol
	copy(xCol.RawVector().Data, input)

	rm.initializeLatents(xCol)

	settledEnergy := rm.energy()
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
			candidateEnergy = rm.energy()

			if !rm.cfg.MonotoneStateSteps || candidateEnergy <= math.Nextafter(settledEnergy, math.Inf(1)) {
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

/*
learn updates generative, recognition, multi-timescale temporal matrices, and
the downstream multi-layer task head via RLS.
*/
func (rm *ResonanceManifold) learn(target []float64) error {
	if rm.settleAdvancedTemporal {
		return fmt.Errorf(
			"%w: resonance: temporal state advanced before learning",
			core.ErrDomain,
		)
	}

	if target != nil && len(target) != rm.targetDim {
		return fmt.Errorf(
			"%w: resonance: target dimension mismatch: expected %d, got %d",
			core.ErrShape,
			rm.targetDim,
			len(target),
		)
	}

	predictions, layerErrors := rm.predictAdjacentLayers()

	// 1. Generative weights update
	for layerIndex, weightMatrix := range rm.generativeWeights {
		localSignal := rm.workspace.localSignal[layerIndex]
		if layerIndex == 0 {
			for i := 0; i < localSignal.Len(); i++ {
				localSignal.SetVec(i, 1.0)
			}
		}
		if layerIndex > 0 {
			denseApplyOneMinusSquareInto(localSignal, predictions[layerIndex])
		}
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
			denseScaleInPlace(weightMatrix, 1.0-rm.cfg.LrGenerative*rm.cfg.WeightDecay)
		}
	}

	// 2. Recognition weights update
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
			denseScaleInPlace(recognitionMatrix, 1.0-rm.cfg.LrRecognition*rm.cfg.WeightDecay)
		}
	}

	// 3. Multi-timescale temporal operators update across all latent layers
	temporalErrors := make([]*mat.VecDense, len(rm.temporalOperators))
	if rm.temporalPriorsReady {
		for latentIndex, operator := range rm.temporalOperators {
			layerIndex := latentIndex + 1
			temporalError := rm.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(rm.latentStates[layerIndex], rm.workspace.temporalPriors[latentIndex])
			temporalErrors[latentIndex] = temporalError

			temporalSignal := rm.workspace.temporalSignals[latentIndex]
			denseApplyOneMinusSquareInto(temporalSignal, rm.workspace.temporalPriors[latentIndex])
			precision := rm.temporalPrecision[latentIndex]
			temporalSignal.MulElemVec(temporalSignal, temporalError)
			temporalSignal.MulElemVec(temporalSignal, precision)
			temporalSignal.ScaleVec(rm.cfg.TemporalWeights[latentIndex], temporalSignal)

			update := rm.workspace.temporalUpdates[latentIndex]
			denseOuterColsInto(update, temporalSignal, rm.workspace.prevLatents[latentIndex], 1.0)

			scale := rm.cfg.LrTemporal
			if norm := mat.Norm(update, 2); norm > rm.cfg.GradClip {
				scale *= rm.cfg.GradClip / norm
			}

			denseScaleInPlace(update, scale)
			operator.Add(operator, update)

			if rm.cfg.WeightDecay > 0 {
				denseScaleInPlace(operator, 1.0-rm.cfg.LrTemporal*rm.cfg.WeightDecay)
			}

			if err := rm.projectTemporalOperatorNorm(latentIndex); err != nil {
				return fmt.Errorf("resonance: constrain temporal operator %d: %w", latentIndex, err)
			}
		}
	}

	// 4. Multi-layer & innovation task head update (RLS)
	var targetCol *mat.VecDense
	var taskError *mat.VecDense
	trainedRows := 0

	if target != nil && rm.taskWeights != nil {
		trainedRows = min(len(target), rm.taskRows)

		targetCol = rm.workspace.yCol
		copy(targetCol.RawVector().Data, target)

		taskPred := rm.workspace.taskPred
		rm.taskPredictionInto(taskPred)

		taskError = rm.workspace.taskError
		taskError.SubVec(targetCol, taskPred)

		readoutData := rm.workspace.readoutBuf.RawVector().Data
		rm.readoutVectorInto(readoutData)
		targetData := targetCol.RawVector().Data
		biasData := rm.taskBias.RawVector().Data

		for rowIndex := range trainedRows {
			reading, err := rm.taskReading(rowIndex, readoutData, targetData[rowIndex])

			if err != nil {
				return fmt.Errorf("resonance: task learner update: %w", err)
			}

			if len(reading) < 2 || len(reading[1]) != rm.readoutDim+1 {
				return fmt.Errorf(
					"resonance: task learner coefficients do not match head %d",
					rm.readoutDim,
				)
			}

			copy(rm.taskWeights.RawRowView(rowIndex), reading[1][1:])
			biasData[rowIndex] = reading[1][0]
		}
	}

	if err := rm.updatePrecision(layerErrors, temporalErrors, targetCol, taskError, trainedRows); err != nil {
		return err
	}

	rm.advanceTemporalState()
	return nil
}

/*
energy is the variational free energy combining precision-weighted error,
multi-timescale temporal priors, $L_2$ decay, and $L_1$ dictionary sparsity.
*/
func (rm *ResonanceManifold) energy() float64 {
	energy := rm.predictionEnergy()

	for latentIndex := range rm.temporalOperators {
		layerIndex := latentIndex + 1
		latent := rm.latentStates[layerIndex]
		if rm.cfg.LatentDecay[latentIndex] > 0 {
			norm := denseColNorm(latent)
			energy += 0.5 * rm.cfg.LatentDecay[latentIndex] * norm * norm
		}
		if rm.cfg.Sparsity[latentIndex] > 0 {
			energy += rm.cfg.Sparsity[latentIndex] * floats.Norm(latent.RawVector().Data, 1)
		}
	}

	return energy
}

/*
predictionEnergy computes total precision-weighted prediction error across all
generative links and multi-timescale temporal links.
*/
func (rm *ResonanceManifold) predictionEnergy() float64 {
	_, layerErrors := rm.predictAdjacentLayers()
	energy := 0.0

	for layerIndex, layerError := range layerErrors {
		if !rm.cfg.UsePrecision {
			energy += 0.5 * denseColDot(layerError, layerError)
			continue
		}

		weightedError := rm.workspace.weightedErr[layerIndex]
		weightedError.MulElemVec(rm.precisionFor(layerIndex), layerError)
		energy += 0.5 * denseColDot(weightedError, layerError)
	}

	if rm.temporalPriorsReady {
		for latentIndex := range rm.temporalOperators {
			layerIndex := latentIndex + 1
			temporalError := rm.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(rm.latentStates[layerIndex], rm.workspace.temporalPriors[latentIndex])

			weight := rm.cfg.TemporalWeights[latentIndex]

			if !rm.cfg.UsePrecision {
				energy += 0.5 * weight * denseColDot(temporalError, temporalError)
				continue
			}

			weightedError := rm.workspace.temporalWeightedErrs[latentIndex]
			weightedError.MulElemVec(rm.temporalPrecision[latentIndex], temporalError)
			energy += 0.5 * weight * denseColDot(weightedError, temporalError)
		}
	}

	return energy
}

func (rm *ResonanceManifold) reconstructionError() float64 {
	reconstruction := rm.workspace.reconPred
	reconstruction.MulVec(rm.generativeWeights[0], rm.latentStates[1])
	// Layer 0 is linear (continuous unbounded z-scores)

	diff := rm.workspace.reconDiff
	diff.SubVec(rm.latentStates[0], reconstruction)

	return denseColNorm(diff)
}

func (rm *ResonanceManifold) taskPredictionInto(dst *mat.VecDense) {
	readoutData := rm.workspace.readoutBuf.RawVector().Data
	rm.readoutVectorInto(readoutData)

	dst.MulVec(rm.taskWeights, rm.workspace.readoutBuf)
	dst.AddVec(dst, rm.taskBias)
}

func (rm *ResonanceManifold) taskPrediction() []float64 {
	if rm.taskWeights == nil || rm.taskRows <= 0 {
		return nil
	}

	taskPred := rm.workspace.taskPred
	rm.taskPredictionInto(taskPred)
	return append([]float64(nil), taskPred.RawVector().Data...)
}

/*
taskReading drives one task-head row's learner with one labeled sample and
returns its posterior reading.
*/
func (rm *ResonanceManifold) taskReading(
	rowIndex int,
	features []float64,
	target float64,
) ([][]float64, error) {
	learner := rm.taskLearners[rowIndex]
	row := append(append(make([]float64, 0, len(features)+1), features...), target)
	var reading [][]float64

	for out := range learner.Next(data.NewValue(row).Next(nil)) {
		reading = *(*[][]float64)(out)
	}

	return reading, learner.Error()
}

/*
observeTask updates one task-head row from one labeled sample. The row is
addressed by its forward horizon, one-based: horizon h supervises the
cumulative move over the next h ticks.
*/
func (rm *ResonanceManifold) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
) error {
	if rm.taskWeights == nil || rm.taskRows <= 0 {
		return fmt.Errorf(
			"%w: resonance: supervised task head required",
			core.ErrShape,
		)
	}

	if horizon < 1 || horizon > rm.taskRows {
		return fmt.Errorf(
			"%w: resonance: task horizon %d out of range [1, %d]",
			core.ErrDomain,
			horizon,
			rm.taskRows,
		)
	}

	if len(features) != rm.readoutDim {
		return fmt.Errorf(
			"%w: resonance: expected %d task features, got %d",
			core.ErrShape,
			rm.readoutDim,
			len(features),
		)
	}

	for index, feature := range features {
		if math.IsNaN(feature) || math.IsInf(feature, 0) {
			return fmt.Errorf(
				"%w: resonance: task feature %d must be finite",
				core.ErrDomain,
				index,
			)
		}
	}

	if math.IsNaN(prediction) || math.IsInf(prediction, 0) ||
		math.IsNaN(target) || math.IsInf(target, 0) {
		return fmt.Errorf(
			"%w: resonance: task prediction and target must be finite",
			core.ErrDomain,
		)
	}

	rowIndex := horizon - 1

	reading, err := rm.taskReading(rowIndex, features, target)

	if err != nil {
		return fmt.Errorf("resonance: task learner update: %w", err)
	}

	if len(reading) < 2 || len(reading[1]) != rm.readoutDim+1 {
		return fmt.Errorf(
			"resonance: task learner coefficients do not match head %d",
			rm.readoutDim,
		)
	}

	copy(rm.taskWeights.RawRowView(rowIndex), reading[1][1:])
	rm.taskBias.RawVector().Data[rowIndex] = reading[1][0]

	rm.updateTaskReliability(rowIndex, target, target-prediction)

	return nil
}

func (rm *ResonanceManifold) latentState() []float64 {
	if len(rm.latentStates) == 0 {
		return nil
	}

	return append([]float64(nil), rm.latentStates[len(rm.latentStates)-1].RawVector().Data...)
}

func (rm *ResonanceManifold) temporalError() (float64, bool) {
	if !rm.temporalPriorsReady || len(rm.temporalOperators) == 0 {
		return 0, false
	}

	topLatentIdx := len(rm.temporalOperators) - 1
	temporalError := rm.workspace.temporalErrors[topLatentIdx]
	return denseColNorm(temporalError), true
}

func (rm *ResonanceManifold) stateGradients(
	predictions []*mat.VecDense,
	layerErrors []*mat.VecDense,
) []*mat.VecDense {
	topIndex := len(rm.latentStates) - 1

	for layerIndex := 1; layerIndex <= topIndex; layerIndex++ {
		gradient := rm.workspace.grads[layerIndex]
		gradient.Zero()
		latentIndex := layerIndex - 1

		if layerIndex < topIndex && !rm.cfg.UsePrecision {
			gradient.AddVec(gradient, layerErrors[layerIndex])
		}

		if layerIndex < topIndex && rm.cfg.UsePrecision {
			weightedError := rm.workspace.weightedErr[layerIndex]
			weightedError.MulElemVec(rm.precisionFor(layerIndex), layerErrors[layerIndex])
			gradient.AddVec(gradient, weightedError)
		}

		belowSignal := rm.workspace.belowSignal[layerIndex-1]
		if layerIndex-1 == 0 {
			for i := 0; i < belowSignal.Len(); i++ {
				belowSignal.SetVec(i, 1.0)
			}
		}
		if layerIndex-1 > 0 {
			denseApplyOneMinusSquareInto(belowSignal, predictions[layerIndex-1])
		}
		if rm.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
			belowSignal.MulElemVec(belowSignal, rm.precisionFor(layerIndex-1))
		}
		if !rm.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
		}

		correction := rm.workspace.correction[layerIndex]
		denseMulWeightTransposeInto(correction, rm.generativeWeights[layerIndex-1], belowSignal)
		gradient.SubVec(gradient, correction)

		if rm.temporalPriorsReady {
			temporalError := rm.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(rm.latentStates[layerIndex], rm.workspace.temporalPriors[latentIndex])

			if rm.cfg.UsePrecision {
				temporalError.MulElemVec(temporalError, rm.temporalPrecision[latentIndex])
			}

			temporalError.ScaleVec(rm.cfg.TemporalWeights[latentIndex], temporalError)
			gradient.AddVec(gradient, temporalError)
		}

		if rm.cfg.LatentDecay[latentIndex] > 0 {
			floats.AddScaled(
				gradient.RawVector().Data,
				rm.cfg.LatentDecay[latentIndex],
				rm.latentStates[layerIndex].RawVector().Data,
			)
		}

		if rm.cfg.Sparsity[latentIndex] > 0 {
			gradientData := gradient.RawVector().Data
			latentData := rm.latentStates[layerIndex].RawVector().Data
			s := rm.cfg.Sparsity[latentIndex]

			for index, val := range latentData {
				if val > 0 {
					gradientData[index] += s
				}

				if val < 0 {
					gradientData[index] -= s
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

func (rm *ResonanceManifold) initializeLatents(xCol *mat.VecDense) {
	bottomUp := rm.workspace.bottomUp
	bottomUp[0].CopyVec(xCol)

	for layerIndex := 0; layerIndex < len(rm.recognitionWeights); layerIndex++ {
		proposal := bottomUp[layerIndex+1]
		proposal.MulVec(rm.recognitionWeights[layerIndex], bottomUp[layerIndex])
		denseApplyTanhInPlace(proposal)
	}

	rm.latentStates[0].CopyVec(xCol)

	if !rm.temporalPriorsReady {
		for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
			rm.latentStates[layerIndex].CopyVec(bottomUp[layerIndex])
		}
		return
	}

	topDown := rm.workspace.topDown
	for latentIndex, operator := range rm.temporalOperators {
		prior := rm.workspace.temporalPriors[latentIndex]
		prior.MulVec(operator, rm.workspace.prevLatents[latentIndex])
		denseApplyTanhInPlace(prior)
		topDown[latentIndex+1].CopyVec(prior)
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

func (rm *ResonanceManifold) advanceTemporalState() {
	for latentIndex := range rm.temporalOperators {
		layerIndex := latentIndex + 1
		rm.workspace.prevLatents[latentIndex].CopyVec(rm.latentStates[layerIndex])
	}
	rm.temporalPriorsReady = true
}

func (rm *ResonanceManifold) precisionFor(layerIndex int) *mat.VecDense {
	return rm.precision[layerIndex]
}

func (rm *ResonanceManifold) projectTemporalOperatorNorm(latentIndex int) error {
	if !(rm.cfg.TemporalNormMax > 0) || rm.cfg.TemporalNormMax >= 1 {
		return errors.New("resonance: temporal operator-norm limit must be in (0, 1)")
	}

	decomposition := &rm.workspace.temporalSVDs[latentIndex]
	operator := rm.temporalOperators[latentIndex]

	if ok := decomposition.Factorize(operator, mat.SVDNone); !ok {
		return errors.New("resonance: temporal singular-value decomposition failed")
	}

	singularValues := decomposition.Values(rm.workspace.layerSVDValues[latentIndex])
	if len(singularValues) == 0 || math.IsNaN(singularValues[0]) || math.IsInf(singularValues[0], 0) {
		return errors.New("resonance: temporal operator norm must be finite")
	}

	operatorNorm := singularValues[0]
	if operatorNorm <= rm.cfg.TemporalNormMax {
		return nil
	}

	denseScaleInPlace(operator, rm.cfg.TemporalNormMax/operatorNorm)
	return nil
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

func (rm *ResonanceManifold) tryStateUpdate(gradients []*mat.VecDense, stepSize float64) {
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
	temporalErrors []*mat.VecDense,
	targetCol *mat.VecDense,
	taskError *mat.VecDense,
	trainedRows int,
) error {
	if !rm.cfg.UsePrecision {
		return nil
	}

	beta := rm.cfg.PrecisionBeta

	for layerIndex, layerError := range layerErrors {
		variance := rm.errorVar[layerIndex]
		denseVarianceEMAInto(variance, layerError, beta, rm.cfg.PrecisionEps)
		densePrecisionFromVarianceInto(rm.precision[layerIndex], variance, rm.cfg.PrecisionMin, rm.cfg.PrecisionMax)
	}

	for latentIndex, tempErr := range temporalErrors {
		if tempErr == nil {
			continue
		}
		variance := rm.temporalVar[latentIndex]
		denseVarianceEMAInto(variance, tempErr, beta, rm.cfg.PrecisionEps)
		densePrecisionFromVarianceInto(rm.temporalPrecision[latentIndex], variance, rm.cfg.PrecisionMin, rm.cfg.PrecisionMax)
	}

	if targetCol != nil && taskError != nil && rm.taskWeights != nil {
		targetData := targetCol.RawVector().Data
		errorData := taskError.RawVector().Data

		for rowIndex := range trainedRows {
			rm.updateTaskReliability(rowIndex, targetData[rowIndex], errorData[rowIndex])
		}
	}

	return nil
}

/*
updateTaskReliability folds one resolved sample into one task row's variance,
scale, precision, and skill readouts. Each row owns its moments, so the nested
multi-horizon head scores each horizon against its own prediction error.
*/
func (rm *ResonanceManifold) updateTaskReliability(
	rowIndex int,
	target float64,
	taskError float64,
) {
	beta := rm.cfg.PrecisionBeta
	squaredError := taskError * taskError
	taskVarianceData := rm.taskVar.RawVector().Data
	taskScaleData := rm.taskScale.RawVector().Data
	taskPrecisionData := rm.taskPrecision.RawVector().Data

	if !rm.taskScaleReady[rowIndex] && squaredError > 0 {
		taskVarianceData[rowIndex] = squaredError
		taskScaleData[rowIndex] = math.Log(squaredError)
	}

	if rm.taskScaleReady[rowIndex] {
		candidateVariance := (1.0-beta)*taskVarianceData[rowIndex] + beta*squaredError
		varianceFloor := rm.cfg.PrecisionEps * math.Exp(taskScaleData[rowIndex])
		taskVarianceData[rowIndex] = math.Max(candidateVariance, varianceFloor)

		if taskVarianceData[rowIndex] > varianceFloor {
			taskScaleData[rowIndex] = (1.0-beta)*taskScaleData[rowIndex] +
				beta*math.Log(taskVarianceData[rowIndex])
		}
	}

	if !rm.taskScaleReady[rowIndex] && squaredError > 0 {
		rm.taskScaleReady[rowIndex] = true
	}

	varianceFloor := math.Exp(taskScaleData[rowIndex])
	taskPrecisionData[rowIndex] = 1.0

	if rm.taskScaleReady[rowIndex] {
		value := varianceFloor / taskVarianceData[rowIndex]
		taskPrecisionData[rowIndex] = math.Min(
			rm.cfg.PrecisionMax,
			math.Max(rm.cfg.PrecisionMin, value),
		)
	}

	rm.updateTaskSkill(rowIndex, target, squaredError)
}

/*
updateTaskSkill maintains one row's exponential moving average of model loss
versus the zero-prediction baseline. Skill above one means the row's forecasts
beat predicting no move, which is the evidence the horizon selector contracts on.
*/
func (rm *ResonanceManifold) updateTaskSkill(
	rowIndex int,
	target float64,
	modelSquaredError float64,
) {
	baselineSquaredError := target * target
	taskModelLossData := rm.taskModelLoss.RawVector().Data
	taskBaselineLossData := rm.taskBaselineLoss.RawVector().Data

	if !rm.taskSkillReady[rowIndex] {
		taskModelLossData[rowIndex] = modelSquaredError
		taskBaselineLossData[rowIndex] = baselineSquaredError
		rm.taskSkillReady[rowIndex] = true

		return
	}

	beta := rm.cfg.PrecisionBeta
	taskModelLossData[rowIndex] = (1.0-beta)*taskModelLossData[rowIndex] +
		beta*modelSquaredError
	taskBaselineLossData[rowIndex] = (1.0-beta)*taskBaselineLossData[rowIndex] +
		beta*baselineSquaredError

	modelLoss := taskModelLossData[rowIndex]
	baselineLoss := taskBaselineLossData[rowIndex]
	lossScale := math.Abs(modelLoss-baselineLoss) + modelLoss + baselineLoss
	lossScale *= 0.5

	skill := 1.0

	if lossScale > 0 {
		numerator := rm.cfg.PrecisionEps*lossScale + baselineLoss
		denominator := rm.cfg.PrecisionEps*lossScale + modelLoss
		skill = math.Min(
			rm.cfg.PrecisionMax,
			math.Max(rm.cfg.PrecisionMin, numerator/denominator),
		)
	}

	rm.taskSkill.RawVector().Data[rowIndex] = skill
}

func (rm *ResonanceManifold) predictAdjacentLayers() ([]*mat.VecDense, []*mat.VecDense) {
	for layerIndex := 0; layerIndex < len(rm.generativeWeights); layerIndex++ {
		prediction := rm.workspace.predictions[layerIndex]
		prediction.MulVec(rm.generativeWeights[layerIndex], rm.latentStates[layerIndex+1])
		if layerIndex > 0 {
			denseApplyTanhInPlace(prediction)
		}

		layerError := rm.workspace.errors[layerIndex]
		layerError.SubVec(rm.latentStates[layerIndex], prediction)
	}

	return rm.workspace.predictions, rm.workspace.errors
}

func (rm *ResonanceManifold) setAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		return fmt.Errorf(
			"%w: resonance: alpha must be finite and in (0, 1]",
			core.ErrDomain,
		)
	}

	newCfg := adaptiveResonanceConfig(alpha, rm.arch)
	rm.cfg.LrState = newCfg.LrState
	rm.cfg.LrGenerative = newCfg.LrGenerative
	rm.cfg.LrTemporal = newCfg.LrTemporal
	rm.cfg.LrRecognition = newCfg.LrRecognition
	rm.cfg.PrecisionBeta = newCfg.PrecisionBeta
	rm.cfg.LatentDecay = newCfg.LatentDecay
	rm.cfg.Sparsity = newCfg.Sparsity
	rm.cfg.WeightDecay = newCfg.WeightDecay
	rm.cfg.GradClip = newCfg.GradClip

	return nil
}

/*
readoutVectorInto writes the multi-layer readout [z_1..z_L, e_0..e_{L-1}]
directly into dst without heap allocation.
*/
func (rm *ResonanceManifold) readoutVectorInto(dst []float64) int {
	_, layerErrors := rm.predictAdjacentLayers()
	offset := 0

	if rm.cfg.ReadoutMode == ReadoutAll || rm.cfg.ReadoutMode == ReadoutLatents {
		for layerIndex := 1; layerIndex < len(rm.latentStates); layerIndex++ {
			data := rm.latentStates[layerIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	if rm.cfg.ReadoutMode == ReadoutAll || rm.cfg.ReadoutMode == ReadoutInnovations {
		for linkIndex := range layerErrors {
			data := layerErrors[linkIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	return offset
}

func (rm *ResonanceManifold) readoutVector() []float64 {
	vector := make([]float64, rm.readoutDim)
	rm.readoutVectorInto(vector)
	return vector
}

func (rm *ResonanceManifold) rolloutRetention(steps int) []float64 {
	if len(rm.temporalOperators) == 0 || steps < 1 {
		return nil
	}

	numLatents := len(rm.temporalOperators)
	currentLatents := make([]*mat.VecDense, numLatents)
	nextLatents := make([]*mat.VecDense, numLatents)
	for i := range numLatents {
		currentLatents[i] = mat.VecDenseCopyOf(rm.latentStates[i+1])
		nextLatents[i] = mat.NewVecDense(rm.arch[i+1], nil)
	}

	initialNormSq := 0.0
	for i := range numLatents {
		norm := denseColNorm(currentLatents[i])
		initialNormSq += norm * norm
	}
	initialNorm := math.Sqrt(initialNormSq)

	retention := make([]float64, steps)

	for step := range steps {
		if step == 0 || initialNorm == 0 {
			retention[step] = 1.0
			continue
		}

		normSq := 0.0
		for i := range numLatents {
			norm := denseColNorm(currentLatents[i])
			normSq += norm * norm
		}
		retention[step] = math.Sqrt(normSq) / initialNorm

		if step+1 < steps {
			for i := range numLatents {
				nextLatents[i].MulVec(rm.temporalOperators[i], currentLatents[i])
				denseApplyTanhInPlace(nextLatents[i])
				currentLatents[i], nextLatents[i] = nextLatents[i], currentLatents[i]
			}
		}
	}

	return retention
}

/*
rolloutTaskForecast returns one forecast per task row, evaluated at the current
settled readout, flattened as {value, scale, dof, ready, innovation, reset} per
row. Element k is the row for horizon k+1: the cumulative directional
prediction over the next k+1 ticks from now. Every element is the supervised
head for its own horizon, so the curve is a genuine multi-horizon forecast
rather than a trajectory through imagined states. A request beyond the head's
rows yields the head's rows. Querying never updates the head.
*/
func (rm *ResonanceManifold) rolloutTaskForecast(steps int) ([]float64, error) {
	if rm.taskWeights == nil || rm.taskRows <= 0 || steps < 1 {
		return nil, nil
	}

	steps = min(steps, rm.taskRows)
	readoutData := rm.workspace.readoutBuf.RawVector().Data
	rm.readoutVectorInto(readoutData)
	query := append([]float64(nil), readoutData...)
	forecast := make([]float64, 0, steps*6)

	for horizonIndex := range steps {
		learner := rm.taskLearners[horizonIndex]
		var reading [][]float64

		for out := range learner.Next(data.NewValue(query).Next(nil)) {
			reading = *(*[][]float64)(out)
		}

		if err := learner.Error(); err != nil {
			return nil, fmt.Errorf("resonance: task forecast: %w", err)
		}

		if len(reading) == 0 || len(reading[0]) < 7 {
			return nil, fmt.Errorf("resonance: task forecast: %w", core.ErrShape)
		}

		prior := reading[0]

		forecast = append(forecast,
			prior[0], prior[1], prior[2],
			prior[4], prior[5], 0,
		)
	}

	return forecast, nil
}
