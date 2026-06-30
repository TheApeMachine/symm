package market

import (
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
)

/*
CognitiveReading is the per-symbol cognitive summary the Cortex surface renders.
It is read from the dmt.Tree cognitive engine (sensory prefix tree, attractor-basin
classification, beam-search lookahead, token surprisal) — not invented downstream.
Each tick a symbol's signal categories are encoded into a sensory token sequence,
the engine is trained on it, and the engine's own outputs (winner class, posterior
spread, surprisal, beam paths) populate this reading.
*/
type CognitiveReading struct {
	Scope            string            `json:"scope"`
	Sequence         string            `json:"sequence"`
	RegimePrefix     string            `json:"regimePrefix"`
	RegimeCohort     int               `json:"regimeCohort"`
	Ambiguous        bool              `json:"ambiguous"`
	Sideline         bool              `json:"sideline"`
	EntropyBits      float64           `json:"entropyBits"`
	EntropyThreshold float64           `json:"entropyThreshold"`
	Surprisal        float64           `json:"surprisal"`
	Surprise         float64           `json:"surprise"`
	ClassConfidence  float64           `json:"classConfidence"`
	ContrastEvidence float64           `json:"contrastEvidence"`
	LookaheadScore   float64           `json:"lookaheadScore"`
	LookaheadPaths   int               `json:"lookaheadPaths"`
	WinnerClass      string            `json:"winnerClass"`
	UpdatedAt        int64             `json:"updatedAt"`
	BeamWidth        int               `json:"beamWidth"`
	MaxHops          int               `json:"maxHops"`
	NodeCount        int               `json:"nodeCount"`
	Branches         []CognitiveBranch `json:"branches,omitempty"`
	Beams            []CognitiveBeam   `json:"beams,omitempty"`
	Classes          []CognitiveClass  `json:"classes,omitempty"`
}

