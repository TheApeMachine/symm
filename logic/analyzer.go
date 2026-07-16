package logic

import (
	"bytes"
	"context"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the composed analysis responsibilities after every signal
has measured the current Thesis. The manifold solver owns the Hawkes-driven GPU
field step, while the Analyzer builds each symbol's evidence topology with Gonum.
*/
type Analyzer struct {
	gate      stageGate
	status    types.Status
	manifold  *manifold.Solver
	hawkes    manifold.HawkesSource
	tree      *dmt.Tree
	resonance map[string]*Resonance
	causal    map[string]*Causal
}

/*
stageGate exposes boot readiness without coupling Analyzer to boot orchestration.
*/
type stageGate interface {
	Ready(system.StageType) bool
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
	gate stageGate,
	api *websocket.API,
	hawkes manifold.HawkesSource,
	tree *dmt.Tree,
) (*Analyzer, error) {
	_ = ctx

	solver, err := manifold.NewSolver(api)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "logic analyzer: manifold init failed", err)
	}

	return &Analyzer{
		gate:      gate,
		status:    types.READY,
		manifold:  solver,
		hawkes:    hawkes,
		tree:      tree,
		resonance: make(map[string]*Resonance),
		causal:    make(map[string]*Causal),
	}, nil
}

func (analyzer *Analyzer) Initialize() error {
	errnie.Info("initializing analyzer")
	analyzer.status = types.READY

	return nil
}

func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
}

func (analyzer *Analyzer) Close() {
	if analyzer.manifold != nil {
		analyzer.manifold.Close()
	}
}

/*
Update delegates Hawkes-driven field analysis after signal measure, then composes
every measurement into its symbol Gonum graph.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	if analyzer.manifold != nil &&
		analyzer.hawkes != nil &&
		analyzer.gate != nil &&
		analyzer.gate.Ready(system.StagePreflight) {
		if err := analyzer.manifold.Update(thesis, analyzer.hawkes); err != nil {
			errnie.Error(err)
		}
	}

	states := make([]manifold.State, 0)

	thesis.Manifold.Range(func(key, value any) bool {
		state, ok := value.(manifold.State)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received invalid manifold state",
				nil,
			))

			return true
		}

		states = append(states, state)
		analyzer.observe(thesis, state)

		return true
	})

	for _, measurement := range thesis.Measurements {
		if measurement == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received a nil measurement",
				nil,
			))

			continue
		}

		value, found := thesis.Graphs.Load(measurement.Symbol)

		if !found {
			value = types.NewGraph(measurement.Symbol)
			thesis.Graphs.Store(measurement.Symbol, value)
		}

		evidenceGraph := value.(*types.Graph)

		if err := evidenceGraph.AddNode(measurement); err != nil {
			errnie.Error(err)
			continue
		}
	}

	thesis.Graphs.Range(func(key, value any) bool {
		evidenceGraph := value.(*types.Graph)
		evidenceGraph.Compose()
		return true
	})

	for _, state := range states {
		analyzer.cognize(thesis, state)
	}
}

/*
observe connects one manifold state to the existing resonance, causal, and
forecast outputs through the Thesis without making those components depend on
the manifold solver.
*/
func (analyzer *Analyzer) observe(
	thesis *types.Thesis,
	state manifold.State,
) {
	if analyzer.resonance == nil {
		analyzer.resonance = make(map[string]*Resonance)
	}

	if analyzer.causal == nil {
		analyzer.causal = make(map[string]*Causal)
	}

	resonance := analyzer.resonance[state.Symbol]

	if resonance == nil {
		resonance = NewResonance(state.Symbol, manifold.DefaultBaselineHalflife())
		analyzer.resonance[state.Symbol] = resonance
	}

	measurements, resonanceOutcome := resonance.Update(state)
	thesis.Measurements = append(thesis.Measurements, measurements...)

	if resonanceOutcome != nil {
		thesis.Resonance = append(thesis.Resonance, resonanceOutcome)
	}

	causal := analyzer.causal[state.Symbol]

	if causal == nil {
		causal = NewCausal(state.Symbol)
		analyzer.causal[state.Symbol] = causal
	}

	hypothesis, causalOutcome, err := causal.Update(state)

	if err != nil {
		errnie.Error(err)
		return
	}

	if causalOutcome != nil {
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		thesis.Causal = append(thesis.Causal, causalOutcome)
	}

	if resonanceOutcome == nil || causalOutcome == nil ||
		!causalOutcome.Ready || causalOutcome.CalibrationSamples == 0 {
		return
	}

	// Candidate notional is capped at the best ask, so the forecast does not
	// claim depth-crossing impact beyond the directly observed touch spread.
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Source: "resonance+causal", Symbol: state.Symbol, At: state.At,
		ObservedInterval: state.Duration, SourceEpoch: state.Epoch,
		HorizonEvents: 1, ExpiresEpoch: state.Epoch + 1,
		Target: causalOutcome.Target, ModelVersion: causalHypothesis,
		Ready: true, Calibrated: true,
		CalibrationSamples:       causalOutcome.CalibrationSamples,
		IncrementalMSE:           causalOutcome.IncrementalMSE,
		IncrementalMSELowerBound: 0,
		ExpectedReturn:           causalOutcome.ExpectedReturn,
		ReferencePrice:           state.ReferencePrice,
		BuyCapacity:              state.BuyCapacity, SellCapacity: state.SellCapacity,
		ExpectedSpread:           state.Spread,
		ExpectedAdverseSelection: math.Max(0, -causalOutcome.Reading.Uplift),
		Uncertainty:              causalOutcome.Uncertainty,
		Confidence: math.Min(
			causalOutcome.Reading.Confidence,
			1/(1+resonanceOutcome.Surprise),
		),
	})
}

