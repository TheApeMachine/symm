/*
Package learning provides streaming learning primitives: recursive least
squares, an adaptive resonance manifold for hierarchical predictive coding, a
temporal ledger for delayed target supervision, and a predictive coder that
orchestrates them.

Everything is a streaming Primitive over an unsafe.Pointer wire. Multi-operation
owners (the manifold, the ledger, the coder) receive command structs and yield
plain reading payloads with exported fields only.
*/
package learning

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"math/rand"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"

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

		if layerDim > inputDim {
			expansionRatio := float64(layerDim) / float64(inputDim)
			sparsity[latentIndex] = alpha * 5e-2 * math.Sqrt(expansionRatio)
		} else {
			sparsity[latentIndex] = alpha * 1e-2
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
SettleIntent settles the manifold over one input. AdvanceTemporal states
whether the multi-timescale temporal state also advances, which learning
forbids until its own update has consumed the current state.
*/
type SettleIntent struct {
	Input           []float64
	AdvanceTemporal bool
}

/*
LearnIntent updates every weight family from the settled state against one
optional target vector.
*/
type LearnIntent struct {
	Target []float64
}

/*
BatchIntent is one full arrival: settle, optionally learn, and report. It
composes the exact ordering the streaming coder needs — settle with temporal
advance only when not learning, learn after settling.
*/
type BatchIntent struct {
	Input           []float64
	Target          []float64
	Learn           bool
	AdvanceTemporal bool
}

/*
TaskIntent supervises one task-head row from one labeled sample. The row is
addressed by its forward horizon, one-based: horizon h supervises the
cumulative move over the next h ticks.
*/
type TaskIntent struct {
	Horizon    int
	Features   []float64
	Prediction float64
	Target     float64
}

/*
ResetIntent zeroes the latent state, and optionally every retained precision
estimate with it.
*/
type ResetIntent struct {
	Precision bool
}

/*
AlphaIntent re-derives the configured learning rates from one pace reading.
*/
type AlphaIntent struct {
	Alpha float64
}

/*
ReadingIntent asks for the manifold's current reading without mutating it.
*/
type ReadingIntent struct{}

/*
RetentionIntent asks how much latent energy survives an unforced rollout of
the temporal operators.
*/
type RetentionIntent struct {
	Steps int
}

/*
ForecastIntent asks for the supervised head's forecast from the current
settled readout.
*/
type ForecastIntent struct {
	Steps int
}

/*
ManifoldCommand discriminates one manifold operation. Exactly one intent must
be set; anything else is a shape failure.
*/
type ManifoldCommand struct {
	Settle      *SettleIntent
	Learn       *LearnIntent
	Batch       *BatchIntent
	ObserveTask *TaskIntent
	Reset       *ResetIntent
	Alpha       *AlphaIntent
	Reading     *ReadingIntent
	Retention   *RetentionIntent
	Forecast    *ForecastIntent
}

/*
RLSOutput is the supervised head's forecast wire projection: the value, its
scale, and the posterior's readiness. It is a wire payload, not a parallel
learner or execution interface.
*/
type RLSOutput struct {
	Value            float64
	Scale            float64
	DegreesOfFreedom float64
	Ready            bool
	Innovation       float64
	Reset            bool
}

/*
ResonanceLayerWire is one layer's settled state and prediction on the wire.
*/
type ResonanceLayerWire struct {
	State      []float64 `json:"state"`
	Prediction []float64 `json:"prediction"`
	ErrorNorm  float64   `json:"errorNorm"`
	Temporal   bool      `json:"temporal"`
}

/*
ManifoldReading is the manifold's answer to a command: its settled dynamics,
its harvested readout, the supervised head's per-row reliability, and, for the
rollout intents, the requested curves.
*/
type ManifoldReading struct {
	Reconstruction      float64
	Energy              float64
	PredictionEnergy    float64
	ReconstructionError float64
	EnergyDensity       float64
	Surprise            float64
	TemporalError       float64
	HasTemporalError    bool

	ReadoutDimension int
	Readout          []float64
	Latent           []float64
	TaskPrediction   []float64
	Layers           []ResonanceLayerWire

	Skill             []float64
	SkillReady        []bool
	SkillAverage      float64
	SkillReadyAvg     bool
	PrecisionReady    []bool
	PrecisionAverage  float64
	PrecisionReadyAvg bool
	ScaleAverage      float64
	ScaleReadyAvg     bool

	Forecast  []RLSOutput
	Retention []float64
}

/*
ResonanceManifold executes hierarchical predictive coding with multi-timescale
operators, sparse overcomplete dictionaries, and innovation feature
harvesting. It owns every recurrence and answers only through its command
wire.
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
	out                ManifoldReading
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
) *ResonanceManifold {
	if len(arch) < 2 {
		return &ResonanceManifold{PrimitiveError: core.NewPrimitiveError(fmt.Errorf(
			"%w: resonance: architecture must contain at least input and one latent layer",
			core.ErrShape,
		),
		)}
	}

	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		return &ResonanceManifold{PrimitiveError: core.NewPrimitiveError(fmt.Errorf(
			"%w: resonance: alpha must be finite and in (0, 1]",
			core.ErrDomain,
		),
		)}
	}

	rows := maxHorizon

	if rows < 1 {
		rows = 1
	}

	return newResonanceManifoldReadout(arch, targetDim, rows, alpha, readout)
}

func newResonanceManifoldReadout(
	arch []int,
	targetDim int,
	taskRows int,
	alpha float64,
	readout ReadoutMode,
) *ResonanceManifold {
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

	var taskWeights *mat.Dense
	var taskBias *mat.VecDense
	var taskLearners []core.Primitive
	var taskVar *mat.VecDense
	var taskScale *mat.VecDense
	var taskScaleReady []bool
	var taskPrecision *mat.VecDense
	var taskModelLoss *mat.VecDense
	var taskBaselineLoss *mat.VecDense
	var taskSkillReady []bool
	var taskSkill *mat.VecDense

	if targetDim > 0 && taskRows > 0 {
		taskWeights = mat.NewDense(taskRows, readoutDim, nil)
		taskBias = mat.NewVecDense(taskRows, nil)
		taskLearners = make([]core.Primitive, taskRows)
		taskVar = mat.NewVecDense(taskRows, nil)
		taskScale = mat.NewVecDense(taskRows, nil)
		taskScaleReady = make([]bool, taskRows)
		taskPrecision = mat.NewVecDense(taskRows, nil)
		taskModelLoss = mat.NewVecDense(taskRows, nil)
		taskBaselineLoss = mat.NewVecDense(taskRows, nil)
		taskSkillReady = make([]bool, taskRows)
		taskSkill = mat.NewVecDense(taskRows, nil)
		denseFill(taskVar, 1.0)
		denseFill(taskPrecision, 1.0)
		denseFill(taskSkill, 1.0)

		lambda := cfg.LambdaRLS

		for rowIndex := range taskRows {
			taskLearners[rowIndex] = NewRLS(readoutDim, 1.0, lambda)
		}
	}

	manifold := &ResonanceManifold{PrimitiveError: core.NewPrimitiveError(), cfg: cfg,
		arch:               arch,
		targetDim:          targetDim,
		taskRows:           taskRows,
		readoutDim:         readoutDim,
		generativeWeights:  weights,
		recognitionWeights: recognition,
		temporalOperators:  temporalOperators,
		taskWeights:        taskWeights,
		taskBias:           taskBias,
		taskLearners:       taskLearners,
		latentStates:       latents,
		errorVar:           errorVar,
		precision:          precision,
		temporalVar:        temporalVar,
		temporalPrecision:  temporalPrecision,
		taskVar:            taskVar,
		taskScale:          taskScale,
		taskScaleReady:     taskScaleReady,
		taskPrecision:      taskPrecision,
		taskModelLoss:      taskModelLoss,
		taskBaselineLoss:   taskBaselineLoss,
		taskSkillReady:     taskSkillReady,
		taskSkill:          taskSkill,
		workspace:          newResonanceWorkspace(arch, taskRows, cfg.ReadoutMode),
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

/*
Next receives *ManifoldCommand payloads and yields a *ManifoldReading for
each. Mutating intents answer with the manifold's full settled reading; the
rollout intents answer with the requested curve. Any invalid intent ends the
stream with the error recorded.
*/
func (resonanceManifold *ResonanceManifold) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	if resonanceManifold.
		Error() !=
		nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*ManifoldCommand)(arriving)
			reading, err := resonanceManifold.execute(command)

			if err != nil {
				resonanceManifold.Error(err)
				return
			}

			resonanceManifold.out = reading

			if !yield(unsafe.Pointer(&resonanceManifold.out)) {
				return
			}
		}
	}
}

