package store

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/algo"

	"gonum.org/v1/gonum/mat"
)

/*
resonanceReadout defines which representation components are harvested into the
downstream task readout and feature extraction vector.
*/
type resonanceReadout uint8

const (
	// readoutAll concatenates [z_1, ..., z_L, e_0, ..., e_{L-1}].
	readoutAll resonanceReadout = iota
	// readoutLatents concatenates only the settled latents [z_1, ..., z_L].
	readoutLatents
	// readoutInnovations concatenates only the prediction error residuals [e_0, ..., e_{L-1}].
	readoutInnovations
)

/*
resonanceConfig sizes numerical inference from the representation and derives
learning gain from observed support.
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

	resonanceReadout resonanceReadout
}

/*
adaptiveResonanceConfig derives numerical work and gains from observed
support and representation dimensions. Stopping tolerance is
floating-point resolution; update gains are normalized by observed source energy.
*/
func adaptiveResonanceConfig(alpha float64, arch []int) resonanceConfig {
	weights := make([]float64, len(arch)-1)
	dimensions := 0
	for index := range weights {
		weights[index] = 1
		dimensions += arch[index+1]
	}
	// IEEE-754 resolution and the representation's dimension bound numerical work;
	// neither sets a market horizon, retained sample count or confidence threshold.
	resolution := math.Nextafter(1, 2) - 1
	return resonanceConfig{
		MaxInferenceSteps: dimensions, MinInferenceSteps: 1, LrState: alpha,
		EarlyStopTol: math.Sqrt(resolution), EarlyStopPatience: 1,
		MonotoneStateSteps: true, LineSearchHalvings: 53,
		LrGenerative: alpha, LrTemporal: alpha, LrRecognition: alpha,
		TemporalWeights: weights,

		resonanceReadout: readoutLatents,
	}
}

/*
resonanceManifold owns the learned generative, recognition and temporal matrices
and settles their squared prediction-error objective.
*/
type resonanceManifold struct {
	cfg        resonanceConfig
	arch       []int
	readoutDim int
	taskRows   int

	taskWeights      *mat.Dense
	taskBias         *mat.VecDense
	taskLearners     []*resonanceTask
	taskVar          *mat.VecDense
	taskScale        *mat.VecDense
	taskPrecision    *mat.VecDense
	taskModelLoss    *mat.VecDense
	taskBaselineLoss *mat.VecDense
	taskSkill        *mat.VecDense
	taskScaleReady   []bool
	taskSkillReady   []bool
	taskSupport      []int

	workspace *resonanceWorkspace

	latentStates       []*mat.VecDense
	generativeWeights  []*mat.Dense
	recognitionWeights []*mat.Dense
	temporalOperators  []*mat.Dense

	temporalPriorsReady bool
	lastInferenceSteps  int

	settlePipeline func(*resonanceManifold) *resonanceManifold
	learnPipeline  func(*resonanceManifold) *resonanceManifold
	err            error
}

type resonanceForecast struct {
	Value            float64
	Scale            float64
	DegreesOfFreedom float64
	Ready            bool
	Innovation       float64
	Reset            bool
}

type resonanceLayer struct {
	State      []float64 `json:"state"`
	Prediction []float64 `json:"prediction"`
	ErrorNorm  float64   `json:"errorNorm"`
	Temporal   bool      `json:"temporal"`
}

type resonanceReading struct {
	Reconstruction      []float64
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
	Layers           []resonanceLayer

	Skill             []float64
	SkillReady        []bool
	SkillAverage      float64
	SkillReadyAvg     bool
	PrecisionReady    []bool
	PrecisionAverage  float64
	PrecisionReadyAvg bool
	ScaleAverage      float64
	ScaleReadyAvg     bool

	Forecast  []resonanceForecast
	Retention []float64
}

// resonanceCommand orchestrates manifold execution.
type resonanceCommand struct {
	Forecast  *resonanceQuery
	Batch     *resonanceBatch
	Alpha     *resonanceAlpha
	Reading   *resonanceInspect
	Retention *resonanceRetention
}

type resonanceRetention struct {
	Steps int
}

type resonanceAlpha struct {
	Alpha float64
}

type resonanceInspect struct{}

type resonanceBatch struct {
	Input           []float64
	Learn           bool
	AdvanceTemporal bool
}

type resonanceQuery struct {
	Steps int
}

