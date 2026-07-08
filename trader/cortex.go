package trader

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

var cortexSourceOrder = []types.SourceType{
	types.SourceCorrelation,
	types.SourceCVD,
	types.SourceDepthFlow,
	types.SourceExhaustion,
	types.SourceFluid,
	types.SourceHawkes,
	types.SourceLeadLag,
	types.SourceLiquidity,
	types.SourcePumpDump,
	types.SourceSentiment,
	types.SourceToxicity,
}

/*
Cortex keeps the DMT market-memory model current.
It turns live measurements and decision-stage frames into sensory sequences,
trains the tree, then reads back the tree state used by the cognitive surface.
*/
type Cortex struct {
	tree        *dmt.Tree
	topology    *Topology
	router      *CortexRouter
	beamScratch *dmt.BeamSearchScratch
	beamWidth   int
	maxHops     int
}

type CognitiveReading struct {
	Scope              string            `json:"scope"`
	Sequence           string            `json:"sequence"`
	RegimePrefix       string            `json:"regimePrefix"`
	RegimeCohort       uint64            `json:"regimeCohort"`
	Ambiguous          bool              `json:"ambiguous"`
	Sideline           bool              `json:"sideline"`
	EntropyBits        float64           `json:"entropyBits"`
	EntropyThreshold   float64           `json:"entropyThreshold"`
	ClassConfidence    float64           `json:"classConfidence"`
	ContrastEvidence   float64           `json:"contrastEvidence"`
	LookaheadScore     float64           `json:"lookaheadScore"`
	LookaheadPaths     int               `json:"lookaheadPaths"`
	WinnerClass        string            `json:"winnerClass"`
	PrewarmPaths       int               `json:"prewarmPaths"`
	PrewarmScore       float64           `json:"prewarmScore"`
	PredictedReturnBps float64           `json:"predictedReturnBps"`
	CorpusMatchCount   int               `json:"corpusMatchCount"`
	TopSimilarity      float64           `json:"topSimilarity"`
	UpdatedAt          int64             `json:"updatedAt"`
	BeamWidth          int               `json:"beamWidth"`
	MaxHops            int               `json:"maxHops"`
	NodeCount          int               `json:"nodeCount"`
	Branches           []CognitiveBranch `json:"branches"`
	Beams              []CognitiveBeam   `json:"beams"`
	Classes            []CognitiveClass  `json:"classes"`
}

type CognitiveBranch struct {
	ID          int     `json:"id"`
	ParentID    int     `json:"parentId"`
	Token       string  `json:"token"`
	Prefix      string  `json:"prefix"`
	Depth       int     `json:"depth"`
	Probability float64 `json:"probability"`
	Count       uint64  `json:"count"`
}

type CognitiveBeam struct {
	Sequence string  `json:"sequence"`
	Score    float64 `json:"score"`
}

type CognitiveClass struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

type cortexReading struct {
	category types.Category
	metrics  map[string]float64
}

type cortexObservation struct {
	symbol       string
	at           time.Time
	measurements map[types.SourceType]cortexReading
	manifold     *logic.ManifoldFrame
	resonance    *logic.ResonanceFrame
	causal       *logic.CausalFrame
}

type cortexPrediction struct {
	token       string
	label       string
	probability float64
	count       uint64
}

func newCortex(tree *dmt.Tree) *Cortex {
	if tree == nil {
		return nil
	}

	beamWidth := viper.GetViper().GetInt("cognitive.beam_width")

	if beamWidth <= 0 {
		beamWidth = 4
	}

	maxHops := viper.GetViper().GetInt("cognitive.beam_hops")

	if maxHops <= 0 {
		maxHops = 3
	}

	return &Cortex{
		tree:     tree,
		topology: newTopology(),
		router:   NewCortexRouter(),
		beamScratch: &dmt.BeamSearchScratch{
			CurrentBeams: make([]dmt.BeamPath, 0, beamWidth),
			NextBeams:    make([]dmt.BeamPath, 0, beamWidth),
			LookupBuffer: make([]dmt.LookaheadPrediction, 0, 32),
		},
		beamWidth: beamWidth,
		maxHops:   maxHops,
	}
}

