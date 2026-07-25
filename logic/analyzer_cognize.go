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
	"github.com/theapemachine/symm/logic/category"
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
	cutID types.CutID,
	tick int64,
) ([]time.Time, bool) {
	remObservations := make([]time.Time, 0, len(states))
	remRequested := false
	cognizeStarted := time.Now()
	categoryTokens := analyzer.cognitionTokens(thesis, states)

	for _, state := range states {
		if state.Replay {
			analyzer.recall(thesis, state, categoryTokens[state.Symbol])
			continue
		}

		if !analyzer.cognize(thesis, state, categoryTokens[state.Symbol]) {
			continue
		}

		remObservations = append(remObservations, state.At)
		value, found := thesis.Cognition.Load(state.Symbol)

		if found {
			reading := value.(types.Cognition)
			remRequested = remRequested || reading.Ambiguous

			if analyzer.manifold != nil {
				errnie.Error(analyzer.manifold.CommitPhase(reading))
			}
		}
	}

	remPending := 0

	if analyzer.rem != nil {
		remPending = analyzer.rem.Pending()
	}

	payload := map[string]any{
		"states":      len(states),
		"ns":          time.Since(cognizeStarted).Nanoseconds(),
		"ambiguous":   remRequested,
		"rem_pending": remPending,
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "cognize", payload))

	return remObservations, remRequested
}

/*
cognitionTokens builds one category token bag per symbol for this cut so
cognize/recall do not rescan the full category slice for every manifold state.
*/
func (analyzer *Analyzer) cognitionTokens(
	thesis *types.Thesis,
	states []manifold.State,
) map[string][]string {
	if thesis == nil || len(states) == 0 {
		return nil
	}

	reporter := category.Report(analyzer.categories)

	if reporter == nil {
		return nil
	}

	needed := make(map[string]struct{}, len(states))

	for _, state := range states {
		if state.Symbol == "" {
			continue
		}

		needed[state.Symbol] = struct{}{}
	}

	grouped := make(map[string][]types.Category, len(needed))

	for _, row := range thesis.Categories {
		if _, ok := needed[row.Symbol]; !ok {
			continue
		}

		grouped[row.Symbol] = append(grouped[row.Symbol], row)
	}

	tokens := make(map[string][]string, len(needed))

	for symbol := range needed {
		tokens[symbol] = reporter.Tokens(symbol, grouped[symbol])
	}

	return tokens
}

