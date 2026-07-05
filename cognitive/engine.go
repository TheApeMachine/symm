package cognitive

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/cognitive/dmt"
	"github.com/theapemachine/symm/market"
)

const ambiguityFraction = 0.85

func readObservations(
	tree *dmt.Tree,
	observations []observation,
	cache map[string]market.CognitiveReading,
) map[string]market.CognitiveReading {
	readings := make(map[string]market.CognitiveReading, len(observations))
	classifyScratch := &dmt.ClassificationScratch{}
	beamScratch := &dmt.BeamSearchScratch{}

	for _, observation := range observations {
		reading := readObservation(
			tree,
			observation,
			classifyScratch,
			beamScratch,
		)
		readings[observation.symbol] = reading
		if cache != nil {
			cache[observation.symbol] = reading
		}
	}

	return readings
}

func readObservation(
	tree *dmt.Tree,
	observation observation,
	classifyScratch *dmt.ClassificationScratch,
	beamScratch *dmt.BeamSearchScratch,
) market.CognitiveReading {
	return readingFromEngine(
		tree,
		observation.symbol,
		observation.tokens,
		observation.stamp,
		classifyScratch,
		beamScratch,
	)
}

func readingFromEngine(
	tree *dmt.Tree,
	symbol string,
	tokens []token,
	stamp int64,
	classifyScratch *dmt.ClassificationScratch,
	beamScratch *dmt.BeamSearchScratch,
) market.CognitiveReading {
	sort.SliceStable(tokens, func(first, second int) bool {
		if tokens[first].confidence != tokens[second].confidence {
			return tokens[first].confidence > tokens[second].confidence
		}

		if tokens[first].origin != tokens[second].origin {
			return tokens[first].origin < tokens[second].origin
		}

		return tokens[first].category < tokens[second].category
	})

	sequence := sequence(tokens)

	reading := market.CognitiveReading{
		Scope:        symbol,
		Sequence:     sequence,
		RegimeCohort: len(tokens),
		UpdatedAt:    stamp,
		BeamWidth:    beamWidth,
		MaxHops:      maxHops,
	}

	if sequence == "" {
		return reading
	}

	sequenceBytes := []byte(sequence)
	tokenCount := len(tokens)
	reading.EntropyBits = surprisal(tree, sequenceBytes)
	reading.Surprisal = reading.EntropyBits

	tokenCeiling := math.Log2(math.Max(2, float64(tokenCount)))
	reading.EntropyThreshold = float64(tokenCount) * tokenCeiling * ambiguityFraction
	if reading.EntropyThreshold > 0 {
		reading.Surprise = reading.EntropyBits / reading.EntropyThreshold
	}
	reading.Ambiguous = reading.EntropyThreshold > 0 &&
		reading.EntropyBits >= reading.EntropyThreshold

	result := tree.Classify(sequenceBytes, classifyScratch)
	reading.ClassConfidence = result.Highest
	reading.Classes = classes(result)

	if len(result.Scores) > 1 {
		reading.ContrastEvidence = math.Max(0, result.Scores[0].Value-result.Scores[1].Value)
	} else {
		reading.ContrastEvidence = result.Highest
	}

	beams := tree.ExecuteBeamSearch(nil, beamWidth, maxHops, beamScratch)
	reading.LookaheadPaths = len(beams)
	reading.LookaheadScore = bestScore(beams)
	reading.Beams = beamsFor(beams)

	winnerClass := tokens[0].category

	if result.Highest > 0 && len(result.Winner) > 0 {
		winnerClass = string(result.Winner)
	}

	reading.WinnerClass = winnerClass
	reading.RegimePrefix = winnerClass

	tree.TrainSensorySequence(sequenceBytes)

	classBytes := []byte(winnerClass)
	priorBasin := tree.GetAttractorBasin(classBytes, sequenceBytes)
	tree.InsertAttractorBasin(
		classBytes,
		sequenceBytes,
		dmt.CognitiveState{Count: priorBasin.Count + 1, Probability: 1},
	)

	reading.Branches = branches(tree, beamWidth, maxHops)
	reading.NodeCount = len(reading.Branches)
	reading.Sideline = reading.Ambiguous

	return reading
}

func classes(result dmt.ClassificationResult) []market.CognitiveClass {
	if len(result.Scores) == 0 {
		return nil
	}

	classes := make([]market.CognitiveClass, 0, len(result.Scores))

	for _, score := range result.Scores {
		classes = append(classes, market.CognitiveClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	return classes
}

func beamsFor(beams []dmt.BeamPath) []market.CognitiveBeam {
	if len(beams) == 0 {
		return nil
	}

	out := make([]market.CognitiveBeam, 0, len(beams))

	for _, beam := range beams {
		out = append(out, market.CognitiveBeam{
			Sequence: string(beam.Sequence),
			Score:    beam.Score,
		})
	}

	return out
}

func branches(tree *dmt.Tree, width, maxHops int) []market.CognitiveBranch {
	if tree == nil || width <= 0 || maxHops <= 0 {
		return nil
	}

	branches := []market.CognitiveBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "*",
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
			childPrefix := appendToken(nil, prefix, child.Token)
			state := tree.GetSensoryWeight(childPrefix)
			id := len(branches)
			branches = append(branches, market.CognitiveBranch{
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

func surprisal(tree *dmt.Tree, sequence []byte) float64 {
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
			total += 1
		}

		tokenStart = index + 1
	}

	return total
}

func bestScore(beams []dmt.BeamPath) float64 {
	best := math.Inf(-1)

	for _, beam := range beams {
		if beam.Score > best {
			best = beam.Score
		}
	}

	if math.IsInf(best, -1) {
		return 0
	}

	return math.Exp(best)
}