func (cortex *Cortex) Measure(
	measurements []*types.Measurement,
	batch logic.Batch,
) (map[string]CognitiveReading, error) {
	if cortex == nil || cortex.tree == nil {
		return nil, nil
	}

	observations := cortex.observations(measurements, batch)
	readings := make(map[string]CognitiveReading, len(observations))

	for _, observation := range observations {
		sequence, err := cortex.topology.Sequence(observation)

		if err != nil {
			return nil, err
		}

		if len(sequence.Tree) == 0 {
			continue
		}

		treeTokens := append([]string{sequence.Symbol}, sequence.Tree...)

		if err := cortex.train(observation, sequence.Class, treeTokens); err != nil {
			return nil, err
		}

		routing := cortex.router.Route(observation)

		readings[observation.symbol] = cortex.reading(
			observation,
			sequence.Display,
			treeTokens,
			routing,
		)
	}

	return readings, nil
}

func (cortex *Cortex) observations(
	measurements []*types.Measurement,
	batch logic.Batch,
) map[string]*cortexObservation {
	observations := map[string]*cortexObservation{}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		observation := cortex.observation(observations, measurement.Symbol)

		if observation == nil {
			continue
		}

		category := strongestCategory(measurement.Categories)

		if category.Type == types.CategoryTypeNone {
			continue
		}

		observation.measurements[measurement.Source] = cortexReading{
			category: category,
			metrics:  measurement.Metrics,
		}

		if measurement.At.After(observation.at) {
			observation.at = measurement.At
		}
	}

	for _, frame := range batch.Manifold {
		if frame == nil {
			continue
		}

		observation := cortex.observation(observations, frame.Symbol)

		if observation != nil {
			observation.manifold = frame
		}
	}

	for _, frame := range batch.Resonance {
		if frame == nil {
			continue
		}

		observation := cortex.observation(observations, frame.Symbol)

		if observation != nil {
			observation.resonance = frame
		}
	}

	for _, frame := range batch.Causal {
		if frame == nil {
			continue
		}

		observation := cortex.observation(observations, frame.Symbol)

		if observation != nil {
			observation.causal = frame
		}
	}

	return observations
}

func (cortex *Cortex) observation(
	observations map[string]*cortexObservation,
	symbol string,
) *cortexObservation {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return nil
	}

	observation := observations[symbol]

	if observation != nil {
		return observation
	}

	observation = &cortexObservation{
		symbol:       symbol,
		measurements: map[types.SourceType]cortexReading{},
	}
	observations[symbol] = observation

	return observation
}

func (cortex *Cortex) train(
	observation *cortexObservation,
	class string,
	tokens []string,
) error {
	sequence := []byte(strings.Join(tokens, "_"))

	cortex.tree.TrainSensorySequence(sequence)

	if err := cortex.trainBasin([]byte(class), tokens); err != nil {
		return err
	}

	if observation.at.IsZero() {
		return nil
	}

	_, _, err := cortex.tree.CommitToEpisodicBuffer(
		uint64(observation.at.UnixNano()),
		sequence,
	)

	return err
}

func (cortex *Cortex) trainBasin(class []byte, tokens []string) error {
	for index := range tokens {
		prefix := strings.Join(tokens[:index+1], "_")
		parentPrefix := strings.Join(tokens[:index], "_")
		current := cortex.tree.GetAttractorBasin(class, []byte(prefix))
		nextCount := current.Count + 1
		probability := 1.0

		if parentPrefix != "" {
			parent := cortex.tree.GetAttractorBasin(class, []byte(parentPrefix))
			denominator := float64(parent.Count)

			if denominator <= 0 {
				denominator = float64(nextCount)
			}

			probability = float64(nextCount) / denominator

			if probability > 1 {
				probability = 1
			}
		}

		_, _, err := cortex.tree.InsertAttractorBasin(
			class,
			[]byte(prefix),
			dmt.CognitiveState{
				Count:       nextCount,
				Probability: probability,
			},
		)

		if err != nil {
			return err
		}
	}

	return nil
}

