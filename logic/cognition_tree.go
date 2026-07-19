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
	symbolPrefix := cognitionSymbolPrefix(parts)
	tip := analyzer.predictChildren(string(symbolPrefix), cognitionTreeDepth()*cognitionTreeDepth())

	if len(tip) == 0 {
		tip = predictions
	}

	beamWidth = analyzer.treeExpandWidth(cognitionBeamWidth(tip))
	maxHops = cognitionTreeDepth()
	branches, nodeCount = analyzer.cognitionBranches(beamWidth)
	// Beam search is scoped under the symbol hop so each coin explores its own
	// namespace. Searching the empty root made every symbol share one global MAP.
	beams, lookaheadScore, lookaheadPaths = analyzer.cognitionBeams(
		symbolPrefix, beamWidth, maxHops, tip,
	)

	_ = parent

	return branches, beams, classes, beamWidth, maxHops, nodeCount, lookaheadScore, lookaheadPaths
}

/*
cognitionSymbolPrefix returns the leading symbol-* token so beam search stays
inside one coin's radix namespace.
*/
func cognitionSymbolPrefix(parts []string) []byte {
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "symbol-") {
		return nil
	}

	return []byte(parts[0])
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
cognitionTreeDepth is the Cortex radix expand / beam-search horizon. It matches
the mockup's three-level sensory prefix tree so the canvas shows real forks
instead of a single sealed-bag spine.
ponytail: fixed depth-3 visualization horizon; upgrade path is adaptive depth
from live branching entropy so dense namespaces can deepen without a constant.
*/
func cognitionTreeDepth() int {
	return 3
}

/*
cognitionNodeBudget is the size of a complete fanout-ary tree of the given depth,
including the root. It keeps busy ticks from exploding while still allowing the
learned radix forks to render.
*/
func cognitionNodeBudget(fanout int, depth int) int {
	if fanout < 1 {
		fanout = 1
	}

	total := 1
	level := 1

	for hop := 0; hop < depth; hop++ {
		level *= fanout
		total += level
	}

	return total
}

type branchFrontier struct {
	prefix string
	id     int
	depth  int
}

/*
cognitionBranches expands the learned sensory radix tree from the root using
PredictNextSensoryTokens at every node. Walking the sorted observation bag as a
forced spine produced one amber path with stub side-segments; this grows real
forks like the SYMM Terminal mockup.
*/
func (analyzer *Analyzer) cognitionBranches(
	tipWidth int,
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
	depthLimit := cognitionTreeDepth()
	fanout := tipWidth

	if fanout < 1 {
		fanout = 1
	}

	budget := cognitionNodeBudget(fanout, depthLimit)
	frontier := []branchFrontier{{prefix: "", id: 0, depth: 0}}

	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]

		if current.depth >= depthLimit {
			continue
		}

		remaining := budget - len(branches)

		if remaining < 1 {
			break
		}

		limit := fanout

		if limit > remaining {
			limit = remaining
		}

		predictions := analyzer.predictChildren(current.prefix, limit)

		for _, prediction := range predictions {
			token := string(prediction.Token)
			childPrefix := token

			if current.prefix != "" {
				childPrefix = current.prefix + "_" + token
			}

			if _, exists := prefixIndex[childPrefix]; exists {
				continue
			}

			weight := analyzer.tree.GetSensoryWeight([]byte(childPrefix))
			branches = append(branches, types.CognitionBranch{
				ID:          nextID,
				ParentID:    current.id,
				Token:       token,
				Prefix:      childPrefix,
				Depth:       current.depth + 1,
				Probability: prediction.Probability,
				Count:       weight.Count,
			})
			prefixIndex[childPrefix] = nextID
			frontier = append(frontier, branchFrontier{
				prefix: childPrefix,
				id:     nextID,
				depth:  current.depth + 1,
			})
			nextID++

			if len(branches) >= budget {
				return branches, len(branches)
			}
		}
	}

	return branches, len(branches)
}

/*
treeExpandWidth chooses per-level fan-out for the Cortex radix view. Beam search
keeps the tip width; when that tip collapses to a single continuation, the view
lifts to the empty-prefix fork so sibling regimes still render.
ponytail: lift bound is depth² so a dense symbol namespace cannot allocate an
N³ node explosion; upgrade path is entropy-scaled fan-out from MeasureBranchAmbiguity.
*/
func (analyzer *Analyzer) treeExpandWidth(tipWidth int) int {
	if tipWidth < 1 {
		tipWidth = 1
	}

	if tipWidth > 1 {
		return tipWidth
	}

	bound := cognitionTreeDepth() * cognitionTreeDepth()
	rootChildren := analyzer.predictChildren("", bound)

	if len(rootChildren) > tipWidth {
		return len(rootChildren)
	}

	return tipWidth
}

func (analyzer *Analyzer) predictChildren(
	prefix string,
	limit int,
) []dmt.LookaheadPrediction {
	if limit < 1 {
		return nil
	}

	buffer := make([]dmt.LookaheadPrediction, 0, limit)

	return analyzer.tree.PredictNextSensoryTokens([]byte(prefix), buffer)
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
