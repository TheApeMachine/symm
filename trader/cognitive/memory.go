package cognitive

import (
	"bytes"
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
)

/*
Observation is one classified signal reading for a symbol measure cycle.
*/
type Observation struct {
	Token      string
	Confidence float64
}

/*
Reading is the cognitive state derived after sealing a symbol's measure cycle.
*/
type Reading struct {
	Scope            string
	Sequence         []byte
	RegimePrefix     []byte
	RegimeCohort     []string
	Ambiguous        bool
	EntropyBits      float64
	EntropyThreshold float64
	Sideline         bool
	WinnerClass      []byte
	ClassConfidence  float64
	ContrastEvidence float64
	LookaheadScore   float64
	LookaheadPaths   []string
}

type pendingScope struct {
	observations []Observation
}

/*
Memory binds DMT cognitive namespaces to the shared process tree singleton.
*/
type Memory struct {
	ctx              context.Context
	cancel           context.CancelFunc
	tree             *dmt.Tree
	classifyScratch  dmt.ClassificationScratch
	beamScratch      dmt.BeamSearchScratch
	pending          sync.Map
	readings         sync.Map
	beamWidth        int
	beamHops         int
	lastConsolidated time.Time
	remInterval      time.Duration
}

/*
NewMemory constructs cognitive memory on the shared DMT tree singleton.
*/
func NewMemory(ctx context.Context) *Memory {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	beamWidth := viper.GetInt("cognitive.beam_width")

	if beamWidth <= 0 {
		beamWidth = 4
	}

	beamHops := viper.GetInt("cognitive.beam_hops")

	if beamHops <= 0 {
		beamHops = 3
	}

	remInterval := viper.GetDuration("cognitive.rem_interval")

	if remInterval <= 0 {
		remInterval = time.Hour
	}

	return &Memory{
		ctx:         ctx,
		cancel:      cancel,
		tree:        SharedTree(),
		beamWidth:   beamWidth,
		beamHops:    beamHops,
		remInterval: remInterval,
	}
}

/*
ObserveArtifact records one signal reading for the current measure cycle.
*/
func (memory *Memory) ObserveArtifact(
	scope string,
	signalName string,
	artifact *datura.Artifact,
) {
	if memory == nil || artifact == nil || scope == "" || signalName == "" {
		return
	}

	category, confidence, ok := categoryFromArtifact(signalName, artifact)

	if !ok {
		return
	}

	memory.observe(scope, Observation{
		Token:      string(category),
		Confidence: confidence,
	})
}

/*
ObserveMeasurement records one logic.Measurement for the current measure cycle.
*/
func (memory *Memory) ObserveMeasurement(measurement logic.Measurement) {
	if memory == nil || measurement.Symbol == "" || measurement.Category == "" {
		return
	}

	if measurement.Confidence <= 0 {
		return
	}

	memory.observe(measurement.Symbol, Observation{
		Token:      string(measurement.Category),
		Confidence: measurement.Confidence,
	})
}

func (memory *Memory) observe(scope string, observation Observation) {
	raw, _ := memory.pending.LoadOrStore(scope, &pendingScope{})
	batch, ok := raw.(*pendingScope)

	if !ok {
		return
	}

	batch.observations = append(batch.observations, observation)
}

/*
SealScope finalizes the measure cycle for scope and updates cognitive memory.
*/
func (memory *Memory) SealScope(scope string, eventAt time.Time) *Reading {
	if memory == nil || scope == "" {
		return nil
	}

	raw, loaded := memory.pending.LoadAndDelete(scope)

	if !loaded {
		return nil
	}

	batch, ok := raw.(*pendingScope)

	if !ok || len(batch.observations) == 0 {
		return nil
	}

	sequence := buildSequence(batch.observations, scope)

	if len(sequence) == 0 {
		return nil
	}

	memory.tree.TrainSensorySequence(sequence)

	inferredClass, classConfidence, learnErr := memory.tree.UnsupervisedLearn(
		sequence,
		&memory.classifyScratch,
	)

	if learnErr != nil {
		inferredClass = nil
		classConfidence = 0
	}

	classification := memory.tree.Classify(sequence, &memory.classifyScratch)
	regimePrefix := regimePrefixFromSequence(sequence)
	entropyBits, entropyThreshold, ambiguous := memory.sensoryAmbiguity(regimePrefix)
	lookaheadPaths, lookaheadScore := memory.beamPaths(regimePrefix)
	contrastEvidence := memory.basinContrast(classification)

	reading := &Reading{
		Scope:            scope,
		Sequence:         append([]byte(nil), sequence...),
		RegimePrefix:     append([]byte(nil), regimePrefix...),
		RegimeCohort:     memory.RegimeCohort(regimePrefix),
		Ambiguous:        ambiguous,
		EntropyBits:      entropyBits,
		EntropyThreshold: entropyThreshold,
		Sideline:         ambiguous,
		WinnerClass:      append([]byte(nil), inferredClass...),
		ClassConfidence:  classConfidence,
		ContrastEvidence: contrastEvidence,
		LookaheadScore:   lookaheadScore,
		LookaheadPaths:   lookaheadPaths,
	}

	if len(classification.Winner) > 0 && reading.ClassConfidence <= 0 {
		reading.WinnerClass = append([]byte(nil), classification.Winner...)
		reading.ClassConfidence = classification.Highest
	}

	memory.readings.Store(scope, reading)

	return reading
}