/*
execute dispatches one command to its intent and returns its reading.
*/
func (resonanceManifold *ResonanceManifold) execute(
	command *ManifoldCommand,
) (ManifoldReading, error) {
	intents := 0

	for _, set := range []bool{
		command.Settle != nil, command.Learn != nil, command.Batch != nil,
		command.ObserveTask != nil, command.Reset != nil, command.Alpha != nil,
		command.Reading != nil, command.Retention != nil, command.Forecast != nil,
	} {
		if set {
			intents++
		}
	}

	if intents != 1 {
		return ManifoldReading{}, fmt.Errorf(
			"%w: resonance: manifold command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Settle != nil {
		return resonanceManifold.executeSettle(command.Settle)
	}

	if command.Learn != nil {
		return resonanceManifold.executeLearn(command.Learn)
	}

	if command.Batch != nil {
		return resonanceManifold.executeBatch(command.Batch)
	}

	if command.ObserveTask != nil {
		return resonanceManifold.executeTask(command.ObserveTask)
	}

	if command.Reset != nil {
		return resonanceManifold.executeReset(command.Reset)
	}

	if command.Alpha != nil {
		return resonanceManifold.executeAlpha(command.Alpha)
	}

	if command.Reading != nil {
		return resonanceManifold.snapshot(), nil
	}

	if command.Retention != nil {
		return resonanceManifold.executeRetention(command.Retention)
	}

	return resonanceManifold.executeForecast(command.Forecast)
}

func (resonanceManifold *ResonanceManifold) executeSettle(
	intent *SettleIntent,
) (ManifoldReading, error) {
	if err := resonanceManifold.settle(intent.Input, intent.AdvanceTemporal); err != nil {
		return ManifoldReading{}, fmt.Errorf("resonance: settle failed: %w", err)
	}

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeLearn(
	intent *LearnIntent,
) (ManifoldReading, error) {
	if err := resonanceManifold.learn(intent.Target); err != nil {
		return ManifoldReading{}, err
	}

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeBatch(
	intent *BatchIntent,
) (ManifoldReading, error) {
	if len(intent.Input) != resonanceManifold.arch[0] {
		return ManifoldReading{}, fmt.Errorf(
			"%w: resonance: input dimension mismatch",
			core.ErrShape,
		)
	}

	settleAdvanceTemporal := intent.AdvanceTemporal && !intent.Learn

	if err := resonanceManifold.settle(intent.Input, settleAdvanceTemporal); err != nil {
		return ManifoldReading{}, fmt.Errorf("resonance: settle failed: %w", err)
	}

	if intent.Learn {
		if err := resonanceManifold.learn(intent.Target); err != nil {
			return ManifoldReading{}, err
		}
	}

	resonanceManifold.output = resonanceManifold.reconstructionError()

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeTask(
	intent *TaskIntent,
) (ManifoldReading, error) {
	if err := resonanceManifold.observeTask(
		intent.Horizon,
		intent.Features,
		intent.Prediction,
		intent.Target,
	); err != nil {
		return ManifoldReading{}, err
	}

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeReset(
	intent *ResetIntent,
) (ManifoldReading, error) {
	resonanceManifold.resetState(intent.Precision)

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeAlpha(
	intent *AlphaIntent,
) (ManifoldReading, error) {
	if err := resonanceManifold.setAlpha(intent.Alpha); err != nil {
		return ManifoldReading{}, err
	}

	return resonanceManifold.snapshot(), nil
}

func (resonanceManifold *ResonanceManifold) executeRetention(
	intent *RetentionIntent,
) (ManifoldReading, error) {
	if intent.Steps < 1 {
		return ManifoldReading{}, fmt.Errorf(
			"%w: resonance: rollout retention requires a positive step count",
			core.ErrDomain,
		)
	}

	return ManifoldReading{Retention: resonanceManifold.rolloutRetention(intent.Steps)}, nil
}

func (resonanceManifold *ResonanceManifold) executeForecast(
	intent *ForecastIntent,
) (ManifoldReading, error) {
	if intent.Steps < 1 {
		return ManifoldReading{}, fmt.Errorf(
			"%w: resonance: task forecast requires a positive step count",
			core.ErrDomain,
		)
	}

	forecast, err := resonanceManifold.rolloutTaskForecast(intent.Steps)

	if err != nil {
		return ManifoldReading{}, err
	}

	return ManifoldReading{Forecast: forecast}, nil
}

/*
snapshot assembles the manifold's full reading: settled dynamics, harvested
readout, latent state, supervised head predictions, wire layers, and the
per-row and averaged reliability of the task head.
*/
func (resonanceManifold *ResonanceManifold) snapshot() ManifoldReading {
	reading := ManifoldReading{
		Reconstruction:      resonanceManifold.output,
		ReconstructionError: resonanceManifold.reconstructionError(),
		ReadoutDimension:    resonanceManifold.readoutDim,
		Readout:             resonanceManifold.readoutVector(),
		Latent:              resonanceManifold.latentState(),
		TaskPrediction:      resonanceManifold.taskPrediction(),
	}

	reading.Energy = resonanceManifold.energy()
	reading.PredictionEnergy = resonanceManifold.predictionEnergy()
	reading.TemporalError, reading.HasTemporalError = resonanceManifold.temporalError()
	reading.Layers, reading.Surprise, reading.EnergyDensity = resonanceManifold.wireSnapshot()

	if resonanceManifold.taskRows > 0 {
		reading.Skill = append([]float64(nil), resonanceManifold.taskSkill.RawVector().Data...)
		reading.SkillReady = append([]bool(nil), resonanceManifold.taskScaleReady...)
		reading.PrecisionReady = append([]bool(nil), resonanceManifold.taskScaleReady...)
		reading.SkillAverage, reading.SkillReadyAvg = resonanceManifold.taskSkillAverage()
		reading.PrecisionAverage, reading.PrecisionReadyAvg = resonanceManifold.taskPrecisionAverage()
		reading.ScaleAverage, reading.ScaleReadyAvg = resonanceManifold.taskScaleAverage()
	}

	return reading
}

func (resonanceManifold *ResonanceManifold) resetState(resetPrecision bool) {
	for _, latent := range resonanceManifold.latentStates {
		latent.Zero()
	}
	for latentIndex := range resonanceManifold.temporalOperators {
		resonanceManifold.workspace.prevLatents[latentIndex].Zero()
	}
	resonanceManifold.temporalPriorsReady = false
	resonanceManifold.settleAdvancedTemporal = false

	if resetPrecision {
		for layerIndex := 0; layerIndex < len(resonanceManifold.generativeWeights); layerIndex++ {
			denseFill(resonanceManifold.errorVar[layerIndex], 1.0)
			denseFill(resonanceManifold.precision[layerIndex], 1.0)
		}
		for latentIndex := range resonanceManifold.temporalOperators {
			denseFill(resonanceManifold.temporalVar[latentIndex], 1.0)
			denseFill(resonanceManifold.temporalPrecision[latentIndex], 1.0)
		}

		if resonanceManifold.taskRows > 0 {
			denseFill(resonanceManifold.taskVar, 1.0)
			resonanceManifold.taskScale.Zero()
			denseFill(resonanceManifold.taskPrecision, 1.0)
			resonanceManifold.taskModelLoss.Zero()
			resonanceManifold.taskBaselineLoss.Zero()
			denseFill(resonanceManifold.taskSkill, 1.0)
			clear(resonanceManifold.taskScaleReady)
			clear(resonanceManifold.taskSkillReady)
		}
	}
}

/*
settle performs generative inference by minimizing precision-weighted prediction
error, multi-timescale temporal priors, and overcomplete dictionary sparsity.
*/
func (resonanceManifold *ResonanceManifold) settle(input []float64, advanceTemporal bool) error {
	if len(input) != resonanceManifold.arch[0] {
		return fmt.Errorf(
			"%w: resonance: input dimension mismatch",
			core.ErrShape,
		)
	}

	resonanceManifold.settleAdvancedTemporal = false
	resonanceManifold.lastInferenceSteps = 0

	xCol := resonanceManifold.workspace.xCol
	copy(xCol.RawVector().Data, input)

	resonanceManifold.initializeLatents(xCol)

	settledEnergy := resonanceManifold.energy()
	stableSteps := 0

	for step := 0; step < resonanceManifold.cfg.MaxInferenceSteps; step++ {
		resonanceManifold.lastInferenceSteps = step + 1
		predictions, layerErrors := resonanceManifold.predictAdjacentLayers()
		gradients := resonanceManifold.stateGradients(predictions, layerErrors)

		resonanceManifold.saveStates()
		accepted := false
		candidateEnergy := settledEnergy
		stepSize := resonanceManifold.cfg.LrState

		halvings := 0
		if resonanceManifold.cfg.MonotoneStateSteps {
			halvings = resonanceManifold.cfg.LineSearchHalvings
		}

		for halvingIndex := 0; halvingIndex <= halvings; halvingIndex++ {
			resonanceManifold.tryStateUpdate(gradients, stepSize)
			resonanceManifold.latentStates[0].CopyVec(xCol)
			candidateEnergy = resonanceManifold.energy()

			if !resonanceManifold.cfg.MonotoneStateSteps || candidateEnergy <= math.Nextafter(settledEnergy, math.Inf(1)) {
				accepted = true
				break
			}

			resonanceManifold.restoreStates()
			stepSize *= 0.5
		}

		if !accepted {
			resonanceManifold.restoreStates()
			resonanceManifold.latentStates[0].CopyVec(xCol)
			stableSteps = 0
			continue
		}

		deltaEnergy := math.Abs(settledEnergy - candidateEnergy)
		energyScale := math.Max(math.Abs(settledEnergy), resonanceManifold.cfg.PrecisionEps)
		relativeDelta := deltaEnergy / energyScale
		settledEnergy = candidateEnergy

		if step+1 < resonanceManifold.cfg.MinInferenceSteps || relativeDelta >= resonanceManifold.cfg.EarlyStopTol {
			stableSteps = 0
			continue
		}

		stableSteps++
		if stableSteps >= resonanceManifold.cfg.EarlyStopPatience {
			break
		}
	}

	if advanceTemporal {
		resonanceManifold.advanceTemporalState()
		resonanceManifold.settleAdvancedTemporal = true
	}

	return nil
}

/*
learn updates generative, recognition, multi-timescale temporal matrices, and
the downstream multi-layer task head via RLS.
*/
func (resonanceManifold *ResonanceManifold) learn(target []float64) error {
	if resonanceManifold.settleAdvancedTemporal {
		return fmt.Errorf(
			"%w: resonance: temporal state advanced before learning",
			core.ErrDomain,
		)
	}

	if target != nil && len(target) != resonanceManifold.targetDim {
		return fmt.Errorf(
			"%w: resonance: target dimension mismatch: expected %d, got %d",
			core.ErrShape,
			resonanceManifold.targetDim,
			len(target),
		)
	}

	predictions, layerErrors := resonanceManifold.predictAdjacentLayers()

	// 1. Generative weights update
	for layerIndex, weightMatrix := range resonanceManifold.generativeWeights {
		localSignal := resonanceManifold.workspace.localSignal[layerIndex]
		if layerIndex == 0 {
			for i := 0; i < localSignal.Len(); i++ {
				localSignal.SetVec(i, 1.0)
			}
		}
		if layerIndex > 0 {
			denseApplyOneMinusSquareInto(localSignal, predictions[layerIndex])
		}
		precision := resonanceManifold.precisionFor(layerIndex)
		localSignal.MulElemVec(localSignal, layerErrors[layerIndex])
		localSignal.MulElemVec(localSignal, precision)

		update := resonanceManifold.workspace.weightUpdate[layerIndex]
		denseOuterColsInto(update, localSignal, resonanceManifold.latentStates[layerIndex+1], 1.0)

		scale := resonanceManifold.cfg.LrGenerative
		if norm := mat.Norm(update, 2); norm > resonanceManifold.cfg.GradClip {
			scale *= resonanceManifold.cfg.GradClip / norm
		}

		denseScaleInPlace(update, scale)
		weightMatrix.Add(weightMatrix, update)

		if resonanceManifold.cfg.WeightDecay > 0 {
			denseScaleInPlace(weightMatrix, 1.0-resonanceManifold.cfg.LrGenerative*resonanceManifold.cfg.WeightDecay)
		}
	}

	// 2. Recognition weights update
	for layerIndex, recognitionMatrix := range resonanceManifold.recognitionWeights {
		proposal := resonanceManifold.workspace.recProposal[layerIndex]
		proposal.MulVec(recognitionMatrix, resonanceManifold.latentStates[layerIndex])
		denseApplyTanhInPlace(proposal)

		recError := resonanceManifold.workspace.recError[layerIndex]
		recError.SubVec(resonanceManifold.latentStates[layerIndex+1], proposal)

		recSignal := resonanceManifold.workspace.recSignal[layerIndex]
		denseApplyOneMinusSquareInto(recSignal, proposal)
		recSignal.MulElemVec(recSignal, recError)

		update := resonanceManifold.workspace.recUpdate[layerIndex]
		denseOuterColsInto(update, recSignal, resonanceManifold.latentStates[layerIndex], 1.0)

		scale := resonanceManifold.cfg.LrRecognition
		if norm := mat.Norm(update, 2); norm > resonanceManifold.cfg.GradClip {
			scale *= resonanceManifold.cfg.GradClip / norm
		}

		denseScaleInPlace(update, scale)
		recognitionMatrix.Add(recognitionMatrix, update)

		if resonanceManifold.cfg.WeightDecay > 0 {
			denseScaleInPlace(recognitionMatrix, 1.0-resonanceManifold.cfg.LrRecognition*resonanceManifold.cfg.WeightDecay)
		}
	}

	// 3. Multi-timescale temporal operators update across all latent layers
	temporalErrors := make([]*mat.VecDense, len(resonanceManifold.temporalOperators))
	if resonanceManifold.temporalPriorsReady {
		for latentIndex, operator := range resonanceManifold.temporalOperators {
			layerIndex := latentIndex + 1
			temporalError := resonanceManifold.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(resonanceManifold.latentStates[layerIndex], resonanceManifold.workspace.temporalPriors[latentIndex])
			temporalErrors[latentIndex] = temporalError

			temporalSignal := resonanceManifold.workspace.temporalSignals[latentIndex]
			denseApplyOneMinusSquareInto(temporalSignal, resonanceManifold.workspace.temporalPriors[latentIndex])
			precision := resonanceManifold.temporalPrecision[latentIndex]
			temporalSignal.MulElemVec(temporalSignal, temporalError)
			temporalSignal.MulElemVec(temporalSignal, precision)
			temporalSignal.ScaleVec(resonanceManifold.cfg.TemporalWeights[latentIndex], temporalSignal)

			update := resonanceManifold.workspace.temporalUpdates[latentIndex]
			denseOuterColsInto(update, temporalSignal, resonanceManifold.workspace.prevLatents[latentIndex], 1.0)

			scale := resonanceManifold.cfg.LrTemporal
			if norm := mat.Norm(update, 2); norm > resonanceManifold.cfg.GradClip {
				scale *= resonanceManifold.cfg.GradClip / norm
			}

			denseScaleInPlace(update, scale)
			operator.Add(operator, update)

			if resonanceManifold.cfg.WeightDecay > 0 {
				denseScaleInPlace(operator, 1.0-resonanceManifold.cfg.LrTemporal*resonanceManifold.cfg.WeightDecay)
			}

			if err := resonanceManifold.projectTemporalOperatorNorm(latentIndex); err != nil {
				return fmt.Errorf("resonance: constrain temporal operator %d: %w", latentIndex, err)
			}
		}
	}

	// 4. Multi-layer & innovation task head update (RLS)
	var targetCol *mat.VecDense
	var taskError *mat.VecDense
	trainedRows := 0

	if target != nil && resonanceManifold.taskWeights != nil {
		trainedRows = len(target)

		if trainedRows > resonanceManifold.taskRows {
			trainedRows = resonanceManifold.taskRows
		}

		targetCol = resonanceManifold.workspace.yCol
		copy(targetCol.RawVector().Data, target)

		taskPred := resonanceManifold.workspace.taskPred
		resonanceManifold.taskPredictionInto(taskPred)

		taskError = resonanceManifold.workspace.taskError
		taskError.SubVec(targetCol, taskPred)

		readoutData := resonanceManifold.workspace.readoutBuf.RawVector().Data
		resonanceManifold.readoutVectorInto(readoutData)
		targetData := targetCol.RawVector().Data
		biasData := resonanceManifold.taskBias.RawVector().Data

		for rowIndex := range trainedRows {
			reading, err := resonanceManifold.taskReading(rowIndex, readoutData, targetData[rowIndex])

			if err != nil {
				return fmt.Errorf("resonance: task learner update: %w", err)
			}

			intercept, err := taskCoefficients(reading, resonanceManifold.taskWeights.RawRowView(rowIndex))

			if err != nil {
				return fmt.Errorf("resonance: task learner coefficients: %w", err)
			}
			biasData[rowIndex] = intercept
		}
	}

	if err := resonanceManifold.updatePrecision(layerErrors, temporalErrors, targetCol, taskError, trainedRows); err != nil {
		return err
	}

	resonanceManifold.advanceTemporalState()
	return nil
}

/*
energy is the variational free energy combining precision-weighted error,
multi-timescale temporal priors, $L_2$ decay, and $L_1$ dictionary sparsity.
*/
func (resonanceManifold *ResonanceManifold) energy() float64 {
	energy := resonanceManifold.predictionEnergy()

	for latentIndex := range resonanceManifold.temporalOperators {
		layerIndex := latentIndex + 1
		latent := resonanceManifold.latentStates[layerIndex]
		if resonanceManifold.cfg.LatentDecay[latentIndex] > 0 {
			norm := denseColNorm(latent)
			energy += 0.5 * resonanceManifold.cfg.LatentDecay[latentIndex] * norm * norm
		}
		if resonanceManifold.cfg.Sparsity[latentIndex] > 0 {
			energy += resonanceManifold.cfg.Sparsity[latentIndex] * floats.Norm(latent.RawVector().Data, 1)
		}
	}

	return energy
}

/*
predictionEnergy computes total precision-weighted prediction error across all
generative links and multi-timescale temporal links.
*/
func (resonanceManifold *ResonanceManifold) predictionEnergy() float64 {
	_, layerErrors := resonanceManifold.predictAdjacentLayers()
	energy := 0.0

	for layerIndex, layerError := range layerErrors {
		if resonanceManifold.cfg.UsePrecision {
			weightedError := resonanceManifold.workspace.weightedErr[layerIndex]
			weightedError.MulElemVec(resonanceManifold.precisionFor(layerIndex), layerError)
			energy += 0.5 * denseColDot(weightedError, layerError)
		} else {
			energy += 0.5 * denseColDot(layerError, layerError)
		}
	}

	if resonanceManifold.temporalPriorsReady {
		for latentIndex := range resonanceManifold.temporalOperators {
			layerIndex := latentIndex + 1
			temporalError := resonanceManifold.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(resonanceManifold.latentStates[layerIndex], resonanceManifold.workspace.temporalPriors[latentIndex])

			weight := resonanceManifold.cfg.TemporalWeights[latentIndex]
			if resonanceManifold.cfg.UsePrecision {
				weightedError := resonanceManifold.workspace.temporalWeightedErrs[latentIndex]
				weightedError.MulElemVec(resonanceManifold.temporalPrecision[latentIndex], temporalError)
				energy += 0.5 * weight * denseColDot(weightedError, temporalError)
			} else {
				energy += 0.5 * weight * denseColDot(temporalError, temporalError)
			}
		}
	}

	return energy
}

func (resonanceManifold *ResonanceManifold) reconstructionError() float64 {
	reconstruction := resonanceManifold.workspace.reconPred
	reconstruction.MulVec(resonanceManifold.generativeWeights[0], resonanceManifold.latentStates[1])
	// Layer 0 is linear (continuous unbounded z-scores)

	diff := resonanceManifold.workspace.reconDiff
	diff.SubVec(resonanceManifold.latentStates[0], reconstruction)

	return denseColNorm(diff)
}

func (resonanceManifold *ResonanceManifold) taskPredictionInto(dst *mat.VecDense) {
	readoutData := resonanceManifold.workspace.readoutBuf.RawVector().Data
	resonanceManifold.readoutVectorInto(readoutData)

	dst.MulVec(resonanceManifold.taskWeights, resonanceManifold.workspace.readoutBuf)
	dst.AddVec(dst, resonanceManifold.taskBias)
}

func (resonanceManifold *ResonanceManifold) taskPrediction() []float64 {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return nil
	}

	taskPred := resonanceManifold.workspace.taskPred
	resonanceManifold.taskPredictionInto(taskPred)
	return append([]float64(nil), taskPred.RawVector().Data...)
}

/*
taskReading drives one task-head row's learner with one labeled sample and
returns its posterior reading.
*/
func (resonanceManifold *ResonanceManifold) taskReading(
	rowIndex int,
	features []float64,
	target float64,
) (algo.Reading, error) {
	evaluation := resonanceManifold.taskLearners[rowIndex]
	var reading algo.Reading

	for out := range evaluation.Next(sequence.NewValues(Sample{
		Features: features,
		Target:   target,
		Observed: true,
	}).Next(nil)) {
		reading = *(*algo.Reading)(out)
	}

	return reading, evaluation.Error()
}

/*
observeTask updates one task-head row from one labeled sample. The row is
addressed by its forward horizon, one-based: horizon h supervises the
cumulative move over the next h ticks.
*/
func (resonanceManifold *ResonanceManifold) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
) error {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return fmt.Errorf(
			"%w: resonance: supervised task head required",
			core.ErrShape,
		)
	}

	if horizon < 1 || horizon > resonanceManifold.taskRows {
		return fmt.Errorf(
			"%w: resonance: task horizon %d out of range [1, %d]",
			core.ErrDomain,
			horizon,
			resonanceManifold.taskRows,
		)
	}

	if len(features) != resonanceManifold.readoutDim {
		return fmt.Errorf(
			"%w: resonance: expected %d task features, got %d",
			core.ErrShape,
			resonanceManifold.readoutDim,
			len(features),
		)
	}

	for index, feature := range features {
		if !finite(feature) {
			return fmt.Errorf(
				"%w: resonance: task feature %d must be finite",
				core.ErrDomain,
				index,
			)
		}
	}

	if !finite(prediction) || !finite(target) {
		return fmt.Errorf(
			"%w: resonance: task prediction and target must be finite",
			core.ErrDomain,
		)
	}

	rowIndex := horizon - 1

	reading, err := resonanceManifold.taskReading(rowIndex, features, target)

	if err != nil {
		return fmt.Errorf("resonance: task learner update: %w", err)
	}

	intercept, err := taskCoefficients(reading, resonanceManifold.taskWeights.RawRowView(rowIndex))

	if err != nil {
		return fmt.Errorf("resonance: task learner coefficients: %w", err)
	}

	resonanceManifold.taskBias.RawVector().Data[rowIndex] = intercept

	resonanceManifold.updateTaskReliability(rowIndex, target, target-prediction)

	return nil
}

func (resonanceManifold *ResonanceManifold) latentState() []float64 {
	if len(resonanceManifold.latentStates) == 0 {
		return nil
	}

	return append([]float64(nil), resonanceManifold.latentStates[len(resonanceManifold.latentStates)-1].RawVector().Data...)
}

func (resonanceManifold *ResonanceManifold) temporalError() (float64, bool) {
	if !resonanceManifold.temporalPriorsReady || len(resonanceManifold.temporalOperators) == 0 {
		return 0, false
	}

	topLatentIdx := len(resonanceManifold.temporalOperators) - 1
	temporalError := resonanceManifold.workspace.temporalErrors[topLatentIdx]
	return denseColNorm(temporalError), true
}

func (resonanceManifold *ResonanceManifold) wireSnapshot() (
	layers []ResonanceLayerWire,
	surprise float64,
	energyDensity float64,
) {
	predictions, layerErrors := resonanceManifold.predictAdjacentLayers()
	layers = make([]ResonanceLayerWire, len(resonanceManifold.latentStates))
	topIndex := len(resonanceManifold.latentStates) - 1
	temporalNorm, hasTemporal := resonanceManifold.temporalError()

	for layerIndex := range resonanceManifold.latentStates {
		stateMatrix := resonanceManifold.latentStates[layerIndex]
		rowCount, _ := stateMatrix.Dims()
		state := append([]float64(nil), stateMatrix.RawVector().Data...)
		prediction := make([]float64, rowCount)

		if layerIndex < len(predictions) {
			copy(prediction, predictions[layerIndex].RawVector().Data)
		}

		if layerIndex == topIndex && resonanceManifold.temporalPriorsReady {
			copy(prediction, resonanceManifold.workspace.temporalPriors[len(resonanceManifold.temporalOperators)-1].RawVector().Data)
		}

		errorNorm := 0.0
		temporal := false

		switch {
		case layerIndex < len(layerErrors):
			errorNorm = denseColNorm(layerErrors[layerIndex])
		case layerIndex == topIndex && hasTemporal:
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

	reconstructionDimensions := float64(resonanceManifold.arch[0])
	predictionDimensions := 0

	for _, layerError := range layerErrors {
		predictionDimensions += layerError.Len()
	}

	if hasTemporal {
		predictionDimensions += resonanceManifold.arch[topIndex]
	}

	return layers,
		resonanceManifold.reconstructionError() / math.Sqrt(reconstructionDimensions),
		resonanceManifold.predictionEnergy() / float64(predictionDimensions)
}

func (resonanceManifold *ResonanceManifold) taskPrecisionAverage() (float64, bool) {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range resonanceManifold.taskRows {
		if resonanceManifold.taskScaleReady[rowIndex] {
			sum += resonanceManifold.taskPrecision.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (resonanceManifold *ResonanceManifold) taskSkillAverage() (float64, bool) {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range resonanceManifold.taskRows {
		if resonanceManifold.taskSkillReady[rowIndex] {
			sum += resonanceManifold.taskSkill.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (resonanceManifold *ResonanceManifold) taskScaleAverage() (float64, bool) {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range resonanceManifold.taskRows {
		if resonanceManifold.taskScaleReady[rowIndex] {
			sum += resonanceManifold.taskScale.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (resonanceManifold *ResonanceManifold) stateGradients(
	predictions []*mat.VecDense,
	layerErrors []*mat.VecDense,
) []*mat.VecDense {
	topIndex := len(resonanceManifold.latentStates) - 1

	for layerIndex := 1; layerIndex <= topIndex; layerIndex++ {
		gradient := resonanceManifold.workspace.grads[layerIndex]
		gradient.Zero()
		latentIndex := layerIndex - 1

		if layerIndex < topIndex {
			if resonanceManifold.cfg.UsePrecision {
				weightedError := resonanceManifold.workspace.weightedErr[layerIndex]
				weightedError.MulElemVec(resonanceManifold.precisionFor(layerIndex), layerErrors[layerIndex])
				gradient.AddVec(gradient, weightedError)
			} else {
				gradient.AddVec(gradient, layerErrors[layerIndex])
			}
		}

		belowSignal := resonanceManifold.workspace.belowSignal[layerIndex-1]
		if layerIndex-1 == 0 {
			for i := 0; i < belowSignal.Len(); i++ {
				belowSignal.SetVec(i, 1.0)
			}
		}
		if layerIndex-1 > 0 {
			denseApplyOneMinusSquareInto(belowSignal, predictions[layerIndex-1])
		}
		if resonanceManifold.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
			belowSignal.MulElemVec(belowSignal, resonanceManifold.precisionFor(layerIndex-1))
		}
		if !resonanceManifold.cfg.UsePrecision {
			belowSignal.MulElemVec(belowSignal, layerErrors[layerIndex-1])
		}

		correction := resonanceManifold.workspace.correction[layerIndex]
		denseMulWeightTransposeInto(correction, resonanceManifold.generativeWeights[layerIndex-1], belowSignal)
		gradient.SubVec(gradient, correction)

		if resonanceManifold.temporalPriorsReady {
			temporalError := resonanceManifold.workspace.temporalErrors[latentIndex]
			temporalError.SubVec(resonanceManifold.latentStates[layerIndex], resonanceManifold.workspace.temporalPriors[latentIndex])

			if resonanceManifold.cfg.UsePrecision {
				temporalError.MulElemVec(temporalError, resonanceManifold.temporalPrecision[latentIndex])
			}

			temporalError.ScaleVec(resonanceManifold.cfg.TemporalWeights[latentIndex], temporalError)
			gradient.AddVec(gradient, temporalError)
		}

		if resonanceManifold.cfg.LatentDecay[latentIndex] > 0 {
			floats.AddScaled(
				gradient.RawVector().Data,
				resonanceManifold.cfg.LatentDecay[latentIndex],
				resonanceManifold.latentStates[layerIndex].RawVector().Data,
			)
		}

		if resonanceManifold.cfg.Sparsity[latentIndex] > 0 {
			gradientData := gradient.RawVector().Data
			latentData := resonanceManifold.latentStates[layerIndex].RawVector().Data
			s := resonanceManifold.cfg.Sparsity[latentIndex]

			for index, val := range latentData {
				if val > 0 {
					gradientData[index] += s
				} else if val < 0 {
					gradientData[index] -= s
				}
			}
		}

		gradientNorm := denseColNorm(gradient)
		if gradientNorm > resonanceManifold.cfg.GradClip {
			gradient.ScaleVec(resonanceManifold.cfg.GradClip/gradientNorm, gradient)
		}
	}

	return resonanceManifold.workspace.grads
}

func (resonanceManifold *ResonanceManifold) initializeLatents(xCol *mat.VecDense) {
	bottomUp := resonanceManifold.workspace.bottomUp
	bottomUp[0].CopyVec(xCol)

	for layerIndex := 0; layerIndex < len(resonanceManifold.recognitionWeights); layerIndex++ {
		proposal := bottomUp[layerIndex+1]
		proposal.MulVec(resonanceManifold.recognitionWeights[layerIndex], bottomUp[layerIndex])
		denseApplyTanhInPlace(proposal)
	}

	resonanceManifold.latentStates[0].CopyVec(xCol)

	if !resonanceManifold.temporalPriorsReady {
		for layerIndex := 1; layerIndex < len(resonanceManifold.latentStates); layerIndex++ {
			resonanceManifold.latentStates[layerIndex].CopyVec(bottomUp[layerIndex])
		}
		return
	}

	topDown := resonanceManifold.workspace.topDown
	for latentIndex, operator := range resonanceManifold.temporalOperators {
		prior := resonanceManifold.workspace.temporalPriors[latentIndex]
		prior.MulVec(operator, resonanceManifold.workspace.prevLatents[latentIndex])
		denseApplyTanhInPlace(prior)
		topDown[latentIndex+1].CopyVec(prior)
	}

	initMix := resonanceManifold.cfg.TopDownInitMix
	for layerIndex := 1; layerIndex < len(resonanceManifold.latentStates); layerIndex++ {
		merged := resonanceManifold.latentStates[layerIndex]
		merged.ScaleVec(initMix, topDown[layerIndex])
		floats.AddScaled(
			merged.RawVector().Data,
			1.0-initMix,
			bottomUp[layerIndex].RawVector().Data,
		)
		denseClipColInPlace(merged, resonanceManifold.cfg.StateClip)
	}
}

func (resonanceManifold *ResonanceManifold) advanceTemporalState() {
	for latentIndex := range resonanceManifold.temporalOperators {
		layerIndex := latentIndex + 1
		resonanceManifold.workspace.prevLatents[latentIndex].CopyVec(resonanceManifold.latentStates[layerIndex])
	}
	resonanceManifold.temporalPriorsReady = true
}

func (resonanceManifold *ResonanceManifold) precisionFor(layerIndex int) *mat.VecDense {
	return resonanceManifold.precision[layerIndex]
}

func (resonanceManifold *ResonanceManifold) projectTemporalOperatorNorm(latentIndex int) error {
	if !(resonanceManifold.cfg.TemporalNormMax > 0) || resonanceManifold.cfg.TemporalNormMax >= 1 {
		return errors.New("resonance: temporal operator-norm limit must be in (0, 1)")
	}

	decomposition := &resonanceManifold.workspace.temporalSVDs[latentIndex]
	operator := resonanceManifold.temporalOperators[latentIndex]

	if ok := decomposition.Factorize(operator, mat.SVDNone); !ok {
		return errors.New("resonance: temporal singular-value decomposition failed")
	}

	singularValues := decomposition.Values(resonanceManifold.workspace.layerSVDValues[latentIndex])
	if len(singularValues) == 0 || math.IsNaN(singularValues[0]) || math.IsInf(singularValues[0], 0) {
		return errors.New("resonance: temporal operator norm must be finite")
	}

	operatorNorm := singularValues[0]
	if operatorNorm <= resonanceManifold.cfg.TemporalNormMax {
		return nil
	}

	denseScaleInPlace(operator, resonanceManifold.cfg.TemporalNormMax/operatorNorm)
	return nil
}

func (resonanceManifold *ResonanceManifold) saveStates() {
	for layerIndex, latent := range resonanceManifold.latentStates {
		resonanceManifold.workspace.savedStates[layerIndex].CopyVec(latent)
	}
}

func (resonanceManifold *ResonanceManifold) restoreStates() {
	for layerIndex, latent := range resonanceManifold.latentStates {
		latent.CopyVec(resonanceManifold.workspace.savedStates[layerIndex])
	}
}

func (resonanceManifold *ResonanceManifold) tryStateUpdate(gradients []*mat.VecDense, stepSize float64) {
	for layerIndex := 1; layerIndex < len(resonanceManifold.latentStates); layerIndex++ {
		step := resonanceManifold.workspace.stepBuf[layerIndex]
		step.ScaleVec(stepSize, gradients[layerIndex])
		nextState := resonanceManifold.latentStates[layerIndex]
		nextState.SubVec(resonanceManifold.workspace.savedStates[layerIndex], step)
		denseClipColInPlace(nextState, resonanceManifold.cfg.StateClip)
	}
}

func (resonanceManifold *ResonanceManifold) updatePrecision(
	layerErrors []*mat.VecDense,
	temporalErrors []*mat.VecDense,
	targetCol *mat.VecDense,
	taskError *mat.VecDense,
	trainedRows int,
) error {
	if !resonanceManifold.cfg.UsePrecision {
		return nil
	}

	beta := resonanceManifold.cfg.PrecisionBeta

	for layerIndex, layerError := range layerErrors {
		variance := resonanceManifold.errorVar[layerIndex]
		denseVarianceEMAInto(variance, layerError, beta, resonanceManifold.cfg.PrecisionEps)
		densePrecisionFromVarianceInto(resonanceManifold.precision[layerIndex], variance, resonanceManifold.cfg.PrecisionMin, resonanceManifold.cfg.PrecisionMax)
	}

	for latentIndex, tempErr := range temporalErrors {
		if tempErr == nil {
			continue
		}
		variance := resonanceManifold.temporalVar[latentIndex]
		denseVarianceEMAInto(variance, tempErr, beta, resonanceManifold.cfg.PrecisionEps)
		densePrecisionFromVarianceInto(resonanceManifold.temporalPrecision[latentIndex], variance, resonanceManifold.cfg.PrecisionMin, resonanceManifold.cfg.PrecisionMax)
	}

	if targetCol != nil && taskError != nil && resonanceManifold.taskWeights != nil {
		targetData := targetCol.RawVector().Data
		errorData := taskError.RawVector().Data

		for rowIndex := range trainedRows {
			resonanceManifold.updateTaskReliability(rowIndex, targetData[rowIndex], errorData[rowIndex])
		}
	}

	return nil
}

/*
updateTaskReliability folds one resolved sample into one task row's variance,
scale, precision, and skill readouts. Each row owns its moments, so the nested
multi-horizon head scores each horizon against its own prediction error.
*/
func (resonanceManifold *ResonanceManifold) updateTaskReliability(
	rowIndex int,
	target float64,
	taskError float64,
) {
	beta := resonanceManifold.cfg.PrecisionBeta
	squaredError := taskError * taskError
	taskVarianceData := resonanceManifold.taskVar.RawVector().Data
	taskScaleData := resonanceManifold.taskScale.RawVector().Data
	taskPrecisionData := resonanceManifold.taskPrecision.RawVector().Data

	if !resonanceManifold.taskScaleReady[rowIndex] {
		if squaredError > 0 {
			taskVarianceData[rowIndex] = squaredError
			taskScaleData[rowIndex] = math.Log(squaredError)
		}
	} else {
		candidateVariance := (1.0-beta)*taskVarianceData[rowIndex] + beta*squaredError
		varianceFloor := resonanceManifold.cfg.PrecisionEps * math.Exp(taskScaleData[rowIndex])
		taskVarianceData[rowIndex] = math.Max(candidateVariance, varianceFloor)

		if taskVarianceData[rowIndex] > varianceFloor {
			taskScaleData[rowIndex] = (1.0-beta)*taskScaleData[rowIndex] +
				beta*math.Log(taskVarianceData[rowIndex])
		}
	}

	if !resonanceManifold.taskScaleReady[rowIndex] && squaredError > 0 {
		resonanceManifold.taskScaleReady[rowIndex] = true
	}

	varianceFloor := math.Exp(taskScaleData[rowIndex])

	if !resonanceManifold.taskScaleReady[rowIndex] {
		taskPrecisionData[rowIndex] = 1.0
	} else {
		value := varianceFloor / taskVarianceData[rowIndex]
		taskPrecisionData[rowIndex] = math.Min(
			resonanceManifold.cfg.PrecisionMax,
			math.Max(resonanceManifold.cfg.PrecisionMin, value),
		)
	}

	resonanceManifold.updateTaskSkill(rowIndex, target, squaredError)
}

/*
updateTaskSkill maintains one row's exponential moving average of model loss
versus the zero-prediction baseline. Skill above one means the row's forecasts
beat predicting no move, which is the evidence the horizon selector contracts on.
*/
func (resonanceManifold *ResonanceManifold) updateTaskSkill(
	rowIndex int,
	target float64,
	modelSquaredError float64,
) {
	baselineSquaredError := target * target
	taskModelLossData := resonanceManifold.taskModelLoss.RawVector().Data
	taskBaselineLossData := resonanceManifold.taskBaselineLoss.RawVector().Data

	if !resonanceManifold.taskSkillReady[rowIndex] {
		taskModelLossData[rowIndex] = modelSquaredError
		taskBaselineLossData[rowIndex] = baselineSquaredError
		resonanceManifold.taskSkillReady[rowIndex] = true

		return
	}

	beta := resonanceManifold.cfg.PrecisionBeta
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
		numerator := resonanceManifold.cfg.PrecisionEps*lossScale + baselineLoss
		denominator := resonanceManifold.cfg.PrecisionEps*lossScale + modelLoss
		skill = math.Min(
			resonanceManifold.cfg.PrecisionMax,
			math.Max(resonanceManifold.cfg.PrecisionMin, numerator/denominator),
		)
	}

	resonanceManifold.taskSkill.RawVector().Data[rowIndex] = skill
}

func (resonanceManifold *ResonanceManifold) predictAdjacentLayers() ([]*mat.VecDense, []*mat.VecDense) {
	for layerIndex := 0; layerIndex < len(resonanceManifold.generativeWeights); layerIndex++ {
		prediction := resonanceManifold.workspace.predictions[layerIndex]
		prediction.MulVec(resonanceManifold.generativeWeights[layerIndex], resonanceManifold.latentStates[layerIndex+1])
		if layerIndex > 0 {
			denseApplyTanhInPlace(prediction)
		}

		layerError := resonanceManifold.workspace.errors[layerIndex]
		layerError.SubVec(resonanceManifold.latentStates[layerIndex], prediction)
	}

	return resonanceManifold.workspace.predictions, resonanceManifold.workspace.errors
}

func (resonanceManifold *ResonanceManifold) setAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		return fmt.Errorf(
			"%w: resonance: alpha must be finite and in (0, 1]",
			core.ErrDomain,
		)
	}

	newCfg := adaptiveResonanceConfig(alpha, resonanceManifold.arch)
	resonanceManifold.cfg.LrState = newCfg.LrState
	resonanceManifold.cfg.LrGenerative = newCfg.LrGenerative
	resonanceManifold.cfg.LrTemporal = newCfg.LrTemporal
	resonanceManifold.cfg.LrRecognition = newCfg.LrRecognition
	resonanceManifold.cfg.PrecisionBeta = newCfg.PrecisionBeta
	resonanceManifold.cfg.LatentDecay = newCfg.LatentDecay
	resonanceManifold.cfg.Sparsity = newCfg.Sparsity
	resonanceManifold.cfg.WeightDecay = newCfg.WeightDecay
	resonanceManifold.cfg.GradClip = newCfg.GradClip

	return nil
}

/*
readoutVectorInto writes the multi-layer readout [z_1..z_L, e_0..e_{L-1}]
directly into dst without heap allocation.
*/
func (resonanceManifold *ResonanceManifold) readoutVectorInto(dst []float64) int {
	_, layerErrors := resonanceManifold.predictAdjacentLayers()
	offset := 0

	if resonanceManifold.cfg.ReadoutMode == ReadoutAll || resonanceManifold.cfg.ReadoutMode == ReadoutLatents {
		for layerIndex := 1; layerIndex < len(resonanceManifold.latentStates); layerIndex++ {
			data := resonanceManifold.latentStates[layerIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	if resonanceManifold.cfg.ReadoutMode == ReadoutAll || resonanceManifold.cfg.ReadoutMode == ReadoutInnovations {
		for linkIndex := range layerErrors {
			data := layerErrors[linkIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	return offset
}

func (resonanceManifold *ResonanceManifold) readoutVector() []float64 {
	vector := make([]float64, resonanceManifold.readoutDim)
	resonanceManifold.readoutVectorInto(vector)
	return vector
}

func (resonanceManifold *ResonanceManifold) rolloutRetention(steps int) []float64 {
	if len(resonanceManifold.temporalOperators) == 0 || steps < 1 {
		return nil
	}

	numLatents := len(resonanceManifold.temporalOperators)
	currentLatents := make([]*mat.VecDense, numLatents)
	nextLatents := make([]*mat.VecDense, numLatents)
	for i := range numLatents {
		currentLatents[i] = mat.VecDenseCopyOf(resonanceManifold.latentStates[i+1])
		nextLatents[i] = mat.NewVecDense(resonanceManifold.arch[i+1], nil)
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
		} else {
			normSq := 0.0
			for i := range numLatents {
				norm := denseColNorm(currentLatents[i])
				normSq += norm * norm
			}
			retention[step] = math.Sqrt(normSq) / initialNorm
		}

		if step+1 < steps {
			for i := range numLatents {
				nextLatents[i].MulVec(resonanceManifold.temporalOperators[i], currentLatents[i])
				denseApplyTanhInPlace(nextLatents[i])
				currentLatents[i], nextLatents[i] = nextLatents[i], currentLatents[i]
			}
		}
	}

	return retention
}

/*
rolloutTaskForecast returns one forecast per task row, evaluated at the current
settled readout. Element k is the row for horizon k+1: the cumulative
directional prediction over the next k+1 ticks from now. Every element is the
supervised head for its own horizon, so the curve is a genuine multi-horizon
forecast rather than a trajectory through imagined states. A request beyond
the head's rows yields the head's rows.
*/
func (resonanceManifold *ResonanceManifold) rolloutTaskForecast(steps int) ([]RLSOutput, error) {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 || steps < 1 {
		return nil, nil
	}

	if steps > resonanceManifold.taskRows {
		steps = resonanceManifold.taskRows
	}

	readoutData := resonanceManifold.workspace.readoutBuf.RawVector().Data
	resonanceManifold.readoutVectorInto(readoutData)

	forecast := make([]RLSOutput, steps)

	for horizonIndex := range steps {
		output, err := taskForecast(resonanceManifold.taskLearners[horizonIndex], readoutData)

		if err != nil {
			return nil, fmt.Errorf("resonance: task forecast: %w", err)
		}

		forecast[horizonIndex] = output
	}

	return forecast, nil
}

/*
taskForecast evaluates one task-head row's learner on a feature vector without
updating its weights.
*/
func taskForecast(learner core.Primitive, features []float64) (RLSOutput, error) {
	evaluation := learner
	var reading algo.Reading

	for out := range evaluation.Next(sequence.NewValues(Sample{
		Features: features,
	}).Next(nil)) {
		reading = *(*algo.Reading)(out)
	}

	if err := evaluation.Error(); err != nil {
		return RLSOutput{}, err
	}

	return RLSOutput{
		Value:            reading.Prediction,
		Scale:            reading.Scale,
		DegreesOfFreedom: reading.DegreesOfFreedom,
		Ready:            reading.Ready,
		Innovation:       reading.Innovation,
	}, nil
}

/*
taskCoefficients projects the posterior into the dense manifold head.
*/
func taskCoefficients(reading algo.Reading, destination []float64) (float64, error) {
	if len(reading.Beta) != len(destination)+1 {
		return 0, fmt.Errorf(
			"resonance: coefficient width %d does not match head %d",
			len(reading.Beta),
			len(destination),
		)
	}

	copy(destination, reading.Beta[1:])
	return reading.Beta[0], nil
}
