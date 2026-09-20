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
	"math"
	"math/rand"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/types"

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
ResonanceManifold is a multi-timescale predictive coder implementing the
variational free energy principle over a generative neural architecture.
*/
type ResonanceManifoldServer struct {
	cfg        resonanceConfig
	arch       []int
	targetDim  int
	readoutDim int
	taskRows   int

	taskWeights      *mat.Dense
	taskBias         *mat.VecDense
	taskLearners     []*TaskLearnerServer
	taskVar          *mat.VecDense
	taskScale        *mat.VecDense
	taskPrecision    *mat.VecDense
	taskModelLoss    *mat.VecDense
	taskBaselineLoss *mat.VecDense
	taskSkill        *mat.VecDense
	taskScaleReady   []bool
	taskSkillReady   []bool

	workspace *resonanceWorkspace

	latentStates       []*mat.VecDense
	output             float64
	generativeWeights  []*mat.Dense
	recognitionWeights []*mat.Dense
	temporalOperators  []*mat.Dense

	precision         []*mat.VecDense
	errorVar          []*mat.VecDense
	temporalPrecision []*mat.VecDense
	temporalVar       []*mat.VecDense

	temporalPriorsReady    bool
	settleAdvancedTemporal bool
	lastInferenceSteps     int

	settlePipeline types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer]
	learnPipeline  types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer]
	err            error
}

type RLSOutput struct {
	Value            float64
	Scale            float64
	DegreesOfFreedom float64
	Ready            bool
	Innovation       float64
	Reset            bool
}

type ResonanceLayerWire struct {
	State      []float64 `json:"state"`
	Prediction []float64 `json:"prediction"`
	ErrorNorm  float64   `json:"errorNorm"`
	Temporal   bool      `json:"temporal"`
}

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

// ManifoldCommand orchestrates manifold execution.
type ManifoldCommand struct {
	Settle   *SettleIntent
	Forecast *ForecastIntent
	Batch    *BatchIntent
	Alpha    *AlphaIntent
	Reading  *ReadingIntent
	Retention *RetentionIntent
	ObserveTask *TaskIntent
}

type TaskIntent struct {
	Horizon    int
	Features   []float64
	Prediction float64
	Target     float64
}

type RetentionIntent struct {
	Steps int
}

type Sample struct {
	Features []float64
	Target   float64
	Observed bool
}

type AlphaIntent struct {
	Alpha float64
}

type ReadingIntent struct{}

type BatchIntent struct {
	Input           []float64
	Learn           bool
	AdvanceTemporal bool
}

type SettleIntent struct {
	Features        []float64
	Target          []float64
	AdvanceTemporal bool
}

type ForecastIntent struct {
	Steps int
}