/*
SealAllScopes finalizes every pending scope in the current measure tick.
*/
func (memory *Memory) SealAllScopes(scopes []string, eventAt time.Time) []*Reading {
	readings := make([]*Reading, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		reading := memory.SealScope(scope, eventAt)

		if reading != nil {
			readings = append(readings, reading)
			seen[scope] = struct{}{}
		}
	}

	memory.pending.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		if _, alreadySealed := seen[scope]; alreadySealed {
			return true
		}

		reading := memory.SealScope(scope, eventAt)

		if reading != nil {
			readings = append(readings, reading)
		}

		return true
	})

	return readings
}

/*
LatestReadings returns every sealed scope reading sorted by scope.
*/
func (memory *Memory) LatestReadings() []*Reading {
	if memory == nil {
		return nil
	}

	readings := make([]*Reading, 0, 8)

	memory.readings.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		reading, ok := value.(*Reading)

		if !ok || reading == nil {
			return true
		}

		readings = append(readings, reading)

		return true
	})

	sort.Slice(readings, func(i, j int) bool {
		return readings[i].Scope < readings[j].Scope
	})

	return readings
}

/*
ReadingForScope returns the latest sealed cognitive reading for scope.
*/
func (memory *Memory) ReadingForScope(scope string) (*Reading, bool) {
	if memory == nil || scope == "" {
		return nil, false
	}

	raw, ok := memory.readings.Load(scope)

	if !ok {
		return nil, false
	}

	reading, ok := raw.(*Reading)

	return reading, ok
}

/*
Sideline reports whether trading should pause for scope due to branch ambiguity.
*/
func (memory *Memory) Sideline(scope string) bool {
	reading, ok := memory.ReadingForScope(scope)

	if !ok || reading == nil {
		return false
	}

	return reading.Sideline
}

/*
RegimeCohort lists every symbol suffix currently present under a regime prefix.
*/
func (memory *Memory) RegimeCohort(regimePrefix []byte) []string {
	if memory == nil || memory.tree == nil || len(regimePrefix) == 0 {
		return nil
	}

	searchPrefix := sensorySearchPrefix(regimePrefix)
	seen := make(map[string]struct{})
	symbols := make([]string, 0, 8)

	memory.tree.WalkPrefix(searchPrefix, func(key, value []byte) bool {
		if !bytes.HasPrefix(key, []byte(sensoryNamespace)) {
			return true
		}

		sequence := key[len(sensoryNamespace):]
		symbol := symbolFromSequence(sequence)

		if symbol == "" {
			return true
		}

		if _, exists := seen[symbol]; exists {
			return true
		}

		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)

		return true
	})

	sort.Strings(symbols)

	return symbols
}

/*
BeamPathStrings returns ranked multi-hop paths from a regime prefix.
*/
func (memory *Memory) BeamPathStrings(regimePrefix []byte) []string {
	paths, _ := memory.beamPaths(regimePrefix)

	return paths
}

/*
MaybeConsolidate runs REM replay and decay when the configured interval elapses.
*/
func (memory *Memory) MaybeConsolidate(now time.Time) {
	if memory == nil || memory.tree == nil {
		return
	}

	if memory.remInterval <= 0 {
		return
	}

	if !memory.lastConsolidated.IsZero() && now.Sub(memory.lastConsolidated) < memory.remInterval {
		return
	}

	windowStart := uint64(0)

	if !memory.lastConsolidated.IsZero() {
		windowStart = uint64(memory.lastConsolidated.UnixNano())
	}

	windowEnd := uint64(now.UnixNano())

	memory.tree.ExecuteREMSleepConsolidation(windowStart, windowEnd)
	memory.lastConsolidated = now
}

/*
LookupProfile resolves execution parameters, falling back to structural analog keys.
*/
func (memory *Memory) LookupProfile(sequence []byte) ([]byte, bool) {
	if memory == nil || memory.tree == nil || len(sequence) == 0 {
		return nil, false
	}

	profileKey := execProfileKey(sequence)
	value, found := memory.tree.Get(profileKey)

	if found {
		return value, true
	}

	analog, hasAnalog := memory.tree.FindStructuralAnalog(profileKey)

	if !hasAnalog {
		return nil, false
	}

	return memory.tree.Get(analog.ClosestKey)
}

/*
GetAnalogousFallback resolves a key directly or via longest shared-prefix analog.
*/
func (memory *Memory) GetAnalogousFallback(key []byte) ([]byte, bool) {
	if memory == nil || memory.tree == nil || len(key) == 0 {
		return nil, false
	}

	value, found := memory.tree.Get(key)

	if found {
		return value, true
	}

	analog, hasAnalog := memory.tree.FindStructuralAnalog(key)

	if !hasAnalog {
		return nil, false
	}

	return memory.tree.Get(analog.ClosestKey)
}

