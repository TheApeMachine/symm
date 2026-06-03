package scan

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/guard"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/progress"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

type actionGroup uint8

const (
	actionGroupEntry actionGroup = iota
	actionGroupExit
)

/*
ScanSearch scores a bounded beam of candidate branch trees.
*/
type ScanSearch struct {
	ctx                   context.Context
	pool                  *qpool.Q
	profile               *profile.Profile
	rows                  []perspectives.Measurement
	tape                  replay.ReplayTape
	coOccurrence          *cooccurrence.CoOccurrenceIndex
	options               types.ScanOptions
	guard                 *guard.OverfitGuard
	bestScore             float64
	bestClosedTrades      int
	bestBranch            perspectives.BranchList
	OnBest                func(types.BestTree)
	OnCandidate           func(types.CandidateScore)
	candidates            int
	phaseCandidates       int
	phaseCandidateLimit   int
	topK                  int
	topScores             []types.CandidateScore
	beamScores            []types.CandidateScore
	pairAffinity          *PairAffinityIndex
	progress              *progress.SearchProgress
	haltPhaseOnStagnation bool
	budget                types.SearchBudget
	mu                    sync.Mutex
}

func NewScanSearch(
	ctx context.Context,
	profile *profile.Profile,
	rows []perspectives.Measurement,
	options types.ScanOptions,
) *ScanSearch {
	return NewScanSearchWithTape(ctx, profile, rows, replay.ReplayTape{}, options)
}

type scanCandidate struct {
	branches perspectives.BranchList
	group    actionGroup
	index    int
}

type scanResult struct {
	candidate     scanCandidate
	branches      perspectives.BranchList
	rawScore      float64
	adjustedScore float64
	closedTrades  int
}

/*
NewScanSearchWithTape reuses a precompiled replay tape when scoring candidates.
*/
func NewScanSearchWithTape(
	ctx context.Context,
	profile *profile.Profile,
	rows []perspectives.Measurement,
	tape replay.ReplayTape,
	options types.ScanOptions,
) *ScanSearch {
	options = normalizeScanOptions(options)
	profile.PrepareCache()

	if tape.Len() == 0 {
		tape = replay.PrecompileTape(rows)
	}

	searchBudget := options.Budget

	if searchBudget.IsZero() {
		searchBudget = budget.DeriveSearchBudget(profile, tape, options.Workers)
	}

	options = budget.ApplyBudgetToScanOptions(options, searchBudget)
	coOccurrenceIndex := cooccurrence.NewCoOccurrenceIndex(tape, searchBudget.MinChainSupport)

	return &ScanSearch{
		ctx:          ctx,
		pool:         options.Pool,
		profile:      profile,
		rows:         rows,
		tape:         tape,
		coOccurrence: coOccurrenceIndex,
		options:      options,
		guard:        guard.NewOverfitGuard(ctx, options.Guard, tape, profile),
		pairAffinity: NewPairAffinityIndex(),
		budget:       searchBudget,
	}
}

/*
Run scores primitive branches, sibling pairs, and beam-expanded reasoning chains.
Each deepening pass expands only the top beam survivors from the previous depth.
*/
func (search *ScanSearch) Run() (perspectives.BranchList, types.ScanStats) {
	search.topK = 0
	search.topScores = nil

	return search.best(), search.run()
}

/*
RunTopK returns the highest-scoring guard-valid shallow trees for MCTS seeding.
*/
func (search *ScanSearch) RunTopK(limit int) ([]types.CandidateScore, types.ScanStats) {
	search.topK = limit
	search.topScores = make([]types.CandidateScore, 0, limit)
	stats := search.run()

	return search.topScoresClone(), stats
}

func (search *ScanSearch) run() types.ScanStats {
	search.bestScore = math.Inf(-1)
	search.bestClosedTrades = -1

	searchProgress := progress.NewSearchProgress()
	search.progress = searchProgress

	actionBranches := search.actionBranches()

	search.runAdaptivePhase("decision seeds", func(send func(scanCandidate) bool) {
		search.emitDecisionSeeds(send)
	})

	survivors := search.BeamScoresClone()
	targetDepth := progress.SeedSearchTargetDepth(survivors)

	for search.hasCandidateBudget() && targetDepth <= search.options.MaxReasoningSteps {
		if len(survivors) == 0 {
			break
		}

		if searchProgress.Stagnant(search.options.BeamWidth) {
			search.logSearchStalled(targetDepth)
		}

		previous := survivors
		branchers := search.rankedEntryBranchers()

		search.runAdaptivePhase(
			fmt.Sprintf("deepen gates (depth %d)", targetDepth+1),
			func(send func(scanCandidate) bool) {
				search.emitDeepeningExpansions(send, survivors, branchers)
			},
		)
		search.mergeDeepeningSurvivors(previous)
		survivors = search.BeamScoresClone()

		if searchProgress.Stagnant(search.options.BeamWidth) {
			previous = survivors
			search.runAdaptivePhase(
				fmt.Sprintf("widen exits (depth %d)", targetDepth),
				func(send func(scanCandidate) bool) {
					search.emitWidenExpansions(send, survivors, actionBranches)
				},
			)
			search.mergeDeepeningSurvivors(append(previous, survivors...))
			survivors = search.BeamScoresClone()
		}

		if !searchProgress.Stagnant(search.options.BeamWidth) {
			continue
		}

		nextDepth := targetDepth + 1

		log.TuneLog("no improvement at depth %d, advancing to %d", targetDepth, nextDepth)
		targetDepth = nextDepth
		searchProgress.ResetStagnation()
	}

	return types.ScanStats{
		Candidates: search.candidates,
		Workers:    search.options.Workers,
	}
}

func (search *ScanSearch) logSearchStalled(targetDepth int) {
	bestScore := search.progress.BestScore()

	if math.IsInf(bestScore, 0) {
		log.TuneLog(
			"reward stalled after %d candidates without improvement (depth %d), nesting entry gates",
			search.progress.SinceImprovement(),
			targetDepth,
		)

		return
	}

	log.TuneLog(
		"reward stalled at %.6f after %d candidates without improvement (depth %d), nesting entry gates",
		bestScore,
		search.progress.SinceImprovement(),
		targetDepth,
	)
}

func (search *ScanSearch) hasCandidateBudget() bool {
	if search.options.CandidateLimit <= 0 {
		return true
	}

	return search.candidates < search.options.CandidateLimit
}

func (search *ScanSearch) runAdaptivePhase(
	phase string,
	generate func(send func(scanCandidate) bool),
) {
	search.phaseCandidates = 0
	search.phaseCandidateLimit = 0
	search.haltPhaseOnStagnation = true

	search.runBeamPhase(phase, generate)

	search.haltPhaseOnStagnation = false
}

func normalizeScanOptions(options types.ScanOptions) types.ScanOptions {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	return options
}