/*
recall republishes the focused symbol's cognitive visualization from the current
trained tree when its physical state is an unchanged replay. It does not train,
commit an episode, or advance a causal model, so presentation cannot fabricate a
market observation.
*/
func (analyzer *Analyzer) recall(
	thesis *types.Thesis,
	state manifold.State,
	categoryTokens []string,
) {
	if analyzer.tree == nil {
		return
	}

	reading, found := analyzer.cognition[state.Symbol]
	focus := types.Focus()

	if found && reading.At.Equal(state.At) {
		if focus == "" || reading.Symbol != focus || len(reading.Branches) > 0 {
			thesis.Cognition.Store(state.Symbol, reading)
			return
		}
	}

	parts, sequence := analyzer.sensorySequence(state, categoryTokens)

	if len(sequence) == 0 {
		return
	}

	parent := sequence

	if boundary := bytes.LastIndexByte(sequence, '_'); boundary > 0 {
		parent = sequence[:boundary]
	}

	reading = analyzer.readCognition(state, parts, sequence, parent)
	thesis.Cognition.Store(state.Symbol, reading)

	if analyzer.cognition == nil {
		analyzer.cognition = make(map[string]types.Cognition)
	}

	analyzer.cognition[state.Symbol] = reading
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
DMT sensory sequence, anchors the intensity-derived attractor on that sequence,
then publishes the posterior classification strategy consumes. Publishing only
the pre-train prior left Ready empty whenever the measurement bag changed,
because exact sequence repeats almost never occur on a live tape.
*/
func (analyzer *Analyzer) cognize(
	thesis *types.Thesis,
	state manifold.State,
	categoryTokens []string,
) bool {
	if analyzer.tree == nil || !state.GasReady() {
		return false
	}

	parts, sequence := analyzer.sensorySequence(state, categoryTokens)

	if len(sequence) == 0 {
		return false
	}

	parent := sequence

	if boundary := bytes.LastIndexByte(sequence, '_'); boundary > 0 {
		parent = sequence[:boundary]
	}

	if analyzer.cognition == nil {
		analyzer.cognition = make(map[string]types.Cognition)
	}

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

	if !analyzer.trainAttractors(state, sequence) {
		return false
	}

	reading := analyzer.readCognition(state, parts, sequence, parent)
	thesis.Cognition.Store(state.Symbol, reading)
	analyzer.cognition[state.Symbol] = reading

	return true
}

/*
sensorySequence builds the deterministic DMT token stream for one state.
The symbol token stays first so the shared radix tree namespaces each coin;
category and transition tokens (bounded by active taxonomy) plus a few field
scalars form the bag — raw measurement keys are not expanded into the tree.
*/
func (analyzer *Analyzer) sensorySequence(
	state manifold.State,
	categoryTokens []string,
) ([]string, []byte) {
	replacer := strings.NewReplacer("_", "-", "/", "-")
	symbolToken := "symbol-" + replacer.Replace(state.Symbol)
	evidence := make([]string, 0, 16)

	for _, token := range categoryTokens {
		evidence = append(evidence, replacer.Replace(token))
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
readCognition classifies the sensory sequence for strategy on the hot path —
winner, confidence, ambiguity, contrast, cohort, predictions, and classes —
plus the lookahead strength the terminal shows as category strength, then
attaches the full Cortex radix/beam visualization only for the focused symbol.
*/
func (analyzer *Analyzer) readCognition(
	state manifold.State,
	parts []string,
	sequence []byte,
	parent []byte,
) types.Cognition {
	var classificationScratch dmt.ClassificationScratch
	classification, err := analyzer.tree.Classify(sequence, &classificationScratch)

	if err != nil {
		return types.Cognition{
			Source:   "dmt",
			Symbol:   state.Symbol,
			At:       state.At,
			Sequence: string(sequence),
			Ready:    false,
			Error:    err.Error(),
		}
	}

	storageParent := append([]byte("s/"), parent...)
	ambiguity := analyzer.tree.MeasureBranchAmbiguity(storageParent)
	predictionBuffer := make([]dmt.LookaheadPrediction, 0, len(parts))
	predictions := analyzer.tree.PredictNextSensoryTokens(parent, predictionBuffer)
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
		Classes:          cognitionClasses(classification),
		LookaheadScore:   analyzer.lookaheadScore(predictions),
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

	analyzer.attachVisualization(&reading, sequence, parent, parts, classification, predictions)

	return reading
}

/*
attachVisualization materializes the Cortex radix and beam tree only for the UI
focus symbol. Expanding the full branch fan-out and running beam search for every
symbol every tick is the cold path; strategy already has its classification,
ambiguity, contrast, and lookahead fields from readCognition's hot path.
*/
func (analyzer *Analyzer) attachVisualization(
	reading *types.Cognition,
	sequence []byte,
	parent []byte,
	parts []string,
	classification dmt.ClassificationResult,
	predictions []dmt.LookaheadPrediction,
) {
	if reading == nil {
		return
	}

	focus := types.Focus()

	if focus == "" {
		return
	}

	if reading.Symbol != focus {
		return
	}

	branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
		analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)

	reading.Branches = branches
	reading.Beams = beams
	reading.Classes = classes
	reading.BeamWidth = beamWidth
	reading.MaxHops = maxHops
	reading.NodeCount = nodeCount
	reading.LookaheadScore = lookaheadScore
	reading.LookaheadPaths = lookaheadPaths
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
