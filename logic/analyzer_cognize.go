package logic

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
cognizeStates runs DMT cognition for each gas-ready state and collects REM
observation times plus ambiguity requests.
*/
func (analyzer *Analyzer) cognizeStates(
	thesis *types.Thesis,
	states []manifold.State,
) ([]time.Time, bool) {
	remObservations := make([]time.Time, 0, len(states))
	remRequested := false
	cognizeStarted := time.Now()

	for _, state := range states {
		if state.Replay {
			continue
		}

		if !analyzer.cognize(thesis, state) {
			continue
		}

		remObservations = append(remObservations, state.At)
		value, found := thesis.Cognition.Load(state.Symbol)

		if found {
			remRequested = remRequested || value.(types.Cognition).Ambiguous
		}
	}

	remPending := 0

	if analyzer.rem != nil {
		remPending = analyzer.rem.Pending()
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "cognize", map[string]any{
		"states":      len(states),
		"ns":          time.Since(cognizeStarted).Nanoseconds(),
		"ambiguous":   remRequested,
		"rem_pending": remPending,
	}))

	return remObservations, remRequested
}

/*
consolidate accumulates episodic observations and requests off-path REM when
DMT reports ambiguous branching. The ambiguity gate supplies the trigger, so
REM has no unrelated timer or fixed batch threshold.
*/
func (analyzer *Analyzer) consolidate(
	thesis *types.Thesis,
	observations []time.Time,
	requested bool,
) {
	if analyzer.rem == nil {
		analyzer.rem = newREMSleep(analyzer.ctx, analyzer.tree)
		analyzer.rem.SetRecorder(analyzer.recorder)
	}

	analyzer.rem.Accumulate(observations)

	if requested {
		analyzer.rem.Request(thesis.Tick)
	}

	analyzer.rem.Stamp(thesis)
}

/*
cognize turns the Thesis evidence for one manifold state into a deterministic
DMT sensory sequence, reads learned cognition, and writes the decoupled result
back onto the Thesis before learning the current observation.
*/
func (analyzer *Analyzer) cognize(
	thesis *types.Thesis,
	state manifold.State,
) bool {
	if analyzer.tree == nil || !state.GasReady() {
		return false
	}

	parts, sequence := analyzer.sensorySequence(thesis, state)

	if len(sequence) == 0 {
		return false
	}

	parent := sequence

	if boundary := bytes.LastIndexByte(sequence, '_'); boundary > 0 {
		parent = sequence[:boundary]
	}

	reading := analyzer.readCognition(state, parts, sequence, parent)
	thesis.Cognition.Store(state.Symbol, reading)

	if analyzer.cognition == nil {
		analyzer.cognition = make(map[string]types.Cognition)
	}

	analyzer.cognition[state.Symbol] = reading
	analyzer.tree.TrainSensorySequence(sequence)

	if _, _, err := analyzer.tree.CommitToEpisodicBuffer(
		uint64(state.At.UnixNano()), sequence,
	); err != nil {
		// The dmt persistence wrapper renders only "dmt/tree" and hides the
		// underlying WAL cause; unwrap so the real failure is visible.
		cause := err
		for unwrapped := errors.Unwrap(cause); unwrapped != nil; unwrapped = errors.Unwrap(cause) {
			cause = unwrapped
		}

		errnie.Error(errnie.Err(errnie.IO, "logic cognize: episodic commit failed", err).
			With("cause", cause.Error()).
			With("symbol", state.Symbol))

		return false
	}

	return analyzer.trainAttractors(state, sequence)
}

