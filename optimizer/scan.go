package optimizer

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

type actionGroup uint8

const (
	actionGroupEntry actionGroup = iota
	actionGroupExit
)

/*
ScanOptions bounds the offline parallel branch scan.
*/
type ScanOptions struct {
	Workers           int
	MaxThresholds     int
	BeamWidth         int
	CandidateLimit    int
	MaxReasoningSteps int
	Guard             GuardOptions
	Budget            SearchBudget
	Pool              *qpool.Q
}

/*
ScanStats reports how much of the bounded space was scored.
*/
type ScanStats struct {
	Candidates int
	Workers    int
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
ScanSearch scores a bounded beam of candidate branch trees.
*/
type ScanSearch struct {
	ctx                   context.Context
	pool                  *qpool.Q
	profile               *Profile
	rows                  []perspectives.Measurement
	tape                  ReplayTape
	coOccurrence          *CoOccurrenceIndex
	options               ScanOptions
	guard                 *OverfitGuard
	bestScore             float64
	bestClosedTrades      int
	bestBranch            perspectives.BranchList
	onBest                func(BestTree)
	onCandidate           func(CandidateScore)
	candidates            int
	phaseCandidates       int
	phaseCandidateLimit   int
	topK                  int
	topScores             []CandidateScore
	beamScores            []CandidateScore
	pairAffinity          *PairAffinityIndex
	progress              *SearchProgress
	haltPhaseOnStagnation bool
	budget                SearchBudget
	mu                    sync.Mutex
}

func NewScanSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	options ScanOptions,
) *ScanSearch {
	return NewScanSearchWithTape(ctx, profile, rows, ReplayTape{}, options)
}

/*
NewScanSearchWithTape reuses a precompiled replay tape when scoring candidates.
*/
func NewScanSearchWithTape(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	tape ReplayTape,
	options ScanOptions,
) *ScanSearch {
	options = normalizeScanOptions(options)
	profile.PrepareCache()

	if tape.Len() == 0 {
		tape = PrecompileTape(rows)
	}

	budget := options.Budget

	if budget.IsZero() {
		budget = DeriveSearchBudget(profile, tape, options.Workers)
	}

	options = applyBudgetToScanOptions(options, budget)
	coOccurrence := NewCoOccurrenceIndex(tape, budget.MinChainSupport)

	return &ScanSearch{
		ctx:          ctx,
		pool:         options.Pool,
		profile:      profile,
		rows:         rows,
		tape:         tape,
		coOccurrence: coOccurrence,
		options:      options,
		guard:        NewOverfitGuard(ctx, options.Guard, tape, profile),
		pairAffinity: NewPairAffinityIndex(),
		budget:       budget,
	}
}

/*
Run scores primitive branches, sibling pairs, and beam-expanded reasoning chains.
Each deepening pass expands only the top beam survivors from the previous depth.
*/
func (search *ScanSearch) Run() (perspectives.BranchList, ScanStats) {
	search.topK = 0
	search.topScores = nil

	return search.best(), search.run()
}

/*
RunTopK returns the highest-scoring guard-valid shallow trees for MCTS seeding.
*/
func (search *ScanSearch) RunTopK(limit int) ([]CandidateScore, ScanStats) {
	search.topK = limit
	search.topScores = make([]CandidateScore, 0, limit)
	stats := search.run()

	return search.topScoresClone(), stats
}

func (search *ScanSearch) run() ScanStats {
	search.bestScore = math.Inf(-1)
	search.bestClosedTrades = -1

	progress := NewSearchProgress()
	search.progress = progress

	actionBranches := search.actionBranches()

	search.runAdaptivePhase("decision seeds", func(send func(scanCandidate) bool) {
		search.emitDecisionSeeds(send)
	})

	survivors := search.beamScoresClone()
	targetDepth := seedSearchTargetDepth(survivors)

	for search.hasCandidateBudget() && targetDepth <= search.options.MaxReasoningSteps {
		if len(survivors) == 0 {
			break
		}

		if progress.Stagnant(search.options.BeamWidth) {
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
		survivors = search.beamScoresClone()

		if progress.Stagnant(search.options.BeamWidth) {
			previous = survivors
			search.runAdaptivePhase(
				fmt.Sprintf("widen exits (depth %d)", targetDepth),
				func(send func(scanCandidate) bool) {
					search.emitWidenExpansions(send, survivors, actionBranches)
				},
			)
			search.mergeDeepeningSurvivors(append(previous, survivors...))
			survivors = search.beamScoresClone()
		}

		if !progress.Stagnant(search.options.BeamWidth) {
			continue
		}

		nextDepth := targetDepth + 1

		TuneLog("no improvement at depth %d, advancing to %d", targetDepth, nextDepth)
		targetDepth = nextDepth
		progress.ResetStagnation()
	}

	return ScanStats{
		Candidates: search.candidates,
		Workers:    search.options.Workers,
	}
}

func (search *ScanSearch) logSearchStalled(targetDepth int) {
	bestScore := search.progress.BestScore()

	if math.IsInf(bestScore, 0) {
		TuneLog(
			"reward stalled after %d candidates without improvement (depth %d), nesting entry gates",
			search.progress.SinceImprovement(),
			targetDepth,
		)

		return
	}

	TuneLog(
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

func normalizeScanOptions(options ScanOptions) ScanOptions {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	return options
}