type cognitiveToken struct {
	origin     string
	category   string
	confidence float64
	stamp      int64
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

// Beam search bounds the Cortex surface renders: four candidate paths, three hops
// of lookahead — the same width/depth the cortex tree visualization expects.
const (
	cognitiveBeamWidth = 4
	cognitiveMaxHops   = 3
)

/*
CognitiveReadings encodes each symbol's signal categories into a sensory token
sequence, trains the dmt.Tree cognitive engine on it, and reads the engine's own
classification, surprisal, and beam-search outputs back into one reading per
symbol. The tree is the shared cognitive store (cognitive.persist_dir); training
mutates it in place so the engine learns the market's regime sequences over time.
*/
func CognitiveReadings(
	tree *dmt.Tree,
	measurements []*datura.Artifact,
) map[string]CognitiveReading {
	if tree == nil {
		return nil
	}

	bySymbol := make(map[string]map[string]cognitiveToken)
	stampBySymbol := make(map[string]int64)

	for _, measurement := range measurements {
		symbol, err := measurement.Scope()

		if err != nil || symbol == "" {
			continue
		}

		origin, err := measurement.Origin()

		if err != nil || origin == "" {
			continue
		}

		confidence := datura.Peek[float64](measurement, "output", "confidence")

		if confidence <= 0 {
			continue
		}

		categoryIndex := int(datura.Peek[float64](measurement, "output", "category"))
		category, ok := logic.Categories[categoryIndex]

		if !ok || category == logic.CategoryTypeNone {
			continue
		}

		if bySymbol[symbol] == nil {
			bySymbol[symbol] = make(map[string]cognitiveToken)
		}

		stamp := measurement.Timestamp()
		token := cognitiveToken{
			origin:     origin,
			category:   string(category),
			confidence: confidence,
			stamp:      stamp,
		}

		prior, exists := bySymbol[symbol][origin]

		if !exists || token.confidence > prior.confidence ||
			(token.confidence == prior.confidence && token.stamp > prior.stamp) {
			bySymbol[symbol][origin] = token
		}

		if stamp > stampBySymbol[symbol] {
			stampBySymbol[symbol] = stamp
		}
	}

	readings := make(map[string]CognitiveReading, len(bySymbol))
	classifyScratch := &dmt.ClassificationScratch{}
	beamScratch := &dmt.BeamSearchScratch{}

	for symbol, byOrigin := range bySymbol {
		tokens := make([]cognitiveToken, 0, len(byOrigin))

		for _, token := range byOrigin {
			tokens = append(tokens, token)
		}

		readings[symbol] = readingFromEngine(
			tree,
			symbol,
			tokens,
			stampBySymbol[symbol],
			classifyScratch,
			beamScratch,
		)
	}

	return readings
}

/*
ApplyCognitiveReadings stamps each live measurement with the DMT cognitive
surprisal for its symbol. The values come from CognitiveReadings, which reads the
shared dmt.Tree before training on the current sequence. Downstream playbook,
trader, and UI code must read these backend fields instead of inventing
surprise/status from presentation state.
*/
func ApplyCognitiveReadings(
	measurements []*datura.Artifact,
	readings map[string]CognitiveReading,
) {
	if len(measurements) == 0 || len(readings) == 0 {
		return
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol, err := measurement.Scope()

		if err != nil || symbol == "" {
			continue
		}

		reading, ok := readings[symbol]

		if !ok {
			continue
		}

		surprise := reading.Surprise

		if surprise == 0 && reading.EntropyThreshold > 0 {
			surprise = reading.EntropyBits / reading.EntropyThreshold
		}

		measurement.MergeOutputs(map[string]any{
			"surprisal":                reading.Surprisal,
			"surprise":                 surprise,
			"surpriseThreshold":        reading.EntropyThreshold,
			"cognitiveClassConfidence": reading.ClassConfidence,
			"cognitiveSequence":        reading.Sequence,
			"status":                   cognitiveMeasurementStatus(measurement, reading),
		})
	}
}

/*
sensorySequence encodes a symbol's signal tokens into the underscore-delimited
sensory sequence the dmt engine trains on. Tokens are ordered by confidence so the
most-trusted regime leads the prefix, and category names are sanitised to the
engine's token alphabet (underscores delimit tokens, so they cannot appear inside
one).
*/
func sensorySequence(tokens []cognitiveToken) string {
	parts := make([]string, 0, len(tokens))

	for _, token := range tokens {
		category := strings.ReplaceAll(token.category, "_", "-")

		if category == "" {
			continue
		}

		parts = append(parts, category)
	}

	return strings.Join(parts, "_")
}

/*
readingFromEngine trains the cognitive engine on a symbol's sensory sequence and
reads its classification, surprisal, and beam-search outputs into a reading. Every
scored field comes from the engine: the winner and posterior from Classify, the
entropy from token surprisal, the lookahead from ExecuteBeamSearch.
*/
func readingFromEngine(
	tree *dmt.Tree,
	symbol string,
	tokens []cognitiveToken,
	stamp int64,
	classifyScratch *dmt.ClassificationScratch,
	beamScratch *dmt.BeamSearchScratch,
) CognitiveReading {
	sort.SliceStable(tokens, func(first, second int) bool {
		if tokens[first].confidence != tokens[second].confidence {
			return tokens[first].confidence > tokens[second].confidence
		}

		if tokens[first].origin != tokens[second].origin {
			return tokens[first].origin < tokens[second].origin
		}

		return tokens[first].category < tokens[second].category
	})

	sequence := sensorySequence(tokens)

	reading := CognitiveReading{
		Scope:        symbol,
		Sequence:     sequence,
		RegimeCohort: len(tokens),
		UpdatedAt:    stamp,
		BeamWidth:    cognitiveBeamWidth,
		MaxHops:      cognitiveMaxHops,
	}

	if sequence == "" {
		return reading
	}

	sequenceBytes := []byte(sequence)

	// Predict BEFORE learning — this is the predictive-coding order. Entropy,
	// classification, and beam lookahead read the engine's state as it stands
	// having NOT yet trained on this exact sequence, so they measure genuine
	// prediction error against prior experience. Training first would memorise the
	// sequence (probability→1, surprisal→0) and report fabricated certainty.
	tokenCount := len(tokens)
	reading.EntropyBits = sensorySurprisal(tree, sequenceBytes)
	reading.Surprisal = reading.EntropyBits

	// The gate is the surprisal a maximally-novel sequence of this length carries
	// (every token unpredicted): tokens × log2(alphabet). A read is ambiguous when
	// its measured surprisal approaches that ceiling — the engine has learned
	// little structure for this regime, so no regime can be trusted.
	tokenCeiling := math.Log2(math.Max(2, float64(tokenCount)))
	reading.EntropyThreshold = float64(tokenCount) * tokenCeiling * cognitiveAmbiguityFraction
	if reading.EntropyThreshold > 0 {
		reading.Surprise = reading.EntropyBits / reading.EntropyThreshold
	}
	reading.Ambiguous = reading.EntropyThreshold > 0 &&
		reading.EntropyBits >= reading.EntropyThreshold

	// Classification posterior over attractor basins learned on prior ticks.
	result := tree.Classify(sequenceBytes, classifyScratch)
	reading.ClassConfidence = result.Highest
	reading.Classes = cognitiveClasses(result)

	if len(result.Scores) > 1 {
		reading.ContrastEvidence = math.Max(0, result.Scores[0].Value-result.Scores[1].Value)
	} else {
		reading.ContrastEvidence = result.Highest
	}

	// Beam-search lookahead over the sensory prefixes learned so far.
	beams := tree.ExecuteBeamSearch(
		nil,
		cognitiveBeamWidth,
		cognitiveMaxHops,
		beamScratch,
	)
	reading.LookaheadPaths = len(beams)
	reading.LookaheadScore = bestBeamScore(beams)
	reading.Beams = cognitiveBeams(beams)

	// The winning regime: the classifier's winner once a basin exists, otherwise
	// the leading category names the nascent regime this tick is establishing.
	winnerClass := tokens[0].category

	if result.Highest > 0 && len(result.Winner) > 0 {
		winnerClass = string(result.Winner)
	}

	reading.WinnerClass = winnerClass
	reading.RegimePrefix = winnerClass

	// Now LEARN: train the prefix hierarchy and accumulate this sequence into the
	// winning regime's attractor basin. Reinforcing the basin (count grows with
	// each recurrence) is what lets Classify discriminate regimes on later ticks —
	// without a basin the engine would forever report no attractor match.
	tree.TrainSensorySequence(sequenceBytes)

	classBytes := []byte(winnerClass)
	priorBasin := tree.GetAttractorBasin(classBytes, sequenceBytes)
	tree.InsertAttractorBasin(
		classBytes,
		sequenceBytes,
		dmt.CognitiveState{Count: priorBasin.Count + 1, Probability: 1},
	)

	reading.Branches = cognitiveBranches(tree, cognitiveBeamWidth, cognitiveMaxHops)
	reading.NodeCount = len(reading.Branches)

	// Sideline when the engine cannot commit: an ambiguous read means no regime
	// is trustworthy enough to act on.
	reading.Sideline = reading.Ambiguous

	return reading
}

func cognitiveMeasurementStatus(measurement *datura.Artifact, reading CognitiveReading) string {
	confidence := datura.Peek[float64](measurement, "output", "confidence")
	strength := datura.Peek[float64](measurement, "output", "strength")

	if confidence <= 0 || strength <= 0 {
		return "standby"
	}

	if reading.Sideline || reading.Ambiguous {
		return "ambiguous"
	}

	if reading.ClassConfidence <= 0 {
		return "calibrating"
	}

	origin, err := measurement.Origin()

	if err == nil && origin == string(logic.SourceCausal) &&
		!datura.Peek[bool](measurement, "output", "counterfactualReady") {
		return "calibrating"
	}

	return "measured"
}

func cognitiveClasses(result dmt.ClassificationResult) []CognitiveClass {
	if len(result.Scores) == 0 {
		return nil
	}

	classes := make([]CognitiveClass, 0, len(result.Scores))

	for _, score := range result.Scores {
		classes = append(classes, CognitiveClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	return classes
}

func cognitiveBeams(beams []dmt.BeamPath) []CognitiveBeam {
	if len(beams) == 0 {
		return nil
	}

	out := make([]CognitiveBeam, 0, len(beams))

	for _, beam := range beams {
		out = append(out, CognitiveBeam{
			Sequence: string(beam.Sequence),
			Score:    beam.Score,
		})
	}

	return out
}

func cognitiveBranches(tree *dmt.Tree, width, maxHops int) []CognitiveBranch {
	if tree == nil || width <= 0 || maxHops <= 0 {
		return nil
	}

	branches := []CognitiveBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "•",
		Prefix:      "",
		Probability: 1,
	}}
	var predictions []dmt.LookaheadPrediction

	var grow func(parentID int, prefix []byte, depth int)
	grow = func(parentID int, prefix []byte, depth int) {
		if depth >= maxHops {
			return
		}

		if cap(predictions) < width*4 {
			predictions = make([]dmt.LookaheadPrediction, 0, width*4)
		}

		children := tree.PredictNextSensoryTokens(prefix, predictions[:0])

		sort.SliceStable(children, func(left, right int) bool {
			if children[left].Probability == children[right].Probability {
				return string(children[left].Token) < string(children[right].Token)
			}

			return children[left].Probability > children[right].Probability
		})

		if len(children) > width {
			children = children[:width]
		}

		copied := make([]dmt.LookaheadPrediction, len(children))

		for index, child := range children {
			copied[index] = dmt.LookaheadPrediction{
				Token:       append([]byte(nil), child.Token...),
				Probability: child.Probability,
			}
		}

		for _, child := range copied {
			childPrefix := appendCognitiveToken(nil, prefix, child.Token)
			state := tree.GetSensoryWeight(childPrefix)
			id := len(branches)
			branches = append(branches, CognitiveBranch{
				ID:          id,
				ParentID:    parentID,
				Token:       string(child.Token),
				Prefix:      string(childPrefix),
				Depth:       depth + 1,
				Probability: child.Probability,
				Count:       state.Count,
			})

			grow(id, childPrefix, depth+1)
		}
	}

	grow(0, nil, 0)

	return branches
}

func appendCognitiveToken(buffer []byte, prefix []byte, token []byte) []byte {
	if len(prefix) > 0 {
		buffer = append(buffer, prefix...)
		buffer = append(buffer, '_')
	}

	return append(buffer, token...)
}

// Fraction of the maximum possible surprisal above which a sequence reads
// ambiguous. 0.85 keeps the gate near the ceiling so only sequences the engine
// has learned almost no structure for flip ambiguous.
const cognitiveAmbiguityFraction = 0.85

/*
sensorySurprisal sums the per-token information content (bits) of a sequence
against the SENSORY weights TrainSensorySequence writes (the s/ namespace),
walking each underscore-delimited prefix. A prefix the engine has learned carries
its stored probability → -log2(p) bits; a prefix never seen carries 0 stored
probability and is treated as maximally surprising for one token (1 bit). The sum
is the sequence's total surprise — high for a novel regime, low for a familiar
one. (dmt.GetSurprisal reads the separate context-weight store, not the sensory
store this engine trains, so it is not the right source here.)
*/
func sensorySurprisal(tree *dmt.Tree, sequence []byte) float64 {
	total := 0.0
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		prefix := sequence[:index]
		weight := tree.GetSensoryWeight(prefix)

		if weight.Probability > 0 {
			total += -math.Log2(weight.Probability)
		} else {
			// Unseen prefix: one bit of surprise (a maximally novel binary step).
			total += 1
		}

		tokenStart = index + 1
	}

	return total
}

func bestBeamScore(beams []dmt.BeamPath) float64 {
	best := math.Inf(-1)

	for _, beam := range beams {
		if beam.Score > best {
			best = beam.Score
		}
	}

	if math.IsInf(best, -1) {
		return 0
	}

	// Beam scores are summed log-probabilities (≤ 0); map to a bounded (0,1]
	// confidence so the surface can size bars without re-scaling.
	return math.Exp(best)
}
