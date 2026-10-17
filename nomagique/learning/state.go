package learning

import (
	"bytes"
	"encoding"
	"encoding/gob"
	"fmt"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"gonum.org/v1/gonum/mat"
)

/*
Saved state: what a PredictiveCoder has learned, so a later run continues
it instead of starting cold. Each type saves exactly the state it carries
from one arrival to the next (weights, posteriors, running estimates,
counters), never its configuration or scratch buffers: a state is restored
into a coder built by the same constructor, and every dimension and setting
it can check is checked, so a state never lands in a coder it does not fit.

A coder's pending predictions (issued, not yet fully resolved) are never
saved: resolving them later against prices after the gap would train the
head on outcomes it was never in a position to forecast. Its owner drops
them first (DropPending) and records how many.
*/

func encode(what string, state any) ([]byte, error) {
	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(state); err != nil {
		return nil, fmt.Errorf("learning: %s state: %w", what, err)
	}

	return buf.Bytes(), nil
}

func decode(what string, blob []byte, state any) error {
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(state); err != nil {
		return fmt.Errorf("%w: learning: %s state does not decode: %v", core.ErrShape, what, err)
	}

	return nil
}

func marshalPart(what string, part any) ([]byte, error) {
	marshaler, ok := part.(encoding.BinaryMarshaler)

	if !ok {
		return nil, fmt.Errorf("%w: learning: %s (%T) cannot save its state", core.ErrShape, what, part)
	}

	return marshaler.MarshalBinary()
}

func unmarshalPart(what string, part any, blob []byte) error {
	unmarshaler, ok := part.(encoding.BinaryUnmarshaler)

	if !ok {
		return fmt.Errorf("%w: learning: %s (%T) cannot restore a state", core.ErrShape, what, part)
	}

	if err := unmarshaler.UnmarshalBinary(blob); err != nil {
		return fmt.Errorf("learning: %s: %w", what, err)
	}

	return nil
}

func denseBytes(matrices ...*mat.Dense) ([][]byte, error) {
	blobs := make([][]byte, len(matrices))

	for index, matrix := range matrices {
		if matrix == nil {
			continue
		}

		blob, err := matrix.MarshalBinary()

		if err != nil {
			return nil, err
		}

		blobs[index] = blob
	}

	return blobs, nil
}

func vecBytes(vectors ...*mat.VecDense) ([][]byte, error) {
	blobs := make([][]byte, len(vectors))

	for index, vector := range vectors {
		if vector == nil {
			continue
		}

		blob, err := vector.MarshalBinary()

		if err != nil {
			return nil, err
		}

		blobs[index] = blob
	}

	return blobs, nil
}

/*
intoDense copies each saved matrix into the receiver's of the same shape; a
missing, extra or differently shaped one is an error.
*/
func intoDense(what string, matrices []*mat.Dense, blobs [][]byte) error {
	if len(blobs) != len(matrices) {
		return fmt.Errorf("%w: learning: state holds %d %s, the coder %d", core.ErrShape, len(blobs), what, len(matrices))
	}

	for index, matrix := range matrices {
		if matrix == nil || len(blobs[index]) == 0 {
			if matrix != nil || len(blobs[index]) != 0 {
				return fmt.Errorf("%w: learning: %s %d is held by only one of state and coder", core.ErrShape, what, index)
			}

			continue
		}

		var saved mat.Dense

		if err := saved.UnmarshalBinary(blobs[index]); err != nil {
			return fmt.Errorf("%w: learning: %s %d does not decode: %v", core.ErrShape, what, index, err)
		}

		rows, cols := saved.Dims()
		wantRows, wantCols := matrix.Dims()

		if rows != wantRows || cols != wantCols {
			return fmt.Errorf("%w: learning: %s %d is %dx%d in the state, %dx%d in the coder", core.ErrShape, what, index, rows, cols, wantRows, wantCols)
		}

		matrix.Copy(&saved)
	}

	return nil
}