func (cortex *Cortex) reading(
	observation *cortexObservation,
	tokens []string,
	treeTokens []string,
	routing ContinuousRouting,
) CognitiveReading {
	sequence := strings.Join(tokens, "_")
	treeSequence := strings.Join(treeTokens, "_")
	regimePrefix := parentSequence(sequence)
	treeRegimePrefix := parentSequence(treeSequence)
	entropyBits, entropyThreshold, ambiguous := cortex.entropy(treeRegimePrefix)
	cohort := cortex.cohort(treeRegimePrefix)
	classes, confidence, winnerClass, contrastEvidence := cortex.classes(treeSequence)
	beams, lookaheadScore := cortex.beams(treeTokens[0])
	branches := cortex.branches(treeTokens[0], cortex.maxHops)
	updatedAt := observation.at

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	return CognitiveReading{
		Scope:              observation.symbol,
		Sequence:           sequence,
		RegimePrefix:       regimePrefix,
		RegimeCohort:       cohort,
		Ambiguous:          ambiguous,
		Sideline:           len(classes) == 0,
		EntropyBits:        entropyBits,
		EntropyThreshold:   entropyThreshold,
		ClassConfidence:    confidence,
		ContrastEvidence:   contrastEvidence,
		LookaheadScore:     lookaheadScore,
		LookaheadPaths:     len(beams),
		WinnerClass:        winnerClass,
		PrewarmPaths:       len(beams),
		PrewarmScore:       lookaheadScore,
		PredictedReturnBps: routing.PredictedReturnBps,
		CorpusMatchCount:   routing.MatchCount,
		TopSimilarity:      routing.TopSimilarity,
		UpdatedAt:          updatedAt.UnixMilli(),
		BeamWidth:          cortex.beamWidth,
		MaxHops:            cortex.maxHops,
		NodeCount:          len(branches),
		Branches:           branches,
		Beams:              beams,
		Classes:            classes,
	}
}

func (cortex *Cortex) branches(rootPrefix string, maxDepth int) []CognitiveBranch {
	branches := []CognitiveBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "root",
		Prefix:      "",
		Depth:       0,
		Probability: 1,
		Count:       cortex.cohort(rootPrefix),
	}}

	var walk func(treePrefix string, displayPrefix string, parentID int, depth int)
	walk = func(treePrefix string, displayPrefix string, parentID int, depth int) {
		if depth >= maxDepth {
			return
		}

		predictions := cortex.predictions(treePrefix)

		for _, prediction := range predictions {
			childTreePrefix := joinSequence(treePrefix, prediction.token)
			childDisplayPrefix := joinSequence(displayPrefix, prediction.label)
			branch := CognitiveBranch{
				ID:          len(branches),
				ParentID:    parentID,
				Token:       prediction.label,
				Prefix:      childDisplayPrefix,
				Depth:       depth + 1,
				Probability: prediction.probability,
				Count:       prediction.count,
			}
			branches = append(branches, branch)
			walk(childTreePrefix, childDisplayPrefix, branch.ID, depth+1)
		}
	}

	walk(rootPrefix, "", 0, 0)

	return branches
}

func (cortex *Cortex) beams(rootPrefix string) ([]CognitiveBeam, float64) {
	paths := cortex.tree.ExecuteBeamSearch(
		[]byte(rootPrefix),
		cortex.beamWidth,
		cortex.maxHops,
		cortex.beamScratch,
	)
	beams := make([]CognitiveBeam, 0, len(paths))
	bestScore := 0.0

	for index, path := range paths {
		if index == 0 {
			bestScore = path.Score
		}

		beams = append(beams, CognitiveBeam{
			Sequence: cortex.topology.DisplayPath(rootPrefix, string(path.Sequence)),
			Score:    path.Score,
		})
	}

	return beams, bestScore
}

