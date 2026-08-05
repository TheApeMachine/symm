package cognition

import (
	"bytes"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
Option configures the Cognition solver.
*/
type Option func(*Solver)

/*
WithMaxSequenceLength sets the maximum token depth before a sequence naturally terminates.
*/
func WithMaxSequenceLength(length int) Option {
	return func(s *Solver) {
		s.maxSeqLen = length
	}
}

/*
WithSurprisalLimit sets the information-theoretic threshold (in bits) that triggers
a sequence break when an unexpected category transition occurs.
*/
func WithSurprisalLimit(limitBits float64) Option {
	return func(s *Solver) {
		s.surprisalLimit = limitBits
	}
}

/*
Solver uses dmt.Tree to learn, score, and forecast market category transition sequences,
classify macro regimes via attractor basins, and predict future category paths using beam search.
*/
type Solver struct {
	recorder       *audit.Recorder
	tree           *dmt.Tree
	sequences      map[string][]string // Active category token buffer per symbol
	maxSeqLen      int
	surprisalLimit float64
	tickCounter    uint64
	ui             chan []byte

	// Reusable zero-allocation scratch buffers
	classScratch dmt.ClassificationScratch
	beamScratch  dmt.BeamSearchScratch
}

const categoryTokenSeparator = "\x1f"

/*
NewSolver returns a new cognition solver bound to a radix tree and audit recorder.
*/
func NewSolver(
	tree *dmt.Tree,
	ui chan []byte,
	recorder *audit.Recorder,
	opts ...Option,
) *Solver {
	solver := &Solver{
		recorder:       recorder,
		tree:           tree,
		sequences:      make(map[string][]string),
		maxSeqLen:      6,   // Max 6 category transitions per sequence window
		surprisalLimit: 3.5, // > 3.5 bits surprisal (P < 8.8%) indicates a regime break
		ui:             ui,
		beamScratch: dmt.BeamSearchScratch{
			CurrentBeams: make([]dmt.BeamPath, 0, 4),
			NextBeams:    make([]dmt.BeamPath, 0, 4),
			LookupBuffer: make([]dmt.LookaheadPrediction, 0, 8),
		},
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update ingests the active Thesis categories, evaluates category transition surprisal,
breaks/continues sequence paths, classifies market regimes, and runs lookahead beam search.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil || solver.tree == nil {
		return nil
	}

	solver.tickCounter++
	nowUnix := uint64(thesis.At.UnixNano())

	// 1. Process active categories per symbol
	for symbol, categories := range thesis.Categories {
		if len(categories) == 0 {
			continue
		}

		// Select the dominant category for this symbol on this tick
		dominantCategory := solver.selectDominantCategory(categories)
		if dominantCategory == types.CategoryTypeNone {
			continue
		}

		categoryToken := solver.encodeCategory(dominantCategory)
		activeTokens := solver.sequences[symbol]

		// 2. Evaluate if appending this category causes a Sequence Break
		broken, _ := solver.evalSequenceBreak(activeTokens, categoryToken)

		if broken && len(activeTokens) > 0 {
			// --- SEQUENCE BREAK DETECTED ---
			oldSequenceBytes := solver.sequenceBytes(activeTokens)

			// Commit completed sequence to episodic buffer for REM replay
			_, _ = solver.tree.CommitToEpisodicBuffer(nowUnix, oldSequenceBytes)

			// Run unsupervised learning to strengthen attractor basin weights
			_, _, _ = solver.tree.UnsupervisedLearn(oldSequenceBytes, &solver.classScratch)

			// Start fresh sequence buffer with new category
			activeTokens = []string{categoryToken}
		} else {
			// --- SEQUENCE CONTINUES ---
			activeTokens = append(activeTokens, categoryToken)
		}

		solver.sequences[symbol] = activeTokens
		activeSequenceBytes := solver.sequenceBytes(activeTokens)

		// 3. Train sensory sequence online
		solver.tree.TrainSensorySequence(activeSequenceBytes)

		// 4. Classify macro market regime / concept attractor basin
		classResult := solver.tree.Classify(activeSequenceBytes, &solver.classScratch)

		// 5. Lookahead Beam Search: Predict next 2–3 likely category hops
		const beamWidth = 3
		const maxHops = 2

		beamPaths := solver.tree.ExecuteBeamSearch(
			activeSequenceBytes,
			beamWidth,
			maxHops,
			&solver.beamScratch,
		)

		// 6. Measure Branch Ambiguity (Shannon Entropy)
		ambiguity := solver.tree.MeasureBranchAmbiguity(activeSequenceBytes)

		// 7. Format Lookahead Predictions for Thesis
		predictions := solver.formatLookaheadPredictions(
			beamPaths, activeSequenceBytes,
		)

		classes := make([]types.CognitionClass, 0, len(classResult.Scores))

		for _, score := range classResult.Scores {
			classes = append(classes, types.CognitionClass{
				Name:        string(score.ClassName),
				Probability: score.Value,
			})
		}

		beams := make([]types.CognitionBeam, 0, len(beamPaths))

		for _, path := range beamPaths {
			beams = append(beams, types.CognitionBeam{
				Sequence: solver.decodeCategoryPath(path.Sequence),
				Score:    path.Score,
			})
		}

		branches := []types.CognitionBranch{{
			ID:          0,
			ParentID:    -1,
			Token:       "•",
			Probability: 1,
		}}
		prefixTokens := make([]string, 0, len(activeTokens))

		for index, token := range activeTokens {
			prefixTokens = append(prefixTokens, token)
			prefixBytes := solver.sequenceBytes(prefixTokens)
			prefix := solver.decodeCategoryPath(prefixBytes)
			weight := solver.tree.GetSensoryWeight(prefixBytes)
			branches = append(branches, types.CognitionBranch{
				ID:          index + 1,
				ParentID:    index,
				Token:       solver.decodeCategoryToken(token),
				Prefix:      prefix,
				Depth:       index + 1,
				Probability: weight.Probability,
				Count:       weight.Count,
			})
		}

		contrast := 0.0
		contrastEvidence := 0.0

		if len(classResult.Scores) > 1 {
			contrast = classResult.Scores[0].Value - classResult.Scores[1].Value
			evidence := solver.tree.ComputeBasinContrastiveEvidence(
				classResult.Scores[0].ClassName,
				classResult.Scores[1].ClassName,
				activeSequenceBytes,
			)
			contrastEvidence = evidence.Divergence
		}

		lookaheadScore := 0.0

		if len(beamPaths) > 0 {
			lookaheadScore = beamPaths[0].Score
		}

		winner := string(classResult.Winner)

		cognition := types.Cognition{
			Source:           "cognition",
			Symbol:           symbol,
			At:               thesis.At,
			Sequence:         solver.decodeCategoryPath(activeSequenceBytes),
			RegimePrefix:     winner,
			Winner:           winner,
			WinnerClass:      winner,
			Ready:            winner != "",
			Confidence:       classResult.Highest,
			ClassConfidence:  classResult.Highest,
			Contrast:         contrast,
			ContrastEvidence: contrastEvidence,
			EntropyBits:      ambiguity.EntropyBits,
			EntropyThreshold: ambiguity.Threshold,
			Ambiguous:        ambiguity.Ambiguous,
			Cohort:           solver.tree.GetSensoryWeight(activeSequenceBytes).Count,
			LookaheadScore:   lookaheadScore,
			LookaheadPaths:   len(beamPaths),
			BeamWidth:        beamWidth,
			MaxHops:          maxHops,
			NodeCount:        len(branches),
			Predictions:      predictions,
			Branches:         branches,
			Beams:            beams,
			Classes:          classes,
		}

		thesis.Cognition.Store(symbol, cognition)

		// 9. Audit Recording
		if solver.recorder != nil {
			errnie.Error(audit.Record(
				solver.recorder, "predictive", cognition,
			))
		}
	}

	thesis.Readiness.Cognition = true

	solver.publish(thesis)

	// 10. Periodic REM Sleep Consolidation (every 128 ticks)
	if solver.tickCounter%128 == 0 && nowUnix > 60e9 {
		startWindow := nowUnix - 60e9 // 1 minute window
		solver.tree.ExecuteREMSleepConsolidation(startWindow, nowUnix)
	}

	return nil
}

/*
evalSequenceBreak tests whether appending categoryToken causes a sequence break.
Returns true if surprisal exceeds limit or sequence length exceeds max.
*/
func (solver *Solver) evalSequenceBreak(
	activeTokens []string, nextToken string,
) (broken bool, surprisal float64) {
	if len(activeTokens) == 0 {
		return false, 0.0
	}

	if len(activeTokens) >= solver.maxSeqLen {
		return true, 0.0 // Sequence too long; force fresh break
	}

	// Test candidate path surprisal
	candidateTokens := append(append([]string(nil), activeTokens...), nextToken)
	candidateBytes := solver.sequenceBytes(candidateTokens)

	surprisalItems := solver.tree.GetSurprisal(candidateBytes)

	if len(surprisalItems) == 0 {
		return false, 0.0
	}

	// Check surprisal of the newly appended token
	lastItem := surprisalItems[len(surprisalItems)-1]
	surprisal = lastItem.Surprisal

	if surprisal >= solver.surprisalLimit {
		return true, surprisal // Unexpected transition -> Break Sequence!
	}

	return false, surprisal
}

/*
selectDominantCategory picks the category with the highest confidence/strength.
*/
func (solver *Solver) selectDominantCategory(
	categories []types.Category,
) types.CategoryType {
	if len(categories) == 0 {
		return types.CategoryTypeNone
	}

	best := categories[0]

	for _, cat := range categories[1:] {
		if cat.Confidence*cat.Strength > best.Confidence*best.Strength {
			best = cat
		}
	}

	return best.Type
}

/*
encodeCategory keeps one category as one DMT token.

DMT uses underscore as its sequence boundary, while category names themselves
contain underscores. Passing raw category names therefore trained
"vertical_ignition" as two unrelated states. Unit Separator is not used by any
category name and survives the byte-oriented tree unchanged.
*/
func (solver *Solver) encodeCategory(category types.CategoryType) string {
	return strings.ReplaceAll(string(category), "_", categoryTokenSeparator)
}

func (solver *Solver) decodeCategoryToken(token string) string {
	return strings.ReplaceAll(token, categoryTokenSeparator, "_")
}

func (solver *Solver) sequenceBytes(tokens []string) []byte {
	return []byte(strings.Join(tokens, "_"))
}

func (solver *Solver) decodeCategoryPath(path []byte) string {
	tokens := strings.Split(string(path), "_")

	for index := range tokens {
		tokens[index] = solver.decodeCategoryToken(tokens[index])
	}

	return strings.Join(tokens, " → ")
}

/*
formatLookaheadPredictions extracts future token paths from beam search results.
*/
func (solver *Solver) formatLookaheadPredictions(
	paths []dmt.BeamPath, currentPrefix []byte,
) map[string]float64 {
	if len(paths) == 0 {
		return nil
	}

	predictions := make(map[string]float64, len(paths))

	for _, path := range paths {
		// Strip active prefix to show only future projected hops
		futureSuffix := bytes.TrimPrefix(path.Sequence, currentPrefix)
		futureSuffix = bytes.TrimPrefix(futureSuffix, []byte("_"))

		if len(futureSuffix) == 0 {
			continue
		}

		if !utf8.Valid(futureSuffix) {
			continue
		}

		predictions[solver.decodeCategoryPath(futureSuffix)] = math.Exp(path.Score)
	}

	return predictions
}

/*
publish emits one cognition wire frame per symbol observed on this tick.
*/
func (solver *Solver) publish(thesis *types.Thesis) {
	if solver.ui == nil || thesis == nil {
		return
	}

	/*
		Rows are keyed by symbol rather than listed, because the display reads
		one symbol at a time and a position in a list says nothing about which
		symbol it describes once the set of symbols changes between ticks.
	*/
	rows := datura.NewMap()

	thesis.Cognition.Range(func(key, value any) bool {
		symbol, ok := key.(string)
		if !ok || value == nil {
			return true
		}

		cognition, ok := value.(types.Cognition)
		if !ok {
			return true
		}

		cognition.At = thesis.At
		rows[symbol] = cognition
		return true
	})

	if len(rows) > 0 {
		select {
		case solver.ui <- datura.NewMap("cognition", rows).MarshalAndFree():
		default:
		}
	}
}

/*
Reset clears active sequence buffers for all symbols.
*/
func (solver *Solver) Reset() {
	solver.sequences = make(map[string][]string)
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.sequences = nil
	return nil
}