func intoVec(what string, vectors []*mat.VecDense, blobs [][]byte) error {
	if len(blobs) != len(vectors) {
		return fmt.Errorf("%w: learning: state holds %d %s, the coder %d", core.ErrShape, len(blobs), what, len(vectors))
	}

	for index, vector := range vectors {
		if vector == nil || len(blobs[index]) == 0 {
			if vector != nil || len(blobs[index]) != 0 {
				return fmt.Errorf("%w: learning: %s %d is held by only one of state and coder", core.ErrShape, what, index)
			}

			continue
		}

		var saved mat.VecDense

		if err := saved.UnmarshalBinary(blobs[index]); err != nil {
			return fmt.Errorf("%w: learning: %s %d does not decode: %v", core.ErrShape, what, index, err)
		}

		if saved.Len() != vector.Len() {
			return fmt.Errorf("%w: learning: %s %d has %d values in the state, %d in the coder", core.ErrShape, what, index, saved.Len(), vector.Len())
		}

		vector.CopyVec(&saved)
	}

	return nil
}

/*
manifoldState is what a ResonanceManifold carries across arrivals. Arch and
the task sizes are checked, not restored; Cfg is restored because the pace
retunes its rates (setAlpha).
*/
type manifoldState struct {
	Arch       []int
	TargetDim  int
	TaskRows   int
	ReadoutDim int
	Cfg        resonanceConfig

	Generative, Recognition, Temporal [][]byte
	TaskWeights, TaskBias             [][]byte
	TaskLearners                      [][]byte

	LatentStates, ErrorVar, Precision, TemporalVar, TemporalPrecision [][]byte
	TemporalPriorsReady, SettleAdvancedTemporal                       bool

	TaskVectors    [][]byte // taskVar, taskScale, taskPrecision, taskModelLoss, taskBaselineLoss, taskSkill
	TaskScaleReady []bool
	TaskSkillReady []bool

	LastInferenceSteps int
	Output             float64

	// The temporal prior a step predicts the next from (workspace).
	PrevLatents, TemporalPriors [][]byte
}

func (rm *ResonanceManifold) taskVectors() []*mat.VecDense {
	return []*mat.VecDense{rm.taskVar, rm.taskScale, rm.taskPrecision, rm.taskModelLoss, rm.taskBaselineLoss, rm.taskSkill}
}

func (rm *ResonanceManifold) MarshalBinary() ([]byte, error) {
	if err := rm.Error(); err != nil {
		return nil, fmt.Errorf("learning: resonance manifold failed, its state is not saved: %w", err)
	}

	state := manifoldState{
		Arch: rm.arch, TargetDim: rm.targetDim, TaskRows: rm.taskRows, ReadoutDim: rm.readoutDim, Cfg: rm.cfg,
		TemporalPriorsReady: rm.temporalPriorsReady, SettleAdvancedTemporal: rm.settleAdvancedTemporal,
		TaskScaleReady: rm.taskScaleReady, TaskSkillReady: rm.taskSkillReady,
		LastInferenceSteps: rm.lastInferenceSteps, Output: rm.output,
		TaskLearners: make([][]byte, len(rm.taskLearners)),
	}

	var err error

	for _, part := range []struct {
		into *[][]byte
		from func() ([][]byte, error)
	}{
		{&state.Generative, func() ([][]byte, error) { return denseBytes(rm.generativeWeights...) }},
		{&state.Recognition, func() ([][]byte, error) { return denseBytes(rm.recognitionWeights...) }},
		{&state.Temporal, func() ([][]byte, error) { return denseBytes(rm.temporalOperators...) }},
		{&state.TaskWeights, func() ([][]byte, error) { return denseBytes(rm.taskWeights) }},
		{&state.TaskBias, func() ([][]byte, error) { return vecBytes(rm.taskBias) }},
		{&state.LatentStates, func() ([][]byte, error) { return vecBytes(rm.latentStates...) }},
		{&state.ErrorVar, func() ([][]byte, error) { return vecBytes(rm.errorVar...) }},
		{&state.Precision, func() ([][]byte, error) { return vecBytes(rm.precision...) }},
		{&state.TemporalVar, func() ([][]byte, error) { return vecBytes(rm.temporalVar...) }},
		{&state.TemporalPrecision, func() ([][]byte, error) { return vecBytes(rm.temporalPrecision...) }},
		{&state.TaskVectors, func() ([][]byte, error) { return vecBytes(rm.taskVectors()...) }},
		{&state.PrevLatents, func() ([][]byte, error) { return vecBytes(rm.workspace.prevLatents...) }},
		{&state.TemporalPriors, func() ([][]byte, error) { return vecBytes(rm.workspace.temporalPriors...) }},
	} {
		if *part.into, err = part.from(); err != nil {
			return nil, fmt.Errorf("learning: resonance manifold state: %w", err)
		}
	}

	for index, learner := range rm.taskLearners {
		if state.TaskLearners[index], err = marshalPart(fmt.Sprintf("task learner %d", index), learner); err != nil {
			return nil, err
		}
	}

	return encode("resonance manifold", state)
}