/*
cognize turns the Thesis evidence for one manifold state into a deterministic
DMT sensory sequence, reads learned cognition, and writes the decoupled result
back onto the Thesis before learning the current observation.
*/
func (analyzer *Analyzer) cognize(
	thesis *types.Thesis,
	state manifold.State,
) {
	if analyzer.tree == nil || !state.GasReady() {
		return
	}

	parts := make([]string, 0, len(thesis.Measurements)+4)
	seen := make(map[string]struct{}, len(thesis.Measurements)+4)
	replacer := strings.NewReplacer("_", "-", "/", "-")
	parts = append(parts, "symbol-"+replacer.Replace(state.Symbol))

	for _, measurement := range thesis.Measurements {
		if measurement == nil || measurement.Symbol != state.Symbol ||
			measurement.Normalized == nil ||
			measurement.Validity.State != types.ValidityValid {
			continue
		}

		direction := "negative"

		if *measurement.Normalized > 0 {
			direction = "positive"
		}

		if *measurement.Normalized == 0 {
			direction = "zero"
		}

		token := replacer.Replace(strings.Join([]string{
			string(measurement.Source), string(measurement.Metric),
			string(measurement.Side), direction,
		}, "-"))

		if _, exists := seen[token]; exists {
			continue
		}

		seen[token] = struct{}{}
		parts = append(parts, token)
	}

	for name, value := range map[string]float64{
		"pressure":   state.Reading.PressureGradX,
		"divergence": state.Reading.Divergence,
		"stress":     state.StressAnisotropy,
	} {
		direction := "negative"

		if value > 0 {
			direction = "positive"
		}

		if value == 0 {
			direction = "zero"
		}

		parts = append(parts, name+"-"+direction)
	}

	sort.Strings(parts)
	sequence := []byte(strings.Join(parts, "_"))

	if len(sequence) == 0 {
		return
	}

	parent := sequence

	if boundary := bytes.LastIndexByte(sequence, '_'); boundary > 0 {
		parent = sequence[:boundary]
	}

	var classificationScratch dmt.ClassificationScratch
	classification := analyzer.tree.Classify(sequence, &classificationScratch)
	storageParent := append([]byte("s/"), parent...)
	ambiguity := analyzer.tree.MeasureBranchAmbiguity(storageParent)
	predictionBuffer := make([]dmt.LookaheadPrediction, 0, len(parts))
	predictions := analyzer.tree.PredictNextSensoryTokens(parent, predictionBuffer)
	reading := types.Cognition{
		Source: "dmt", Symbol: state.Symbol, At: state.At,
		Sequence: string(sequence), Ready: len(classification.Winner) > 0,
		Winner: string(classification.Winner), Confidence: classification.Highest,
		EntropyBits: ambiguity.EntropyBits, EntropyThreshold: ambiguity.Threshold,
		Ambiguous:   ambiguity.Ambiguous,
		Cohort:      analyzer.tree.GetSensoryWeight(sequence).Count,
		Predictions: make(map[string]float64, len(predictions)),
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

	branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths :=
		analyzer.cognitionVisualization(
			sequence, parent, parts, classification, predictions,
		)

	reading.RegimePrefix = string(parent)
	reading.LookaheadScore = lookaheadScore
	reading.LookaheadPaths = lookaheadPaths
	reading.BeamWidth = beamWidth
	reading.MaxHops = maxHops
	reading.NodeCount = nodeCount
	reading.Branches = branches
	reading.Beams = beams
	reading.Classes = classes

	thesis.Cognition.Store(state.Symbol, reading)
	analyzer.tree.TrainSensorySequence(sequence)

	if _, _, err := analyzer.tree.CommitToEpisodicBuffer(
		uint64(state.At.UnixNano()), sequence,
	); err != nil {
		errnie.Error(err)
	}

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
}