// NewResonanceManifoldServer constructs a multi-layer predictive coder pipeline.
func NewResonanceManifoldServer(
	arch []int,
	taskRows int,
	targetDim int,
	alpha float64,
	readoutMode ReadoutMode,
) *ResonanceManifoldServer {
	cfg := adaptiveResonanceConfig(alpha, arch)
	cfg.ReadoutMode = readoutMode

	numLinks := len(arch) - 1
	numLatents := len(arch) - 1

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

	workspace := newResonanceWorkspace(arch, taskRows, cfg.ReadoutMode)

	m := &ResonanceManifoldServer{
		cfg:                cfg,
		arch:               arch,
		targetDim:          targetDim,
		readoutDim:         readoutDim,
		taskRows:           taskRows,
		workspace:          workspace,
		latentStates:       make([]*mat.VecDense, len(arch)),
		generativeWeights:  make([]*mat.Dense, numLinks),
		recognitionWeights: make([]*mat.Dense, numLinks),
		temporalOperators:  make([]*mat.Dense, numLatents),
		precision:          make([]*mat.VecDense, numLinks),
		errorVar:           make([]*mat.VecDense, numLinks),
		temporalPrecision:  make([]*mat.VecDense, numLatents),
		temporalVar:        make([]*mat.VecDense, numLatents),
	}

	for layerIndex, layerDim := range arch {
		m.latentStates[layerIndex] = mat.NewVecDense(layerDim, nil)
		if layerIndex > 0 {
			latentIndex := layerIndex - 1
			m.temporalOperators[latentIndex] = mat.NewDense(layerDim, layerDim, nil)
			m.temporalPrecision[latentIndex] = mat.NewVecDense(layerDim, nil)
			m.temporalVar[latentIndex] = mat.NewVecDense(layerDim, nil)
			denseFill(m.temporalPrecision[latentIndex], 1.0)
			denseFill(m.temporalVar[latentIndex], 1.0)
		}
	}

	for linkIndex := range numLinks {
		rowDim := arch[linkIndex]
		colDim := arch[linkIndex+1]
		m.generativeWeights[linkIndex] = mat.NewDense(rowDim, colDim, nil)
		m.recognitionWeights[linkIndex] = mat.NewDense(colDim, rowDim, nil)
		m.precision[linkIndex] = mat.NewVecDense(rowDim, nil)
		m.errorVar[linkIndex] = mat.NewVecDense(rowDim, nil)
		denseFill(m.precision[linkIndex], 1.0)
		denseFill(m.errorVar[linkIndex], 1.0)

		for i := 0; i < rowDim; i++ {
			for j := 0; j < colDim; j++ {
				m.generativeWeights[linkIndex].Set(i, j, (rand.Float64()-0.5)*0.1)
				m.recognitionWeights[linkIndex].Set(j, i, (rand.Float64()-0.5)*0.1)
			}
		}
	}

	if taskRows > 0 {
		m.taskWeights = mat.NewDense(taskRows, readoutDim, nil)
		m.taskBias = mat.NewVecDense(taskRows, nil)
		m.taskVar = mat.NewVecDense(taskRows, nil)
		m.taskScale = mat.NewVecDense(taskRows, nil)
		m.taskPrecision = mat.NewVecDense(taskRows, nil)
		m.taskModelLoss = mat.NewVecDense(taskRows, nil)
		m.taskBaselineLoss = mat.NewVecDense(taskRows, nil)
		m.taskSkill = mat.NewVecDense(taskRows, nil)
		m.taskScaleReady = make([]bool, taskRows)
		m.taskSkillReady = make([]bool, taskRows)

		denseFill(m.taskVar, 1.0)
		denseFill(m.taskPrecision, 1.0)
		denseFill(m.taskSkill, 1.0)

		m.taskLearners = make([]*TaskLearnerServer, taskRows)
		for i := range taskRows {
			m.taskLearners[i] = NewTaskLearnerServer(readoutDim, cfg.LambdaRLS)
		}
	}

	m.settlePipeline = m.buildSettlePipeline()
	m.learnPipeline = m.buildLearnPipeline()

	return m
}