func (rm *ResonanceManifold) UnmarshalBinary(blob []byte) error {
	var state manifoldState

	if err := decode("resonance manifold", blob, &state); err != nil {
		return err
	}

	if !slices.Equal(state.Arch, rm.arch) || state.TargetDim != rm.targetDim || state.TaskRows != rm.taskRows ||
		state.ReadoutDim != rm.readoutDim || state.Cfg.ReadoutMode != rm.cfg.ReadoutMode || len(state.TaskLearners) != len(rm.taskLearners) {
		return fmt.Errorf(
			"%w: learning: manifold state is arch %v target %d rows %d readout %d/%d with %d task learners; the coder arch %v target %d rows %d readout %d/%d with %d",
			core.ErrShape, state.Arch, state.TargetDim, state.TaskRows, state.ReadoutDim, state.Cfg.ReadoutMode, len(state.TaskLearners),
			rm.arch, rm.targetDim, rm.taskRows, rm.readoutDim, rm.cfg.ReadoutMode, len(rm.taskLearners),
		)
	}

	if len(state.TaskScaleReady) != len(rm.taskScaleReady) || len(state.TaskSkillReady) != len(rm.taskSkillReady) {
		return fmt.Errorf("%w: learning: manifold state task readiness does not fit the coder", core.ErrShape)
	}

	for _, part := range []error{
		intoDense("generative weights", rm.generativeWeights, state.Generative),
		intoDense("recognition weights", rm.recognitionWeights, state.Recognition),
		intoDense("temporal operators", rm.temporalOperators, state.Temporal),
		intoDense("task weights", []*mat.Dense{rm.taskWeights}, state.TaskWeights),
		intoVec("task bias", []*mat.VecDense{rm.taskBias}, state.TaskBias),
		intoVec("latent states", rm.latentStates, state.LatentStates),
		intoVec("error variances", rm.errorVar, state.ErrorVar),
		intoVec("precisions", rm.precision, state.Precision),
		intoVec("temporal variances", rm.temporalVar, state.TemporalVar),
		intoVec("temporal precisions", rm.temporalPrecision, state.TemporalPrecision),
		intoVec("task vectors", rm.taskVectors(), state.TaskVectors),
		intoVec("previous latents", rm.workspace.prevLatents, state.PrevLatents),
		intoVec("temporal priors", rm.workspace.temporalPriors, state.TemporalPriors),
	} {
		if part != nil {
			return part
		}
	}

	for index, learner := range rm.taskLearners {
		if err := unmarshalPart(fmt.Sprintf("task learner %d", index), learner, state.TaskLearners[index]); err != nil {
			return err
		}
	}

	rm.cfg = state.Cfg
	rm.temporalPriorsReady, rm.settleAdvancedTemporal = state.TemporalPriorsReady, state.SettleAdvancedTemporal
	copy(rm.taskScaleReady, state.TaskScaleReady)
	copy(rm.taskSkillReady, state.TaskSkillReady)
	rm.lastInferenceSteps, rm.output = state.LastInferenceSteps, state.Output
	return nil
}