// newResonanceManifold constructs a multi-layer predictive coder pipeline.
func newResonanceManifold(
	arch []int,
	taskRows int,
	alpha float64,
	readoutMode resonanceReadout,
) *resonanceManifold {
	cfg := adaptiveResonanceConfig(alpha, arch)
	cfg.resonanceReadout = readoutMode

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

	switch cfg.resonanceReadout {
	case readoutAll:
		readoutDim = totalLatentDim + totalErrorDim
	case readoutLatents:
		readoutDim = totalLatentDim
	case readoutInnovations:
		readoutDim = totalErrorDim
	}

	workspace := newResonanceWorkspace(arch, taskRows, cfg.resonanceReadout)

	m := &resonanceManifold{
		cfg:                cfg,
		arch:               arch,
		readoutDim:         readoutDim,
		taskRows:           taskRows,
		workspace:          workspace,
		latentStates:       make([]*mat.VecDense, len(arch)),
		generativeWeights:  make([]*mat.Dense, numLinks),
		recognitionWeights: make([]*mat.Dense, numLinks),
		temporalOperators:  make([]*mat.Dense, numLatents),
	}

	for layerIndex, layerDim := range arch {
		m.latentStates[layerIndex] = mat.NewVecDense(layerDim, nil)
		if layerIndex > 0 {
			latentIndex := layerIndex - 1
			m.temporalOperators[latentIndex] = mat.NewDense(layerDim, layerDim, nil)
		}
	}

	for linkIndex := range numLinks {
		rowDim := arch[linkIndex]
		colDim := arch[linkIndex+1]
		m.generativeWeights[linkIndex] = mat.NewDense(rowDim, colDim, nil)
		m.recognitionWeights[linkIndex] = mat.NewDense(colDim, rowDim, nil)

		for i := 0; i < rowDim; i++ {
			for j := 0; j < colDim; j++ {
				// Orthonormal cosine coordinates establish a deterministic basis.
				// The normalization follows the DCT-II identity, not a market prior.
				scale := math.Sqrt(2 / float64(rowDim))
				if j == 0 {
					scale = 1 / math.Sqrt(float64(rowDim))
				}
				basis := scale * math.Cos(math.Pi*(float64(i)+.5)*float64(j)/float64(rowDim))
				m.generativeWeights[linkIndex].Set(i, j, basis)
				m.recognitionWeights[linkIndex].Set(j, i, basis)
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
		m.taskSupport = make([]int, taskRows)

		m.taskLearners = make([]*resonanceTask, taskRows)
		for i := range taskRows {
			m.taskLearners[i] = newResonanceTask(readoutDim)
		}
	}

	m.settlePipeline = m.buildSettlePipeline()
	m.learnPipeline = m.buildLearnPipeline()

	return m
}

// Execute processes a single resonanceCommand and returns the resulting resonanceReading.
func (m *resonanceManifold) Execute(cmd resonanceCommand) (resonanceReading, error) {

	if cmd.Forecast != nil {
		if cmd.Forecast.Steps < 1 {
			m.err = fmt.Errorf("resonance: task forecast requires a positive step count")
			return resonanceReading{}, m.err
		}
		forecast, err := m.rolloutTaskForecast(cmd.Forecast.Steps)
		if err != nil {
			m.err = err
			return resonanceReading{}, err
		}
		return resonanceReading{Forecast: forecast}, nil
	}

	if cmd.Retention != nil {
		return resonanceReading{Retention: m.retentionVector()}, nil
	}

	if cmd.Alpha != nil {
		if err := m.setAlpha(cmd.Alpha.Alpha); err != nil {
			m.err = err
			return resonanceReading{}, err
		}
		return m.snapshot(), nil
	}

	if cmd.Reading != nil {
		return m.snapshot(), nil
	}

	if cmd.Batch != nil {
		if len(cmd.Batch.Input) != m.arch[0] {
			m.err = fmt.Errorf("resonance: input dimension mismatch")
			return resonanceReading{}, m.err
		}
		xCol := m.workspace.xCol
		copy(xCol.RawVector().Data, cmd.Batch.Input)
		m.initializeLatents(xCol)
		m.settlePipeline(m)

		if cmd.Batch.AdvanceTemporal {
			m.advanceTemporalState()
		}

		if cmd.Batch.Learn {
			m.learnPipeline(m)
		}

		return m.snapshot(), m.err
	}

	return m.snapshot(), nil
}

// Error returns any errors encountered during processing.
func (m *resonanceManifold) Error() error {
	return m.err
}

/*
snapshot assembles the manifold's full reading: settled dynamics, harvested
readout, latent state, supervised head predictions, wire layers, and the
per-row and averaged reliability of the task head.
*/
func (manifold *resonanceManifold) snapshot() resonanceReading {
	reading := resonanceReading{
		ReconstructionError: manifold.reconstructionError(),
		ReadoutDimension:    manifold.readoutDim,
		Readout:             manifold.readoutVector(),
		Latent:              manifold.latentState(),
		TaskPrediction:      manifold.taskPrediction(),
		Retention:           manifold.retentionVector(),
	}

	reading.Energy = manifold.energy()
	reading.PredictionEnergy = manifold.predictionEnergy()
	reading.TemporalError, reading.HasTemporalError = manifold.temporalError()
	reading.Layers, reading.Surprise, reading.EnergyDensity = manifold.wireSnapshot()
	reading.Reconstruction = reading.Layers[0].Prediction

	if manifold.taskRows > 0 {
		reading.Skill = append([]float64(nil), manifold.taskSkill.RawVector().Data...)
		reading.SkillReady = append([]bool(nil), manifold.taskSkillReady...)
		reading.PrecisionReady = append([]bool(nil), manifold.taskScaleReady...)
		reading.SkillAverage, reading.SkillReadyAvg = manifold.taskSkillAverage()
		reading.PrecisionAverage, reading.PrecisionReadyAvg = manifold.taskPrecisionAverage()
		reading.ScaleAverage, reading.ScaleReadyAvg = manifold.taskScaleAverage()
	}

	return reading
}

func (manifold *resonanceManifold) retentionVector() []float64 {
	values := make([]float64, len(manifold.temporalOperators))
	for index, operator := range manifold.temporalOperators {
		width, _ := operator.Dims()
		norm := mat.Norm(operator, 2)
		values[index] = norm * norm / float64(width)
	}
	return values
}

/*
energy is the sum of squared generative and temporal innovations.
*/
func (manifold *resonanceManifold) energy() float64 { return manifold.predictionEnergy() }

/*
predictionEnergy computes squared reconstruction and temporal errors across all links.
*/
func (manifold *resonanceManifold) predictionEnergy() float64 {
	_, errors := manifold.predictAdjacentLayers()
	energy := 0.0
	for _, residual := range errors {
		energy += denseColDot(residual, residual) / 2
	}
	if !manifold.temporalPriorsReady {
		return energy
	}
	for index := range manifold.temporalOperators {
		residual := manifold.workspace.temporalErrors[index]
		residual.SubVec(manifold.latentStates[index+1], manifold.workspace.temporalPriors[index])
		energy += denseColDot(residual, residual) / 2
	}
	return energy
}

func (manifold *resonanceManifold) reconstructionError() float64 {
	reconstruction := manifold.workspace.reconPred
	reconstruction.MulVec(manifold.generativeWeights[0], manifold.latentStates[1])
	// Layer 0 is linear (continuous unbounded z-scores)

	diff := manifold.workspace.reconDiff
	diff.SubVec(manifold.latentStates[0], reconstruction)

	return denseColNorm(diff)
}

func (manifold *resonanceManifold) taskPredictionInto(dst *mat.VecDense) {
	readoutData := manifold.workspace.readoutBuf.RawVector().Data
	manifold.readoutVectorInto(readoutData)

	dst.MulVec(manifold.taskWeights, manifold.workspace.readoutBuf)
	dst.AddVec(dst, manifold.taskBias)
}

func (manifold *resonanceManifold) taskPrediction() []float64 {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 {
		return nil
	}

	taskPred := manifold.workspace.taskPred
	manifold.taskPredictionInto(taskPred)
	return append([]float64(nil), taskPred.RawVector().Data...)
}

/*
taskReading drives one task-head row's learner with one labeled sample and
returns its posterior reading.
*/
func (manifold *resonanceManifold) taskReading(
	rowIndex int,
	features []float64,
	target float64,
) (algo.RLSPosterior, error) {
	if rowIndex < 0 || rowIndex >= len(manifold.taskLearners) {
		return algo.RLSPosterior{}, fmt.Errorf("resonance: invalid task row %d", rowIndex)
	}
	evaluation := manifold.taskLearners[rowIndex]
	reading := evaluation.Evaluate(features, target, true)
	return reading, nil
}

/*
observeTask updates one task-head row from one labeled sample. The row is
addressed by its forward horizon, one-based: horizon h supervises the
cumulative move over the next h ticks.
*/
func (manifold *resonanceManifold) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
	issued bool,
) error {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 {
		return fmt.Errorf("resonance: supervised task head required (shape mismatch)")
	}

	if horizon < 1 || horizon > manifold.taskRows {
		return fmt.Errorf("resonance: task horizon %d out of range [1, %d] (domain mismatch)", horizon, manifold.taskRows)
	}

	if len(features) != manifold.readoutDim {
		return fmt.Errorf("resonance: expected %d task features, got %d (shape mismatch)", manifold.readoutDim, len(features))
	}

	rowIndex := horizon - 1

	reading, err := manifold.taskReading(rowIndex, features, target)

	if err != nil {
		return fmt.Errorf("resonance: task learner update: %w", err)
	}

	intercept, err := taskCoefficients(reading, manifold.taskWeights.RawRowView(rowIndex))

	if err != nil {
		return fmt.Errorf("resonance: task learner coefficients: %w", err)
	}

	manifold.taskBias.RawVector().Data[rowIndex] = intercept

	if issued {
		manifold.updateTaskReliability(rowIndex, target, target-prediction)
	}

	return nil
}

func (manifold *resonanceManifold) latentState() []float64 {
	if len(manifold.latentStates) == 0 {
		return nil
	}

	return append([]float64(nil), manifold.latentStates[len(manifold.latentStates)-1].RawVector().Data...)
}

func (manifold *resonanceManifold) temporalError() (float64, bool) {
	if !manifold.temporalPriorsReady || len(manifold.temporalOperators) == 0 {
		return 0, false
	}

	topLatentIdx := len(manifold.temporalOperators) - 1
	temporalError := manifold.workspace.temporalErrors[topLatentIdx]
	return denseColNorm(temporalError), true
}

func (manifold *resonanceManifold) wireSnapshot() (
	layers []resonanceLayer,
	surprise float64,
	energyDensity float64,
) {
	predictions, layerErrors := manifold.predictAdjacentLayers()
	layers = make([]resonanceLayer, len(manifold.latentStates))
	topIndex := len(manifold.latentStates) - 1
	temporalNorm, hasTemporal := manifold.temporalError()

	for layerIndex := range manifold.latentStates {
		stateMatrix := manifold.latentStates[layerIndex]
		rowCount, _ := stateMatrix.Dims()
		state := append([]float64(nil), stateMatrix.RawVector().Data...)
		prediction := make([]float64, rowCount)

		if layerIndex < len(predictions) {
			copy(prediction, predictions[layerIndex].RawVector().Data)
		}

		if layerIndex == topIndex && manifold.temporalPriorsReady {
			copy(prediction, manifold.workspace.temporalPriors[len(manifold.temporalOperators)-1].RawVector().Data)
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

		layers[layerIndex] = resonanceLayer{
			State:      state,
			Prediction: prediction,
			ErrorNorm:  errorNorm,
			Temporal:   temporal,
		}
	}

	reconstructionDimensions := float64(manifold.arch[0])
	predictionDimensions := 0

	for _, layerError := range layerErrors {
		predictionDimensions += layerError.Len()
	}

	if hasTemporal {
		predictionDimensions += manifold.arch[topIndex]
	}

	return layers,
		manifold.reconstructionError() / math.Sqrt(reconstructionDimensions),
		manifold.predictionEnergy() / float64(predictionDimensions)
}

func (manifold *resonanceManifold) taskPrecisionAverage() (float64, bool) {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range manifold.taskRows {
		if manifold.taskScaleReady[rowIndex] {
			sum += manifold.taskPrecision.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (manifold *resonanceManifold) taskSkillAverage() (float64, bool) {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range manifold.taskRows {
		if manifold.taskSkillReady[rowIndex] {
			sum += manifold.taskSkill.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (manifold *resonanceManifold) taskScaleAverage() (float64, bool) {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 {
		return 0, false
	}

	var sum float64
	var readyCount int

	for rowIndex := range manifold.taskRows {
		if manifold.taskScaleReady[rowIndex] {
			sum += manifold.taskScale.RawVector().Data[rowIndex]
			readyCount++
		}
	}

	if readyCount == 0 {
		return 0, false
	}

	return sum / float64(readyCount), true
}

func (manifold *resonanceManifold) stateGradients(predictions, errors []*mat.VecDense) []*mat.VecDense {
	for layer := 1; layer < len(manifold.latentStates); layer++ {
		gradient := manifold.workspace.grads[layer]
		gradient.Zero()
		if layer < len(errors) {
			gradient.AddVec(gradient, errors[layer])
		}
		below := manifold.workspace.belowSignal[layer-1]
		denseFill(below, 1)
		if layer > 1 {
			denseApplyOneMinusSquareInto(below, predictions[layer-1])
		}
		below.MulElemVec(below, errors[layer-1])
		correction := manifold.workspace.correction[layer]
		denseMulWeightTransposeInto(correction, manifold.generativeWeights[layer-1], below)
		gradient.SubVec(gradient, correction)
		if manifold.temporalPriorsReady {
			residual := manifold.workspace.temporalErrors[layer-1]
			residual.SubVec(manifold.latentStates[layer], manifold.workspace.temporalPriors[layer-1])
			gradient.AddVec(gradient, residual)
		}
	}
	return manifold.workspace.grads
}

func (manifold *resonanceManifold) initializeLatents(xCol *mat.VecDense) {
	bottomUp := manifold.workspace.bottomUp
	bottomUp[0].CopyVec(xCol)

	for layerIndex := 0; layerIndex < len(manifold.recognitionWeights); layerIndex++ {
		proposal := bottomUp[layerIndex+1]
		proposal.MulVec(manifold.recognitionWeights[layerIndex], bottomUp[layerIndex])
		denseApplyTanhInPlace(proposal)
	}

	manifold.latentStates[0].CopyVec(xCol)

	if !manifold.temporalPriorsReady {
		for layerIndex := 1; layerIndex < len(manifold.latentStates); layerIndex++ {
			manifold.latentStates[layerIndex].CopyVec(bottomUp[layerIndex])
		}
		return
	}

	for latentIndex, operator := range manifold.temporalOperators {
		prior := manifold.workspace.temporalPriors[latentIndex]
		prior.MulVec(operator, manifold.workspace.prevLatents[latentIndex])
		denseApplyTanhInPlace(prior)
	}

	for layerIndex := 1; layerIndex < len(manifold.latentStates); layerIndex++ {
		manifold.latentStates[layerIndex].CopyVec(bottomUp[layerIndex])
	}
}

func (manifold *resonanceManifold) advanceTemporalState() {
	for latentIndex := range manifold.temporalOperators {
		layerIndex := latentIndex + 1
		manifold.workspace.prevLatents[latentIndex].CopyVec(manifold.latentStates[layerIndex])
	}
	manifold.temporalPriorsReady = true
}

func (manifold *resonanceManifold) saveStates() {
	for layerIndex, latent := range manifold.latentStates {
		manifold.workspace.savedStates[layerIndex].CopyVec(latent)
	}
}

func (manifold *resonanceManifold) restoreStates() {
	for layerIndex, latent := range manifold.latentStates {
		latent.CopyVec(manifold.workspace.savedStates[layerIndex])
	}
}

func (manifold *resonanceManifold) tryStateUpdate(gradients []*mat.VecDense, stepSize float64) {
	for layerIndex := 1; layerIndex < len(manifold.latentStates); layerIndex++ {
		step := manifold.workspace.stepBuf[layerIndex]
		step.ScaleVec(stepSize, gradients[layerIndex])
		nextState := manifold.latentStates[layerIndex]
		nextState.SubVec(manifold.workspace.savedStates[layerIndex], step)
	}
}

func (manifold *resonanceManifold) updateTaskReliability(rowIndex int, target, taskError float64) {
	manifold.taskSupport[rowIndex]++
	support := float64(manifold.taskSupport[rowIndex])
	model := manifold.taskModelLoss.RawVector().Data
	baseline := manifold.taskBaselineLoss.RawVector().Data
	model[rowIndex] += (taskError*taskError - model[rowIndex]) / support
	baseline[rowIndex] += (target*target - baseline[rowIndex]) / support
	if model[rowIndex] > 0 {
		manifold.taskVar.SetVec(rowIndex, model[rowIndex])
		manifold.taskScale.SetVec(rowIndex, math.Sqrt(model[rowIndex]))
		manifold.taskPrecision.SetVec(rowIndex, 1/model[rowIndex])
		manifold.taskScaleReady[rowIndex] = true
		manifold.taskSkill.SetVec(rowIndex, baseline[rowIndex]/model[rowIndex])
		manifold.taskSkillReady[rowIndex] = true
	}
}

func (manifold *resonanceManifold) predictAdjacentLayers() ([]*mat.VecDense, []*mat.VecDense) {
	for layerIndex := 0; layerIndex < len(manifold.generativeWeights); layerIndex++ {
		prediction := manifold.workspace.predictions[layerIndex]
		prediction.MulVec(manifold.generativeWeights[layerIndex], manifold.latentStates[layerIndex+1])
		if layerIndex > 0 {
			denseApplyTanhInPlace(prediction)
		}

		layerError := manifold.workspace.errors[layerIndex]
		layerError.SubVec(manifold.latentStates[layerIndex], prediction)
	}

	return manifold.workspace.predictions, manifold.workspace.errors
}

func (manifold *resonanceManifold) setAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return fmt.Errorf("resonance: alpha must be finite and in (0, 1] (domain mismatch)")
	}

	newCfg := adaptiveResonanceConfig(alpha, manifold.arch)
	manifold.cfg.LrState = newCfg.LrState
	manifold.cfg.LrGenerative = newCfg.LrGenerative
	manifold.cfg.LrTemporal = newCfg.LrTemporal
	manifold.cfg.LrRecognition = newCfg.LrRecognition

	return nil
}

/*
readoutVectorInto writes the multi-layer readout [z_1..z_L, e_0..e_{L-1}]
directly into dst without heap allocation.
*/
func (manifold *resonanceManifold) readoutVectorInto(dst []float64) int {
	_, layerErrors := manifold.predictAdjacentLayers()
	offset := 0

	if manifold.cfg.resonanceReadout == readoutAll || manifold.cfg.resonanceReadout == readoutLatents {
		for layerIndex := 1; layerIndex < len(manifold.latentStates); layerIndex++ {
			data := manifold.latentStates[layerIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	if manifold.cfg.resonanceReadout == readoutAll || manifold.cfg.resonanceReadout == readoutInnovations {
		for linkIndex := range layerErrors {
			data := layerErrors[linkIndex].RawVector().Data
			copy(dst[offset:offset+len(data)], data)
			offset += len(data)
		}
	}

	return offset
}

func (manifold *resonanceManifold) readoutVector() []float64 {
	vector := make([]float64, manifold.readoutDim)
	manifold.readoutVectorInto(vector)
	return vector
}

/*
rolloutTaskForecast returns one forecast per task row, evaluated at the current
settled readout. Element k is the row for horizon k+1: the cumulative
directional prediction over the next k+1 ticks from now. Every element is the
supervised head for its own horizon, so the curve is a genuine multi-horizon
forecast rather than a trajectory through imagined states. A request beyond
the head's rows yields the head's rows.
*/
func (manifold *resonanceManifold) rolloutTaskForecast(steps int) ([]resonanceForecast, error) {
	if manifold.taskWeights == nil || manifold.taskRows <= 0 || steps < 1 {
		return nil, nil
	}

	if steps > manifold.taskRows {
		steps = manifold.taskRows
	}

	readoutData := manifold.workspace.readoutBuf.RawVector().Data
	manifold.readoutVectorInto(readoutData)

	forecast := make([]resonanceForecast, steps)

	for horizonIndex := range steps {
		output, err := taskForecast(manifold.taskLearners[horizonIndex], readoutData)

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
func taskForecast(learner *resonanceTask, features []float64) (resonanceForecast, error) {
	if learner == nil {
		return resonanceForecast{}, fmt.Errorf("resonance: learner is nil")
	}
	reading := learner.Evaluate(features, 0, false)

	return resonanceForecast{
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
It reuses the node's Gonum workspace for each inference step.
*/
func (manifold *resonanceManifold) buildSettlePipeline() func(*resonanceManifold) *resonanceManifold {
	var settledEnergy float64
	var stableSteps int

	calcGradients := func(m *resonanceManifold) *resonanceManifold {
		predictions, layerErrors := m.predictAdjacentLayers()
		m.stateGradients(predictions, layerErrors)
		return m
	}

	lineSearch := func(m *resonanceManifold) *resonanceManifold {
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
		relativeDelta := 0.0
		if settledEnergy != 0 {
			relativeDelta = deltaEnergy / math.Abs(settledEnergy)
		}
		settledEnergy = candidateEnergy

		if m.lastInferenceSteps < m.cfg.MinInferenceSteps || relativeDelta >= m.cfg.EarlyStopTol {
			stableSteps = 0
			return m
		}
		stableSteps++
		return m
	}

	stepPipeline := func(m *resonanceManifold) *resonanceManifold {
		m.lastInferenceSteps++
		m = calcGradients(m)
		return lineSearch(m)
	}

	return func(m *resonanceManifold) *resonanceManifold {
		settledEnergy = m.energy()
		stableSteps = 0
		m.lastInferenceSteps = 0
		for iteration := 0; iteration < m.cfg.MaxInferenceSteps; iteration++ {
			if stableSteps >= m.cfg.EarlyStopPatience {
				break
			}
			m = stepPipeline(m)
		}
		return m
	}
}

/*
buildLearnPipeline constructs the pure algebraic pipeline for weight updates.
*/
func (manifold *resonanceManifold) buildLearnPipeline() func(*resonanceManifold) *resonanceManifold {
	var predictions, layerErrors []*mat.VecDense

	predict := func(m *resonanceManifold) *resonanceManifold {
		predictions, layerErrors = m.predictAdjacentLayers()
		return m
	}

	generativeUpdate := func(m *resonanceManifold) *resonanceManifold {
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
			localSignal.MulElemVec(localSignal, layerErrors[layerIndex])

			update := m.workspace.weightUpdate[layerIndex]
			denseOuterColsInto(update, localSignal, m.latentStates[layerIndex+1], 1.0)

			scale := m.cfg.LrGenerative
			sourceEnergy := denseColDot(m.latentStates[layerIndex+1], m.latentStates[layerIndex+1])
			if sourceEnergy == 0 {
				continue
			}
			scale /= sourceEnergy

			denseScaleInPlace(update, scale)
			weightMatrix.Add(weightMatrix, update)
		}
		return m
	}

	recognitionUpdate := func(m *resonanceManifold) *resonanceManifold {
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
			sourceEnergy := denseColDot(m.latentStates[layerIndex], m.latentStates[layerIndex])
			if sourceEnergy == 0 {
				continue
			}
			scale /= sourceEnergy

			denseScaleInPlace(update, scale)
			recognitionMatrix.Add(recognitionMatrix, update)
		}
		return m
	}

	temporalUpdate := func(m *resonanceManifold) *resonanceManifold {
		if m.temporalPriorsReady {
			for latentIndex, operator := range m.temporalOperators {
				layerIndex := latentIndex + 1
				temporalError := m.workspace.temporalErrors[latentIndex]
				temporalError.SubVec(m.latentStates[layerIndex], m.workspace.temporalPriors[latentIndex])

				temporalSignal := m.workspace.temporalSignals[latentIndex]
				denseApplyOneMinusSquareInto(temporalSignal, m.workspace.temporalPriors[latentIndex])
				temporalSignal.MulElemVec(temporalSignal, temporalError)
				temporalSignal.ScaleVec(m.cfg.TemporalWeights[latentIndex], temporalSignal)

				update := m.workspace.temporalUpdates[latentIndex]
				denseOuterColsInto(update, temporalSignal, m.workspace.prevLatents[latentIndex], 1.0)

				scale := m.cfg.LrTemporal
				sourceEnergy := denseColDot(m.workspace.prevLatents[latentIndex], m.workspace.prevLatents[latentIndex])
				if sourceEnergy == 0 {
					continue
				}
				scale /= sourceEnergy

				denseScaleInPlace(update, scale)
				operator.Add(operator, update)
			}
		}
		return m
	}

	advance := func(m *resonanceManifold) *resonanceManifold {
		m.advanceTemporalState()
		return m
	}

	return func(m *resonanceManifold) *resonanceManifold {
		m = predict(m)
		m = generativeUpdate(m)
		m = recognitionUpdate(m)
		m = temporalUpdate(m)

		if m.err != nil {
			return m
		}
		return advance(m)
	}
}