// Execute processes a single ManifoldCommand and returns the resulting ManifoldReading.
func (m *ResonanceManifoldServer) Execute(cmd ManifoldCommand) (ManifoldReading, error) {
	if cmd.Settle != nil {
		if len(cmd.Settle.Features) != m.arch[0] {
			m.err = fmt.Errorf("resonance: input dimension mismatch")
			return ManifoldReading{}, m.err
		}

		xCol := m.workspace.xCol
		copy(xCol.RawVector().Data, cmd.Settle.Features)
		m.initializeLatents(xCol)

		m.settleAdvancedTemporal = false
		m.settlePipeline(m)

		if cmd.Settle.AdvanceTemporal {
			m.advanceTemporalState()
			m.settleAdvancedTemporal = true
		}

		if cmd.Settle.Target != nil {
			if m.settleAdvancedTemporal {
				m.err = fmt.Errorf("resonance: temporal state advanced before learning")
				return ManifoldReading{}, m.err
			}
			if len(cmd.Settle.Target) != m.targetDim {
				m.err = fmt.Errorf("resonance: target dimension mismatch")
				return ManifoldReading{}, m.err
			}

			if m.taskWeights != nil {
				yCol := m.workspace.yCol
				copy(yCol.RawVector().Data, cmd.Settle.Target)
			}

			m.learnPipeline(m)
		}

		return m.snapshot(), nil
	}

	if cmd.Forecast != nil {
		if cmd.Forecast.Steps < 1 {
			m.err = fmt.Errorf("resonance: task forecast requires a positive step count")
			return ManifoldReading{}, m.err
		}
		forecast, err := m.rolloutTaskForecast(cmd.Forecast.Steps)
		if err != nil {
			m.err = err
			return ManifoldReading{}, err
		}
		return ManifoldReading{Forecast: forecast}, nil
	}

	if cmd.Retention != nil {
		return ManifoldReading{Retention: m.retentionVector()}, nil
	}

	if cmd.Alpha != nil {
		if err := m.setAlpha(cmd.Alpha.Alpha); err != nil {
			m.err = err
			return ManifoldReading{}, err
		}
		return m.snapshot(), nil
	}

	if cmd.Reading != nil {
		return m.snapshot(), nil
	}

	if cmd.Batch != nil {
		if len(cmd.Batch.Input) != m.arch[0] {
			m.err = fmt.Errorf("resonance: input dimension mismatch")
			return ManifoldReading{}, m.err
		}
		xCol := m.workspace.xCol
		copy(xCol.RawVector().Data, cmd.Batch.Input)
		m.initializeLatents(xCol)

		m.settleAdvancedTemporal = false
		m.settlePipeline(m)

		if cmd.Batch.AdvanceTemporal {
			m.advanceTemporalState()
			m.settleAdvancedTemporal = true
		}

		if cmd.Batch.Learn {
			m.learnPipeline(m)
		}

		return m.snapshot(), nil
	}

	if cmd.ObserveTask != nil {
		err := m.observeTask(cmd.ObserveTask.Horizon, cmd.ObserveTask.Features, cmd.ObserveTask.Prediction, cmd.ObserveTask.Target)
		if err != nil {
			m.err = err
			return ManifoldReading{}, err
		}
		return m.snapshot(), nil
	}

	return m.snapshot(), nil
}

// AsValue returns a types.Value closure for executing commands.
func (m *ResonanceManifoldServer) AsValue() types.Value[ManifoldCommand, ManifoldReading] {
	return func(cmd ManifoldCommand) ManifoldReading {
		reading, _ := m.Execute(cmd)
		return reading
	}
}

// Error returns any errors encountered during processing.
func (m *ResonanceManifoldServer) Error() error {
	return m.err
}