/*
sensorySequence builds the deterministic DMT token stream for one state.
The symbol token stays first so the shared radix tree namespaces each coin;
remaining evidence tokens are sorted for a stable bag under that hop.
*/
func (analyzer *Analyzer) sensorySequence(
	thesis *types.Thesis,
	state manifold.State,
) ([]string, []byte) {
	evidence := make([]string, 0, len(thesis.Measurements)+4)
	seen := make(map[string]struct{}, len(thesis.Measurements)+4)
	replacer := strings.NewReplacer("_", "-", "/", "-")
	symbolToken := "symbol-" + replacer.Replace(state.Symbol)

	for _, measurement := range thesis.Measurements {
		if measurement == nil || measurement.Symbol != state.Symbol ||
			measurement.Normalized == nil ||
			measurement.Validity.State != types.ValidityValid {
			continue
		}

		direction := signedToken(*measurement.Normalized)
		token := replacer.Replace(strings.Join([]string{
			string(measurement.Source), string(measurement.Metric),
			string(measurement.Side), direction,
		}, "-"))

		if _, exists := seen[token]; exists {
			continue
		}

		seen[token] = struct{}{}
		evidence = append(evidence, token)
	}

	for name, value := range map[string]float64{
		"pressure":   state.Reading.PressureGradX,
		"divergence": state.Reading.Divergence,
		"stress":     state.StressAnisotropy,
	} {
		evidence = append(evidence, name+"-"+signedToken(value))
	}

	sort.Strings(evidence)
	parts := append([]string{symbolToken}, evidence...)

	return parts, []byte(strings.Join(parts, "_"))
}

/*
signedToken maps a signed scalar onto the DMT vocabulary polarity tokens.
*/
func signedToken(value float64) string {
	if value > 0 {
		return "positive"
	}

	if value == 0 {
		return "zero"
	}

	return "negative"
}

/*
readCognition classifies the sensory sequence for strategy and attaches the
Cortex radix/beam visualization so the cognitive tree is never an empty shell.
*/
func (analyzer *Analyzer) readCognition(
	state manifold.State,
	parts []string,
	sequence []byte,
	parent []byte,
) types.Cognition {
	var classificationScratch dmt.ClassificationScratch
	classification := analyzer.tree.Classify(sequence, &classificationScratch)
	storageParent := append([]byte("s/"), parent...)
	ambiguity := analyzer.tree.MeasureBranchAmbiguity(storageParent)
	predictionBuffer := make([]dmt.LookaheadPrediction, 0, len(parts))
	predictions := analyzer.tree.PredictNextSensoryTokens(parent, predictionBuffer)
	branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
		analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)
	reading := types.Cognition{
		Source:           "dmt",
		Symbol:           state.Symbol,
		At:               state.At,
		Sequence:         string(sequence),
		Ready:            len(classification.Winner) > 0,
		Winner:           string(classification.Winner),
		Confidence:       classification.Highest,
		EntropyBits:      ambiguity.EntropyBits,
		EntropyThreshold: ambiguity.Threshold,
		Ambiguous:        ambiguity.Ambiguous,
		Cohort:           analyzer.tree.GetSensoryWeight(sequence).Count,
		Predictions:      make(map[string]float64, len(predictions)),
		Branches:         branches,
		Beams:            beams,
		Classes:          classes,
		BeamWidth:        beamWidth,
		MaxHops:          maxHops,
		NodeCount:        nodeCount,
		LookaheadScore:   lookaheadScore,
		LookaheadPaths:   lookaheadPaths,
		RegimePrefix:     string(parent),
	}

	if len(classification.Scores) > 1 {
		reading.Contrast = analyzer.tree.ComputeBasinContrastiveEvidence(
			classification.Scores[0].ClassName,
			classification.Scores[1].ClassName,
			sequence,
		).Divergence
	}

	for _, prediction := range predictions {
		reading.Predictions[string(prediction.Token)] = prediction.Probability
	}

	return reading
}

/*
trainAttractors updates buy/sell/balanced basin weights for the sensory sequence.
*/
func (analyzer *Analyzer) trainAttractors(
	state manifold.State,
	sequence []byte,
) bool {
	class := []byte("balanced")

	if state.BuyIntensity > state.SellIntensity {
		class = []byte("buy")
	}

	if state.SellIntensity > state.BuyIntensity {
		class = []byte("sell")
	}

	attractors := [][]byte{[]byte("buy"), []byte("sell"), []byte("balanced")}
	weights := make([]dmt.CognitiveState, len(attractors))
	total := uint64(1)

	for index, candidate := range attractors {
		weights[index] = analyzer.tree.GetAttractorBasin(candidate, sequence)
		total += weights[index].Count

		if bytes.Equal(candidate, class) {
			weights[index].Count++
		}
	}

	for index, candidate := range attractors {
		if weights[index].Count == 0 {
			continue
		}

		weights[index].Probability = float64(weights[index].Count) / float64(total)

		if _, _, err := analyzer.tree.InsertAttractorBasin(
			candidate, sequence, weights[index],
		); err != nil {
			errnie.Error(err)
		}
	}

	return true
}