/*
rlsState is an RLS head row: its learner's posterior, with the dimension
and forgetting it was built with (checked).
*/
type rlsState struct {
	Dimension int
	Lambda    float64
	Learner   []byte
}

func (op *RLS) MarshalBinary() ([]byte, error) {
	if err := op.Error(); err != nil {
		return nil, fmt.Errorf("learning: RLS failed, its state is not saved: %w", err)
	}

	learner, err := marshalPart("RLS learner", op.learner)

	if err != nil {
		return nil, err
	}

	return encode("RLS", rlsState{Dimension: op.dimension, Lambda: op.lambda, Learner: learner})
}

func (op *RLS) UnmarshalBinary(blob []byte) error {
	var state rlsState

	if err := decode("RLS", blob, &state); err != nil {
		return err
	}

	if state.Dimension != op.dimension || state.Lambda != op.lambda {
		return fmt.Errorf(
			"%w: learning: RLS state is dimension %d lambda %g, this RLS %d %g",
			core.ErrShape, state.Dimension, state.Lambda, op.dimension, op.lambda,
		)
	}

	return unmarshalPart("RLS learner", op.learner, state.Learner)
}

/*
paceState is the pace controller's position and its calibrator's history;
its configuration is checked.
*/
type paceState struct {
	Config     [6]float64 // rest, lower, upper, gain, band, window
	LogAlpha   float64
	Alpha      float64
	Seeded     bool
	Calibrator []byte
}

func (op *Pace) config() [6]float64 {
	return [6]float64{op.rest, op.lower, op.upper, op.gain, op.band, op.window}
}

func (op *Pace) MarshalBinary() ([]byte, error) {
	if err := op.Error(); err != nil {
		return nil, fmt.Errorf("learning: pace failed, its state is not saved: %w", err)
	}

	calibrator, err := marshalPart("pace calibrator", op.calibrator)

	if err != nil {
		return nil, err
	}

	return encode("pace", paceState{
		Config: op.config(), LogAlpha: op.logAlpha, Alpha: op.alpha, Seeded: op.seeded, Calibrator: calibrator,
	})
}

func (op *Pace) UnmarshalBinary(blob []byte) error {
	var state paceState

	if err := decode("pace", blob, &state); err != nil {
		return err
	}

	if state.Config != op.config() {
		return fmt.Errorf(
			"%w: learning: pace state was configured %v (rest, lower, upper, gain, band, window), this pace %v",
			core.ErrShape, state.Config, op.config(),
		)
	}

	if err := unmarshalPart("pace calibrator", op.calibrator, state.Calibrator); err != nil {
		return err
	}

	op.logAlpha, op.alpha, op.seeded = state.LogAlpha, state.Alpha, state.Seeded
	return nil
}

/*
ledgerState is a ledger with no prediction pending: its sequence, its
counts and the most recent resolution it reports.
*/
type ledgerState struct {
	MaxHorizon int
	Seq        int64
	Resolved   int
	Total      int
	Out        [9]float64
}

/*
DropPending discards every prediction the ledger has issued and not fully
resolved, with the references kept to resolve them, and reports how many
it dropped. The next reference resolves nothing issued before it.
*/
func (op *TemporalLedger) DropPending() int {
	dropped := len(op.issued)
	clear(op.issued)
	clear(op.features)
	clear(op.predictions)
	clear(op.references)
	op.oldest = op.seq + 1
	op.out[2] = 0
	return dropped
}

func (op *TemporalLedger) MarshalBinary() ([]byte, error) {
	if err := op.Error(); err != nil {
		return nil, fmt.Errorf("learning: ledger failed, its state is not saved: %w", err)
	}

	if pending := len(op.issued) + len(op.references); pending > 0 {
		return nil, fmt.Errorf(
			"%w: learning: ledger holds %d pending predictions and %d references: drop them (DropPending) before saving",
			core.ErrShape, len(op.issued), len(op.references),
		)
	}

	return encode("ledger", ledgerState{MaxHorizon: op.maxHorizon, Seq: op.seq, Resolved: op.resolved, Total: op.total, Out: op.out})
}