/*
snapshot assembles the manifold's full reading: settled dynamics, harvested
readout, latent state, supervised head predictions, wire layers, and the
per-row and averaged reliability of the task head.
*/
func (resonanceManifold *ResonanceManifoldServer) snapshot() ManifoldReading {
	reading := ManifoldReading{
		Reconstruction:      resonanceManifold.output,
		ReconstructionError: resonanceManifold.reconstructionError(),
		ReadoutDimension:    resonanceManifold.readoutDim,
		Readout:             resonanceManifold.readoutVector(),
		Latent:              resonanceManifold.latentState(),
		TaskPrediction:      resonanceManifold.taskPrediction(),
		Retention:           resonanceManifold.retentionVector(),
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

func (resonanceManifold *ResonanceManifoldServer) retentionVector() []float64 {
	return nil
}

func (resonanceManifold *ResonanceManifoldServer) resetState(resetPrecision bool) {
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
energy is the variational free energy combining precision-weighted error,
multi-timescale temporal priors, $L_2$ decay, and $L_1$ dictionary sparsity.
*/
func (resonanceManifold *ResonanceManifoldServer) energy() float64 {
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
func (resonanceManifold *ResonanceManifoldServer) predictionEnergy() float64 {
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

func (resonanceManifold *ResonanceManifoldServer) reconstructionError() float64 {
	reconstruction := resonanceManifold.workspace.reconPred
	reconstruction.MulVec(resonanceManifold.generativeWeights[0], resonanceManifold.latentStates[1])
	// Layer 0 is linear (continuous unbounded z-scores)

	diff := resonanceManifold.workspace.reconDiff
	diff.SubVec(resonanceManifold.latentStates[0], reconstruction)

	return denseColNorm(diff)
}

func (resonanceManifold *ResonanceManifoldServer) taskPredictionInto(dst *mat.VecDense) {
	readoutData := resonanceManifold.workspace.readoutBuf.RawVector().Data
	resonanceManifold.readoutVectorInto(readoutData)

	dst.MulVec(resonanceManifold.taskWeights, resonanceManifold.workspace.readoutBuf)
	dst.AddVec(dst, resonanceManifold.taskBias)
}

func (resonanceManifold *ResonanceManifoldServer) taskPrediction() []float64 {
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
func (resonanceManifold *ResonanceManifoldServer) taskReading(
	rowIndex int,
	features []float64,
	target float64,
) (algo.RLSPosterior, error) {
	if rowIndex < 0 || rowIndex >= len(resonanceManifold.taskLearners) {
		return algo.RLSPosterior{}, fmt.Errorf("resonance: invalid task row %d", rowIndex)
	}
	evaluation := resonanceManifold.taskLearners[rowIndex]
	reading := evaluation.Evaluate(features, target, true)
	return reading, nil
}

/*
observeTask updates one task-head row from one labeled sample. The row is
addressed by its forward horizon, one-based: horizon h supervises the
cumulative move over the next h ticks.
*/
func (resonanceManifold *ResonanceManifoldServer) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
) error {
	if resonanceManifold.taskWeights == nil || resonanceManifold.taskRows <= 0 {
		return fmt.Errorf("resonance: supervised task head required (shape mismatch)")
	}

	if horizon < 1 || horizon > resonanceManifold.taskRows {
		return fmt.Errorf("resonance: task horizon %d out of range [1, %d] (domain mismatch)", horizon, resonanceManifold.taskRows)
	}

	if len(features) != resonanceManifold.readoutDim {
		return fmt.Errorf("resonance: expected %d task features, got %d (shape mismatch)", resonanceManifold.readoutDim, len(features))
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

func (resonanceManifold *ResonanceManifoldServer) latentState() []float64 {
	if len(resonanceManifold.latentStates) == 0 {
		return nil
	}

	return append([]float64(nil), resonanceManifold.latentStates[len(resonanceManifold.latentStates)-1].RawVector().Data...)
}

func (resonanceManifold *ResonanceManifoldServer) temporalError() (float64, bool) {
	if !resonanceManifold.temporalPriorsReady || len(resonanceManifold.temporalOperators) == 0 {
		return 0, false
	}

	topLatentIdx := len(resonanceManifold.temporalOperators) - 1
	temporalError := resonanceManifold.workspace.temporalErrors[topLatentIdx]
	return denseColNorm(temporalError), true
}

func (resonanceManifold *ResonanceManifoldServer) wireSnapshot() (
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

func (resonanceManifold *ResonanceManifoldServer) taskPrecisionAverage() (float64, bool) {
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

func (resonanceManifold *ResonanceManifoldServer) taskSkillAverage() (float64, bool) {
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

func (resonanceManifold *ResonanceManifoldServer) taskScaleAverage() (float64, bool) {
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

func (resonanceManifold *ResonanceManifoldServer) stateGradients(
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

func (resonanceManifold *ResonanceManifoldServer) initializeLatents(xCol *mat.VecDense) {
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

func (resonanceManifold *ResonanceManifoldServer) advanceTemporalState() {
	for latentIndex := range resonanceManifold.temporalOperators {
		layerIndex := latentIndex + 1
		resonanceManifold.workspace.prevLatents[latentIndex].CopyVec(resonanceManifold.latentStates[layerIndex])
	}
	resonanceManifold.temporalPriorsReady = true
}

func (resonanceManifold *ResonanceManifoldServer) precisionFor(layerIndex int) *mat.VecDense {
	return resonanceManifold.precision[layerIndex]
}

func (resonanceManifold *ResonanceManifoldServer) projectTemporalOperatorNorm(latentIndex int) error {
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

func (resonanceManifold *ResonanceManifoldServer) saveStates() {
	for layerIndex, latent := range resonanceManifold.latentStates {
		resonanceManifold.workspace.savedStates[layerIndex].CopyVec(latent)
	}
}

func (resonanceManifold *ResonanceManifoldServer) restoreStates() {
	for layerIndex, latent := range resonanceManifold.latentStates {
		latent.CopyVec(resonanceManifold.workspace.savedStates[layerIndex])
	}
}

func (resonanceManifold *ResonanceManifoldServer) tryStateUpdate(gradients []*mat.VecDense, stepSize float64) {
	for layerIndex := 1; layerIndex < len(resonanceManifold.latentStates); layerIndex++ {
		step := resonanceManifold.workspace.stepBuf[layerIndex]
		step.ScaleVec(stepSize, gradients[layerIndex])
		nextState := resonanceManifold.latentStates[layerIndex]
		nextState.SubVec(resonanceManifold.workspace.savedStates[layerIndex], step)
		denseClipColInPlace(nextState, resonanceManifold.cfg.StateClip)
	}
}

func (resonanceManifold *ResonanceManifoldServer) updatePrecision(
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
func (resonanceManifold *ResonanceManifoldServer) updateTaskReliability(
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
func (resonanceManifold *ResonanceManifoldServer) updateTaskSkill(
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

func (resonanceManifold *ResonanceManifoldServer) predictAdjacentLayers() ([]*mat.VecDense, []*mat.VecDense) {
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

func (resonanceManifold *ResonanceManifoldServer) setAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return fmt.Errorf("resonance: alpha must be finite and in (0, 1] (domain mismatch)")
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
func (resonanceManifold *ResonanceManifoldServer) readoutVectorInto(dst []float64) int {
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

func (resonanceManifold *ResonanceManifoldServer) readoutVector() []float64 {
	vector := make([]float64, resonanceManifold.readoutDim)
	resonanceManifold.readoutVectorInto(vector)
	return vector
}

func (resonanceManifold *ResonanceManifoldServer) rolloutRetention(steps int) []float64 {
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
func (resonanceManifold *ResonanceManifoldServer) rolloutTaskForecast(steps int) ([]RLSOutput, error) {
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
func taskForecast(learner *TaskLearnerServer, features []float64) (RLSOutput, error) {
	if learner == nil {
		return RLSOutput{}, fmt.Errorf("resonance: learner is nil")
	}
	reading := learner.Evaluate(features, 0.0, false)

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
func taskCoefficients(reading algo.RLSPosterior, destination []float64) (float64, error) {
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

/*
buildSettlePipeline constructs the pure algebraic pipeline for settling the manifold.
It leverages nomagique.IterateUntil to replace manual for loops, preserving the
zero-allocation Gonum workspace performance.
*/
func (resonanceManifold *ResonanceManifoldServer) buildSettlePipeline() types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer] {
	var settledEnergy float64
	var stableSteps int

	calcGradients := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		predictions, layerErrors := m.predictAdjacentLayers()
		m.stateGradients(predictions, layerErrors)
		return m
	}

	lineSearch := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		m.saveStates()
		accepted := false
		candidateEnergy := settledEnergy
		stepSize := m.cfg.LrState

		halvings := 0
		if m.cfg.MonotoneStateSteps {
			halvings = m.cfg.LineSearchHalvings
		}

		for halvingIndex := 0; halvingIndex <= halvings; halvingIndex++ {
			m.tryStateUpdate(m.workspace.grads, stepSize)
			m.latentStates[0].CopyVec(m.workspace.xCol)
			candidateEnergy = m.energy()

			if !m.cfg.MonotoneStateSteps || candidateEnergy <= math.Nextafter(settledEnergy, math.Inf(1)) {
				accepted = true
				break
			}

			m.restoreStates()
			stepSize *= 0.5
		}

		if !accepted {
			m.restoreStates()
			m.latentStates[0].CopyVec(m.workspace.xCol)
			stableSteps = 0
			return m
		}

		deltaEnergy := math.Abs(settledEnergy - candidateEnergy)
		energyScale := math.Max(math.Abs(settledEnergy), m.cfg.PrecisionEps)
		relativeDelta := deltaEnergy / energyScale
		settledEnergy = candidateEnergy

		if m.lastInferenceSteps < m.cfg.MinInferenceSteps || relativeDelta >= m.cfg.EarlyStopTol {
			stableSteps = 0
		} else {
			stableSteps++
		}
		return m
	}

	stepPipeline := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		m.lastInferenceSteps++
		m = calcGradients(m)
		return lineSearch(m)
	}

	condition := types.Value[*ResonanceManifoldServer, bool](func(m *ResonanceManifoldServer) bool {
		return stableSteps >= m.cfg.EarlyStopPatience
	})

	iterate := nomagique.IterateUntil(resonanceManifold.cfg.MaxInferenceSteps, condition, stepPipeline)

	return func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		settledEnergy = m.energy()
		stableSteps = 0
		m.lastInferenceSteps = 0
		return iterate(m)
	}
}

/*
buildLearnPipeline constructs the pure algebraic pipeline for weight updates.
*/
func (resonanceManifold *ResonanceManifoldServer) buildLearnPipeline() types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer] {
	var predictions, layerErrors []*mat.VecDense

	predict := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		predictions, layerErrors = m.predictAdjacentLayers()
		return m
	}

	generativeUpdate := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		for layerIndex, weightMatrix := range m.generativeWeights {
			localSignal := m.workspace.localSignal[layerIndex]
			if layerIndex == 0 {
				for i := 0; i < localSignal.Len(); i++ {
					localSignal.SetVec(i, 1.0)
				}
			}
			if layerIndex > 0 {
				denseApplyOneMinusSquareInto(localSignal, predictions[layerIndex])
			}
			precision := m.precisionFor(layerIndex)
			localSignal.MulElemVec(localSignal, layerErrors[layerIndex])
			localSignal.MulElemVec(localSignal, precision)

			update := m.workspace.weightUpdate[layerIndex]
			denseOuterColsInto(update, localSignal, m.latentStates[layerIndex+1], 1.0)

			scale := m.cfg.LrGenerative
			if norm := mat.Norm(update, 2); norm > m.cfg.GradClip {
				scale *= m.cfg.GradClip / norm
			}

			denseScaleInPlace(update, scale)
			weightMatrix.Add(weightMatrix, update)

			if m.cfg.WeightDecay > 0 {
				denseScaleInPlace(weightMatrix, 1.0-m.cfg.LrGenerative*m.cfg.WeightDecay)
			}
		}
		return m
	}

	recognitionUpdate := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		for layerIndex, recognitionMatrix := range m.recognitionWeights {
			proposal := m.workspace.recProposal[layerIndex]
			proposal.MulVec(recognitionMatrix, m.latentStates[layerIndex])
			denseApplyTanhInPlace(proposal)

			recError := m.workspace.recError[layerIndex]
			recError.SubVec(m.latentStates[layerIndex+1], proposal)

			recSignal := m.workspace.recSignal[layerIndex]
			denseApplyOneMinusSquareInto(recSignal, proposal)
			recSignal.MulElemVec(recSignal, recError)

			update := m.workspace.recUpdate[layerIndex]
			denseOuterColsInto(update, recSignal, m.latentStates[layerIndex], 1.0)

			scale := m.cfg.LrRecognition
			if norm := mat.Norm(update, 2); norm > m.cfg.GradClip {
				scale *= m.cfg.GradClip / norm
			}

			denseScaleInPlace(update, scale)
			recognitionMatrix.Add(recognitionMatrix, update)

			if m.cfg.WeightDecay > 0 {
				denseScaleInPlace(recognitionMatrix, 1.0-m.cfg.LrRecognition*m.cfg.WeightDecay)
			}
		}
		return m
	}

	temporalUpdate := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		if m.temporalPriorsReady {
			for latentIndex, operator := range m.temporalOperators {
				layerIndex := latentIndex + 1
				temporalError := m.workspace.temporalErrors[latentIndex]
				temporalError.SubVec(m.latentStates[layerIndex], m.workspace.temporalPriors[latentIndex])

				temporalSignal := m.workspace.temporalSignals[latentIndex]
				denseApplyOneMinusSquareInto(temporalSignal, m.workspace.temporalPriors[latentIndex])
				precision := m.temporalPrecision[latentIndex]
				temporalSignal.MulElemVec(temporalSignal, temporalError)
				temporalSignal.MulElemVec(temporalSignal, precision)
				temporalSignal.ScaleVec(m.cfg.TemporalWeights[latentIndex], temporalSignal)

				update := m.workspace.temporalUpdates[latentIndex]
				denseOuterColsInto(update, temporalSignal, m.workspace.prevLatents[latentIndex], 1.0)

				scale := m.cfg.LrTemporal
				if norm := mat.Norm(update, 2); norm > m.cfg.GradClip {
					scale *= m.cfg.GradClip / norm
				}

				denseScaleInPlace(update, scale)
				operator.Add(operator, update)

				if m.cfg.WeightDecay > 0 {
					denseScaleInPlace(operator, 1.0-m.cfg.LrTemporal*m.cfg.WeightDecay)
				}
				_ = m.projectTemporalOperatorNorm(latentIndex)
			}
		}
		return m
	}

	taskUpdate := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		targetCol := m.workspace.yCol
		trainedRows := 0
		if m.taskWeights != nil {
			trainedRows = targetCol.Len()
			if trainedRows > m.taskRows {
				trainedRows = m.taskRows
			}

			taskPred := m.workspace.taskPred
			m.taskPredictionInto(taskPred)

			taskError := m.workspace.taskError
			taskError.SubVec(targetCol, taskPred)

			readoutData := m.workspace.readoutBuf.RawVector().Data
			m.readoutVectorInto(readoutData)
			targetData := targetCol.RawVector().Data
			biasData := m.taskBias.RawVector().Data

			for rowIndex := range trainedRows {
				reading, _ := m.taskReading(rowIndex, readoutData, targetData[rowIndex])
				intercept, _ := taskCoefficients(reading, m.taskWeights.RawRowView(rowIndex))
				biasData[rowIndex] = intercept
			}
		}
		return m
	}

	precisionUpdate := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		targetCol := m.workspace.yCol
		taskError := m.workspace.taskError
		trainedRows := targetCol.Len()
		_ = m.updatePrecision(layerErrors, m.workspace.temporalErrors, targetCol, taskError, trainedRows)
		return m
	}

	advance := func(m *ResonanceManifoldServer) *ResonanceManifoldServer {
		m.advanceTemporalState()
		return m
	}

	return types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](nomagique.NewNumber(
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](predict),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](generativeUpdate),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](recognitionUpdate),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](temporalUpdate),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](taskUpdate),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](precisionUpdate),
		types.Value[*ResonanceManifoldServer, *ResonanceManifoldServer](advance),
	))
}