/*
StoreProfile writes execution parameters at a regime-first profile key.
*/
func (memory *Memory) StoreProfile(sequence []byte, profile []byte) (*dmt.Tree, bool) {
	if memory == nil || memory.tree == nil || len(sequence) == 0 || len(profile) == 0 {
		return nil, false
	}

	return memory.tree.Insert(execProfileKey(sequence), append([]byte(nil), profile...))
}

func (memory *Memory) sensoryAmbiguity(regimePrefix []byte) (float64, float64, bool) {
	if memory == nil || memory.tree == nil || len(regimePrefix) == 0 {
		return 0, 0, false
	}

	var predictions [32]dmt.LookaheadPrediction

	branches := memory.tree.PredictNextSensoryTokens(regimePrefix, predictions[:0])
	entropyBits := branchEntropy(branches)

	if len(branches) <= 1 {
		return entropyBits, math.MaxFloat64, false
	}

	parentState := memory.tree.GetSensoryWeight(regimePrefix)
	threshold := ambiguityEntropyThreshold(len(branches), parentState)

	return entropyBits, threshold, entropyBits >= threshold
}

func (memory *Memory) beamPaths(regimePrefix []byte) ([]string, float64) {
	if memory == nil || memory.tree == nil || len(regimePrefix) == 0 {
		return nil, 0
	}

	paths := memory.tree.ExecuteBeamSearch(
		regimePrefix,
		memory.beamWidth,
		memory.beamHops,
		&memory.beamScratch,
	)

	if len(paths) == 0 {
		return nil, 0
	}

	strings := make([]string, 0, len(paths))

	for _, path := range paths {
		strings = append(strings, string(path.Sequence))
	}

	return strings, paths[0].Score
}

func (memory *Memory) basinContrast(result dmt.ClassificationResult) float64 {
	if memory == nil || memory.tree == nil || len(result.Scores) < 2 {
		return 0
	}

	evidence := memory.tree.ComputeBasinContrastiveEvidence(
		result.Scores[0].ClassName,
		result.Scores[1].ClassName,
		result.Scores[0].ClassName,
	)

	return evidence.Divergence
}

func categoryFromArtifact(
	signalName string,
	artifact *datura.Artifact,
) (logic.CategoryType, float64, bool) {
	measurement, ok := logic.MeasurementFromArtifact(signalName, artifact)

	if !ok || measurement.Category == "" || measurement.Confidence <= 0 {
		return "", 0, false
	}

	return measurement.Category, measurement.Confidence, true
}

func buildSequence(observations []Observation, scope string) []byte {
	if len(observations) == 0 || scope == "" {
		return nil
	}

	sorted := append([]Observation(nil), observations...)
	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		return sorted[leftIndex].Confidence > sorted[rightIndex].Confidence
	})

	sequence := make([]byte, 0, 128)

	for _, observation := range sorted {
		if observation.Token == "" {
			continue
		}

		if len(sequence) > 0 {
			sequence = append(sequence, '_')
		}

		sequence = append(sequence, observation.Token...)
	}

	if len(sequence) == 0 {
		return nil
	}

	sequence = append(sequence, '_')
	sequence = append(sequence, scope...)

	return sequence
}

func regimePrefixFromSequence(sequence []byte) []byte {
	lastSeparator := bytes.LastIndexByte(sequence, '_')

	if lastSeparator <= 0 {
		return append([]byte(nil), sequence...)
	}

	return append([]byte(nil), sequence[:lastSeparator]...)
}

func branchEntropy(branches []dmt.LookaheadPrediction) float64 {
	if len(branches) <= 1 {
		return 0
	}

	probabilityMass := 0.0

	for _, branch := range branches {
		probabilityMass += branch.Probability
	}

	if probabilityMass <= 0 {
		return 0
	}

	entropyBits := 0.0

	for _, branch := range branches {
		normalizedProbability := branch.Probability / probabilityMass

		if normalizedProbability <= 0 {
			continue
		}

		entropyBits -= normalizedProbability * math.Log2(normalizedProbability)
	}

	return entropyBits
}

func ambiguityEntropyThreshold(branchCount int, parentState dmt.CognitiveState) float64 {
	if branchCount <= 1 {
		return math.MaxFloat64
	}

	uniformEntropy := math.Log2(float64(branchCount))
	parentUncertainty := 1.0 - parentState.Probability

	if parentState.Probability <= 0 {
		parentUncertainty = 1.0
	}

	return uniformEntropy * (1.0 - parentUncertainty/float64(branchCount))
}

/*
Close shuts down cognitive memory without closing the shared tree singleton.
*/
func (memory *Memory) Close() error {
	if memory == nil {
		return nil
	}

	memory.cancel()

	return nil
}
