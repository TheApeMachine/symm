package logic

import (
	"bytes"
	"strings"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

var cognitionTokenReplacer = strings.NewReplacer("_", "-", "/", "-")

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
		if stateReplay(state) {
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

	tokens := make(map[string][]string, len(needed))

	for symbol := range needed {
		tokens[symbol] = reporter.Tokens(symbol, thesis.Categories[symbol])
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

	symbolToken, sequence, partCount := analyzer.sensorySequence(state, categoryTokens)

	if len(sequence) == 0 {
		return
	}

	parent := sequence

	if boundary := bytes.LastIndexByte(sequence, '_'); boundary > 0 {
		parent = sequence[:boundary]
	}

	reading = analyzer.readCognition(state, symbolToken, partCount, sequence, parent)
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

	symbolToken, sequence, partCount := analyzer.sensorySequence(state, categoryTokens)

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

	if _, ok := analyzer.tree.CommitToEpisodicBuffer(
		uint64(state.At.UnixNano()), sequence,
	); !ok {
		errnie.Error(errnie.Err(
			errnie.IO, "logic cognize: episodic commit failed", nil,
		))

		return false
	}

	if !analyzer.trainAttractors(thesis, state, sequence) {
		return false
	}

	reading := analyzer.readCognition(state, symbolToken, partCount, sequence, parent)
	thesis.Cognition.Store(state.Symbol, reading)
	analyzer.cognition[state.Symbol] = reading

	return true
}

/*
sensorySequence builds the deterministic DMT token stream for one state.
The symbol token starts with the s/ sensory namespace so the shared radix tree
namespaces each coin; temporal category transition tokens (prior -> top) form
the sequence path.
*/
func (analyzer *Analyzer) sensorySequence(
	state manifold.State,
	categoryTokens []string,
) (string, []byte, int) {
	symbolToken := "s/" + cognitionTokenReplacer.Replace(state.Symbol)
	sequenceSize := len(symbolToken)

	for _, token := range categoryTokens {
		replaced := cognitionTokenReplacer.Replace(token)
		sequenceSize += len(replaced) + 1
	}

	if len(categoryTokens) == 0 {
		return symbolToken, []byte(symbolToken), 1
	}

	builder := strings.Builder{}
	builder.Grow(sequenceSize)
	builder.WriteString(symbolToken)

	for _, token := range categoryTokens {
		builder.WriteByte('_')
		builder.WriteString(cognitionTokenReplacer.Replace(token))
	}

	return symbolToken, []byte(builder.String()), len(categoryTokens) + 1
}

/*
readCognition classifies the sensory sequence for strategy on the hot path —
winner, confidence, ambiguity, contrast, cohort, predictions, and classes —
plus the lookahead strength the terminal shows as category strength, then
attaches the full Cortex radix/beam visualization only for the focused symbol.
*/
func (analyzer *Analyzer) readCognition(
	state manifold.State,
	symbolToken string,
	partCount int,
	sequence []byte,
	parent []byte,
) types.Cognition {
	var classificationScratch dmt.ClassificationScratch
	classification := analyzer.tree.Classify(sequence, &classificationScratch)

	storageParent := append([]byte("s/"), parent...)
	ambiguity := analyzer.tree.MeasureBranchAmbiguity(storageParent)
	predictionBuffer := make([]dmt.LookaheadPrediction, 0, partCount)
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

	analyzer.attachVisualization(&reading, sequence, parent, symbolToken, classification, predictions)

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
	symbolToken string,
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
			sequence, parent, symbolToken, classification, predictions,
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
trainAttractors updates physical market regime basin weights for the sensory sequence.
Attractors represent dynamical field categories rather than trading decisions.
*/
func (analyzer *Analyzer) trainAttractors(
	thesis *types.Thesis,
	state manifold.State,
	sequence []byte,
) bool {
	regime := string(types.Equilibrium)

	if state.BuyIntensity > state.SellIntensity {
		regime = string(types.Laminar)
	}

	if state.SellIntensity > state.BuyIntensity {
		regime = string(types.Turbulent)
	}

	if thesis != nil {
		bestCategory := types.Category{Strength: 0}

		for _, value := range thesis.Categories[state.Symbol] {
			if value.Type == types.CausalNoise || value.Type == types.StochasticNoise {
				continue
			}

			if value.Strength > bestCategory.Strength {
				bestCategory = value
			}
		}

		if bestCategory.Strength > 0 {
			regime = string(bestCategory.Type)
		}
	}

	regimeKey := []byte(regime)
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1
			continue
		}

		currentPath := sequence[:index]
		parentPath := []byte(nil)

		if boundary := bytes.LastIndexByte(currentPath, '_'); boundary > 0 {
			parentPath = currentPath[:boundary]
		}

		basin := analyzer.tree.GetAttractorBasin(regimeKey, currentPath)
		basin.Count++

		parentCount := uint64(0)

		if len(parentPath) > 0 {
			parentCount = analyzer.tree.GetAttractorBasin(regimeKey, parentPath).Count
		}

		if basin.Count == 1 && parentCount == 0 {
			basin.Probability = 1
		} else {
			basin.Probability = float64(basin.Count) / float64(basin.Count+parentCount)
		}

		if _, ok := analyzer.tree.InsertAttractorBasin(regimeKey, currentPath, basin); !ok {
			errnie.Error(errnie.Err(
				errnie.IO,
				"failed ot write attractor basin",
				nil,
			))
			return false
		}

		tokenStart = index + 1
	}

	return true
}