func (cortex *Cortex) classes(sequence string) (
	[]CognitiveClass,
	float64,
	string,
	float64,
) {
	var scratch dmt.ClassificationScratch

	classification := cortex.tree.Classify([]byte(sequence), &scratch)
	classes := make([]CognitiveClass, 0, len(classification.Scores))

	for _, score := range classification.Scores {
		classes = append(classes, CognitiveClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	if len(classification.Scores) < 2 {
		return classes, classification.Highest, string(classification.Winner), 0
	}

	evidence := cortex.tree.ComputeBasinContrastiveEvidence(
		append([]byte(nil), classification.Scores[0].ClassName...),
		append([]byte(nil), classification.Scores[1].ClassName...),
		[]byte(sequence),
	)

	return classes,
		classification.Highest,
		string(classification.Winner),
		evidence.Divergence
}

func (cortex *Cortex) entropy(prefix string) (float64, float64, bool) {
	predictions := cortex.predictions(prefix)

	if len(predictions) <= 1 {
		return 0, 0, false
	}

	entropyBits := 0.0

	for _, prediction := range predictions {
		if prediction.probability <= 0 {
			continue
		}

		entropyBits -= prediction.probability * math.Log2(prediction.probability)
	}

	parentState := cortex.tree.GetSensoryWeight([]byte(prefix))
	parentUncertainty := 1.0 - parentState.Probability

	if parentState.Probability <= 0 {
		parentUncertainty = 1
	}

	uniformEntropy := math.Log2(float64(len(predictions)))
	threshold := uniformEntropy * (1 - parentUncertainty/float64(len(predictions)))

	return entropyBits, threshold, entropyBits >= threshold
}

func (cortex *Cortex) predictions(prefix string) []cortexPrediction {
	var lookahead [32]dmt.LookaheadPrediction

	raw := cortex.tree.PredictNextSensoryTokens([]byte(prefix), lookahead[:0])
	predictions := make([]cortexPrediction, 0, len(raw))
	probabilityMass := 0.0

	for _, prediction := range raw {
		probabilityMass += prediction.Probability
	}

	for _, prediction := range raw {
		token := string(prediction.Token)
		label := cortex.topology.Label(token)
		childPrefix := joinSequence(prefix, token)
		probability := prediction.Probability

		if probabilityMass > 0 {
			probability = probability / probabilityMass
		}

		predictions = append(predictions, cortexPrediction{
			token:       token,
			label:       label,
			probability: probability,
			count:       cortex.tree.GetSensoryWeight([]byte(childPrefix)).Count,
		})
	}

	sort.Slice(predictions, func(leftIndex, rightIndex int) bool {
		left := predictions[leftIndex]
		right := predictions[rightIndex]

		if left.probability == right.probability {
			return left.label < right.label
		}

		return left.probability > right.probability
	})

	return predictions
}

func (cortex *Cortex) cohort(prefix string) uint64 {
	if prefix != "" {
		return cortex.tree.GetSensoryWeight([]byte(prefix)).Count
	}

	cohort := uint64(0)

	for _, prediction := range cortex.predictions(prefix) {
		cohort += prediction.count
	}

	return cohort
}

func (observation *cortexObservation) class() string {
	if observation.causal != nil {
		return token(string(observation.causal.Category))
	}

	if observation.resonance != nil {
		return token(string(observation.resonance.Category))
	}

	if observation.manifold != nil {
		return token(string(observation.manifold.Category))
	}

	for _, source := range cortexSourceOrder {
		reading := observation.measurements[source]

		if reading.category.Type != types.CategoryTypeNone {
			return token(string(reading.category.Type))
		}
	}

	return "unclassified"
}

func strongestCategory(categories []types.Category) types.Category {
	best := types.Category{Type: types.CategoryTypeNone}

	for _, category := range categories {
		if category.Type == types.CategoryTypeNone {
			continue
		}

		if best.Type == types.CategoryTypeNone {
			best = category
			continue
		}

		if category.Strength > best.Strength {
			best = category
			continue
		}

		if category.Strength == best.Strength && category.Confidence > best.Confidence {
			best = category
		}
	}

	return best
}

func joinSequence(prefix string, token string) string {
	if prefix == "" {
		return token
	}

	return prefix + "_" + token
}

func displaySequence(rootPrefix string, sequence string) string {
	if sequence == rootPrefix {
		return ""
	}

	prefix := rootPrefix + "_"

	if strings.HasPrefix(sequence, prefix) {
		return strings.TrimPrefix(sequence, prefix)
	}

	return sequence
}

func parentSequence(sequence string) string {
	index := strings.LastIndex(sequence, "_")

	if index < 0 {
		return ""
	}

	return sequence[:index]
}

func tokenPair(left string, right string) string {
	return token(left + "-" + right)
}

func token(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	previousDash := false

	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			previousDash = false
			continue
		}

		if previousDash {
			continue
		}

		builder.WriteByte('-')
		previousDash = true
	}

	out := strings.Trim(builder.String(), "-")

	if out == "" {
		return "unknown"
	}

	return out
}
