package cognition

import (
	"bytes"
	"math"
	"strings"
	"sync"

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
	mu             sync.RWMutex
	sequences      map[string][]string // Active category token buffer per symbol
	maxSeqLen      int
	surprisalLimit float64
	tickCounter    uint64

	// Reusable zero-allocation scratch buffers
	classScratch dmt.ClassificationScratch
	beamScratch  dmt.BeamSearchScratch
}

/*
NewSolver returns a new cognition solver bound to a radix tree and audit recorder.
*/
func NewSolver(tree *dmt.Tree, recorder *audit.Recorder, opts ...Option) *Solver {
	if tree == nil {
		var err error
		tree, err = dmt.NewTree("")
		if err != nil {
			errnie.Error(errnie.Err(errnie.UnprocessableContent, "cognition: tree init failed", err))
		}
	}

	solver := &Solver{
		recorder:       recorder,
		tree:           tree,
		sequences:      make(map[string][]string),
		maxSeqLen:      6,   // Max 6 category transitions per sequence window
		surprisalLimit: 3.5, // > 3.5 bits surprisal (P < 8.8%) indicates a regime break
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

	solver.mu.Lock()
	defer solver.mu.Unlock()

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

		categoryToken := string(dominantCategory)
		activeTokens := solver.sequences[symbol]

		// 2. Evaluate if appending this category causes a Sequence Break
		broken, currentSurprisal := solver.evalSequenceBreak(activeTokens, categoryToken)

		if broken && len(activeTokens) > 0 {
			// --- SEQUENCE BREAK DETECTED ---
			oldSequenceBytes := []byte(strings.Join(activeTokens, "_"))

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
		activeSequenceBytes := []byte(strings.Join(activeTokens, "_"))

		// 3. Train sensory sequence online
		solver.tree.TrainSensorySequence(activeSequenceBytes)

		// 4. Classify macro market regime / concept attractor basin
		classResult := solver.tree.Classify(activeSequenceBytes, &solver.classScratch)

		// 5. Lookahead Beam Search: Predict next 2–3 likely category hops
		beamPaths := solver.tree.ExecuteBeamSearch(
			activeSequenceBytes,
			3, // Beam width
			2, // Max lookahead hops
			&solver.beamScratch,
		)

		// 6. Measure Branch Ambiguity (Shannon Entropy)
		ambiguity := solver.tree.MeasureBranchAmbiguity(activeSequenceBytes)

		// 7. Format Lookahead Predictions for Thesis
		predictions := solver.formatLookaheadPredictions(beamPaths, activeSequenceBytes)

		// 8. Enrich Thesis.Cognition
		cognitionOutcome := map[string]any{
			"activeSequence": string(activeSequenceBytes),
			"winnerRegime":   string(classResult.Winner),
			"confidence":     classResult.Highest,
			"surprisal":      currentSurprisal,
			"isBreak":        broken,
			"ambiguity":      ambiguity.Ambiguous,
			"entropyBits":    ambiguity.EntropyBits,
			"predictions":    predictions,
		}

		thesis.Cognition.Store(symbol, cognitionOutcome)

		// 9. Audit Recording
		if solver.recorder != nil {
			auditEntry := map[string]any{
				"symbol":         symbol,
				"activeSequence": string(activeSequenceBytes),
				"winnerRegime":   string(classResult.Winner),
				"confidence":     classResult.Highest,
				"surprisal":      currentSurprisal,
				"isBreak":        broken,
				"predictions":    predictions,
			}
			solver.recorder.Write(auditEntry)
		}
	}

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
func (solver *Solver) evalSequenceBreak(activeTokens []string, nextToken string) (broken bool, surprisal float64) {
	if len(activeTokens) == 0 {
		return false, 0.0
	}

	if len(activeTokens) >= solver.maxSeqLen {
		return true, 0.0 // Sequence too long; force fresh break
	}

	// Test candidate path surprisal
	candidateTokens := append(append([]string(nil), activeTokens...), nextToken)
	candidateBytes := []byte(strings.Join(candidateTokens, "_"))

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
func (solver *Solver) selectDominantCategory(categories []types.Category) types.CategoryType {
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
formatLookaheadPredictions extracts future token paths from beam search results.
*/
func (solver *Solver) formatLookaheadPredictions(paths []dmt.BeamPath, currentPrefix []byte) []map[string]any {
	if len(paths) == 0 {
		return nil
	}

	predictions := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		// Strip active prefix to show only future projected hops
		futureSuffix := bytes.TrimPrefix(path.Sequence, currentPrefix)
		futureSuffix = bytes.TrimPrefix(futureSuffix, []byte("_"))

		if len(futureSuffix) == 0 {
			continue
		}

		predictions = append(predictions, map[string]any{
			"predictedPath": string(futureSuffix),
			"score":         path.Score,
			"probability":   math.Exp(path.Score),
		})
	}

	return predictions
}

/*
Reset clears active sequence buffers for all symbols.
*/
func (solver *Solver) Reset() {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	solver.sequences = make(map[string][]string)
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	solver.sequences = nil
	return nil
}