func (op *TemporalLedger) UnmarshalBinary(blob []byte) error {
	var state ledgerState

	if err := decode("ledger", blob, &state); err != nil {
		return err
	}

	if state.MaxHorizon != op.maxHorizon {
		return fmt.Errorf("%w: learning: ledger state has horizon %d, this ledger %d", core.ErrShape, state.MaxHorizon, op.maxHorizon)
	}

	if len(op.issued)+len(op.references) > 0 || op.seq != 0 {
		return fmt.Errorf("%w: learning: a state is restored only into a fresh ledger", core.ErrShape)
	}

	op.seq, op.oldest, op.resolved, op.total, op.out = state.Seq, state.Seq+1, state.Resolved, state.Total, state.Out
	return nil
}

/*
coderState is a PredictiveCoder with no prediction pending: its rate, its
resolution counts, the ledger reading it reports, and its parts.
*/
type coderState struct {
	Horizon  int
	Learn    bool
	Alpha    float64
	Resolved float64
	LedgerAt [9]float64
	Manifold []byte
	Pace     []byte
	Ledger   []byte
}

/*
DropPending discards the coder's pending predictions (TemporalLedger.
DropPending) and reports how many it dropped.
*/
func (coder *PredictiveCoder) DropPending() (int, error) {
	ledger, ok := coder.ledger.(*TemporalLedger)

	if !ok {
		return 0, fmt.Errorf("%w: learning: predictive coder has no temporal ledger (the architecture was rejected)", core.ErrShape)
	}

	dropped := ledger.DropPending()
	coder.pending, coder.ledgerAt[2] = 0, 0
	return dropped, nil
}

/*
MarshalBinary is the coder's learned state (encoding.BinaryMarshaler). A
coder with predictions pending is refused: DropPending them first.
*/
func (coder *PredictiveCoder) MarshalBinary() ([]byte, error) {
	if err := coder.Error(); err != nil {
		return nil, fmt.Errorf("learning: predictive coder failed, its state is not saved: %w", err)
	}

	if coder.manifold == nil || coder.ledger == nil {
		return nil, fmt.Errorf("%w: learning: predictive coder has no manifold: the architecture was rejected", core.ErrShape)
	}

	state := coderState{
		Horizon: coder.horizon, Learn: coder.learn, Alpha: coder.alpha, Resolved: coder.resolved, LedgerAt: coder.ledgerAt,
	}

	var err error

	if state.Ledger, err = marshalPart("ledger", coder.ledger); err != nil {
		return nil, err
	}

	if state.Manifold, err = marshalPart("manifold", coder.manifold); err != nil {
		return nil, err
	}

	if state.Pace, err = marshalPart("pace", coder.pace); err != nil {
		return nil, err
	}

	return encode("predictive coder", state)
}

/*
UnmarshalBinary restores a MarshalBinary state into a coder built by the
same constructor with the same arguments.
*/
func (coder *PredictiveCoder) UnmarshalBinary(blob []byte) error {
	var state coderState

	if err := decode("predictive coder", blob, &state); err != nil {
		return err
	}

	if coder.manifold == nil || coder.ledger == nil {
		return fmt.Errorf("%w: learning: predictive coder has no manifold: the architecture was rejected", core.ErrShape)
	}

	if state.Horizon != coder.horizon || state.Learn != coder.learn {
		return fmt.Errorf(
			"%w: learning: coder state has horizon %d learn %t, this coder %d %t",
			core.ErrShape, state.Horizon, state.Learn, coder.horizon, coder.learn,
		)
	}

	for _, part := range []struct {
		what string
		into any
		blob []byte
	}{
		{"manifold", coder.manifold, state.Manifold},
		{"pace", coder.pace, state.Pace},
		{"ledger", coder.ledger, state.Ledger},
	} {
		if err := unmarshalPart(part.what, part.into, part.blob); err != nil {
			return err
		}
	}

	coder.alpha, coder.resolved, coder.ledgerAt, coder.pending = state.Alpha, state.Resolved, state.LedgerAt, 0
	return nil
}
