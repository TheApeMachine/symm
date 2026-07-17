package logic

import (
	"strings"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

/*
cognitionVisualization materializes the sensory prefix tree, beam paths, and basin
posteriors that Cortex renders from one DMT reading.
*/
func (analyzer *Analyzer) cognitionVisualization(
	sequence []byte,
	parent []byte,
	parts []string,
	classification dmt.ClassificationResult,
	predictions []dmt.LookaheadPrediction,
) (
	branches []types.CognitionBranch,
	beams []types.CognitionBeam,
	classes []types.CognitionClass,
	beamWidth int,
	maxHops int,
	nodeCount int,
	lookaheadScore float64,
	lookaheadPaths int,
) {
	if analyzer == nil || analyzer.tree == nil || len(sequence) == 0 {
		return nil, nil, nil, 0, 0, 0, 0, 0
	}

	classes = cognitionClasses(classification)
	branches, nodeCount = analyzer.cognitionBranches(parts)
	beamWidth = cognitionBeamWidth(predictions)
	// Sequence depth is what Cortex labels as maxHops. Beam search must not
	// reuse that depth: measurement cardinality is not a lookahead horizon.
	maxHops = cognitionSequenceDepth(parts)
	beams, lookaheadScore, lookaheadPaths = analyzer.cognitionBeams(
		parent, beamWidth, 1, predictions,
	)

	return branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths
}

func cognitionClasses(
	classification dmt.ClassificationResult,
) []types.CognitionClass {
	classes := make([]types.CognitionClass, 0, len(classification.Scores))

	for _, score := range classification.Scores {
		classes = append(classes, types.CognitionClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	return classes
}

func cognitionBeamWidth(predictions []dmt.LookaheadPrediction) int {
	if len(predictions) == 0 {
		return 1
	}

	return len(predictions)
}

/*
cognitionSequenceDepth reports how deep the current sensory prefix is so the
UI can scale the tree view. It is not a beam-search budget.
*/
func cognitionSequenceDepth(parts []string) int {
	if len(parts) == 0 {
		return 1
	}

	return len(parts)
}

func (analyzer *Analyzer) cognitionBranches(
	parts []string,
) ([]types.CognitionBranch, int) {
	rootWeight := analyzer.tree.GetSensoryWeight(nil)
	branches := []types.CognitionBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "\u2022",
		Prefix:      "",
		Depth:       0,
		Probability: 1,
		Count:       rootWeight.Count,
	}}
	prefixIndex := map[string]int{"": 0}
	nextID := 1

	for depth := 0; depth < len(parts); depth++ {
		parentPrefix := strings.Join(parts[:depth], "_")
		parentID, parentReady := prefixIndex[parentPrefix]

		if !parentReady {
			continue
		}

		predictionBuffer := make([]dmt.LookaheadPrediction, 0, len(parts))
		predictions := analyzer.tree.PredictNextSensoryTokens(
			[]byte(parentPrefix),
			predictionBuffer,
		)

		for _, prediction := range predictions {
			token := string(prediction.Token)
			childPrefix := token

			if parentPrefix != "" {
				childPrefix = parentPrefix + "_" + token
			}

			if _, exists := prefixIndex[childPrefix]; exists {
				continue
			}

			weight := analyzer.tree.GetSensoryWeight([]byte(childPrefix))
			branches = append(branches, types.CognitionBranch{
				ID:          nextID,
				ParentID:    parentID,
				Token:       token,
				Prefix:      childPrefix,
				Depth:       depth + 1,
				Probability: prediction.Probability,
				Count:       weight.Count,
			})
			prefixIndex[childPrefix] = nextID
			nextID++
		}
	}

	return branches, len(branches)
}

func (analyzer *Analyzer) cognitionBeams(
	parent []byte,
	beamWidth int,
	maxHops int,
	predictions []dmt.LookaheadPrediction,
) ([]types.CognitionBeam, float64, int) {
	lookaheadScore := 0.0

	for _, prediction := range predictions {
		lookaheadScore += prediction.Probability
	}

	scratch := &dmt.BeamSearchScratch{
		LookupBuffer: make([]dmt.LookaheadPrediction, 0, beamWidth),
	}
	beamPaths := analyzer.tree.ExecuteBeamSearch(parent, beamWidth, maxHops, scratch)
	beams := make([]types.CognitionBeam, 0, len(beamPaths))

	for _, beamPath := range beamPaths {
		beams = append(beams, types.CognitionBeam{
			Sequence: string(beamPath.Sequence),
			Score:    beamPath.Score,
		})
	}

	return beams, lookaheadScore, len(beamPaths)
}
