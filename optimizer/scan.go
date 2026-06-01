package optimizer

import (
	"context"
	"runtime"
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	DefaultScanBeamWidth      = 256
	DefaultScanCandidateLimit = 100000
	DefaultScanMaxThresholds  = 128
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
	Workers        int
	MaxThresholds  int
	BeamWidth      int
	CandidateLimit int
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
	candidate scanCandidate
	score     float64
}

/*
ScanSearch scores a bounded exhaustive set of candidate branch trees.
*/
type ScanSearch struct {
	ctx         context.Context
	profile     *Profile
	rows        []perspectives.Measurement
	options     ScanOptions
	bestScore   float64
	bestBranch  perspectives.BranchList
	onBest      func(BestTree)
	onCandidate func(CandidateScore)
	candidates  int
	mu          sync.Mutex
}

func NewScanSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	options ScanOptions,
) *ScanSearch {
	return &ScanSearch{
		ctx:     ctx,
		profile: profile,
		rows:    rows,
		options: normalizeScanOptions(options),
	}
}

/*
Run scores primitive branches, parent-child paths, and entry/exit sibling pairs.
*/
func (search *ScanSearch) Run() (perspectives.BranchList, ScanStats) {
	best := perspectives.BranchList{}
	search.bestScore = search.evaluate(best)

	actionBranches := search.actionBranches()
	branchers := search.branchers()

	search.score(func(send func(scanCandidate) bool) {
		search.emitActionBranches(send, actionBranches)
	})

	search.score(func(send func(scanCandidate) bool) {
		search.emitSiblingBranches(send, actionBranches)
	})

	search.score(func(send func(scanCandidate) bool) {
		search.emitNestedBranches(send, branchers, actionBranches)
	})

	stats := ScanStats{
		Candidates: search.candidates,
		Workers:    search.options.Workers,
	}

	return search.best(), stats
}

func normalizeScanOptions(options ScanOptions) ScanOptions {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	if options.MaxThresholds < 0 {
		options.MaxThresholds = DefaultScanMaxThresholds
	}

	if options.BeamWidth <= 0 {
		options.BeamWidth = DefaultScanBeamWidth
	}

	if options.CandidateLimit < 0 {
		options.CandidateLimit = DefaultScanCandidateLimit
	}

	return options
}

func (search *ScanSearch) score(
	generate func(send func(scanCandidate) bool),
) {
	candidates := make(chan scanCandidate, search.options.Workers*2)
	results := make(chan scanResult, search.options.Workers*2)
	var workers sync.WaitGroup

	for range search.options.Workers {
		workers.Add(1)

		go func() {
			defer workers.Done()

			for candidate := range candidates {
				results <- scanResult{
					candidate: candidate,
					score:     search.evaluate(candidate.branches),
				}
			}
		}()
	}

	go func() {
		generate(func(candidate scanCandidate) bool {
			index, ok := search.reserveCandidate()

			if !ok {
				return false
			}

			candidate.index = index
			candidates <- candidate

			return true
		})
		close(candidates)
		workers.Wait()
		close(results)
	}()

	for result := range results {
		search.accept(result)
	}
}

func (search *ScanSearch) reserveCandidate() (int, bool) {
	search.mu.Lock()
	defer search.mu.Unlock()

	if search.options.CandidateLimit > 0 &&
		search.candidates >= search.options.CandidateLimit {
		return 0, false
	}

	search.candidates++

	return search.candidates, true
}

func (search *ScanSearch) accept(result scanResult) {
	if search.onCandidate != nil {
		search.onCandidate(CandidateScore{
			Candidate: result.candidate.index,
			Score:     result.score,
			Branches:  result.candidate.branches.Clone(),
		})
	}

	search.mu.Lock()
	defer search.mu.Unlock()

	if result.score <= search.bestScore {
		return
	}

	search.bestScore = result.score
	search.bestBranch = result.candidate.branches.Clone()

	if search.onBest != nil {
		search.onBest(BestTree{
			Iteration: result.candidate.index,
			Score:     result.score,
			Branches:  result.candidate.branches.Clone(),
		})
	}
}

func (search *ScanSearch) best() perspectives.BranchList {
	search.mu.Lock()
	defer search.mu.Unlock()

	return search.bestBranch.Clone()
}

func (search *ScanSearch) evaluate(branches perspectives.BranchList) float64 {
	return NewReplaySimulation(search.ctx, branches, search.rows).Score()
}

func (search *ScanSearch) actionBranches() []scanCandidate {
	candidates := make([]scanCandidate, 0)

	for _, branch := range search.branches(perspectives.ObservationNotHolding) {
		for _, actionType := range searchEntryActions {
			if actionType == perspectives.ActionNone {
				continue
			}

			next := branch
			next.Action = perspectives.Action{Type: actionType}
			candidates = append(candidates, scanCandidate{
				branches: perspectives.BranchList{next},
				group:    actionGroupEntry,
			})
		}
	}

	for _, branch := range search.branches(perspectives.ObservationHolding) {
		for _, actionType := range searchExitActions {
			if actionType == perspectives.ActionNone {
				continue
			}

			next := branch
			next.Action = perspectives.Action{Type: actionType}
			candidates = append(candidates, scanCandidate{
				branches: perspectives.BranchList{next},
				group:    actionGroupExit,
			})
		}
	}

	return candidates
}

func (search *ScanSearch) branchers() []perspectives.Branch {
	return search.branches(perspectives.ObservationNone)
}

func (search *ScanSearch) branches(
	observation perspectives.ObservationType,
) []perspectives.Branch {
	categories := search.profile.Categories()
	branches := make([]perspectives.Branch, 0)

	for _, category := range categories {
		for _, unit := range searchUnits {
			values := search.profile.Values(
				category,
				unit,
				search.options.MaxThresholds,
			)

			for _, condition := range searchConditions {
				for _, value := range values {
					branches = append(branches, perspectives.Branch{
						Category:    category,
						Observation: observation,
						Condition:   condition,
						Unit:        unit,
						Value:       value,
						ValueSet:    true,
					})
				}
			}
		}
	}

	return branches
}

func (search *ScanSearch) emitActionBranches(
	send func(scanCandidate) bool,
	candidates []scanCandidate,
) bool {
	for _, candidate := range candidates {
		if !send(candidate) {
			return false
		}
	}

	return true
}

func (search *ScanSearch) emitNestedBranches(
	send func(scanCandidate) bool,
	branchers []perspectives.Branch,
	actions []scanCandidate,
) {
	for _, brancher := range branchers {
		for _, action := range actions {
			child := action.branches[0]
			parent := brancher
			parent.Branches = []perspectives.Branch{child}

			if !send(scanCandidate{
				branches: perspectives.BranchList{parent},
				group:    action.group,
			}) {
				return
			}
		}
	}
}

func (search *ScanSearch) emitSiblingBranches(
	send func(scanCandidate) bool,
	actions []scanCandidate,
) {
	entries := search.groupCandidates(actions, actionGroupEntry)
	exits := search.groupCandidates(actions, actionGroupExit)

	for _, entry := range entries {
		for _, exit := range exits {
			branches := entry.branches.Clone()
			branches = append(branches, exit.branches.Clone()...)

			if !send(scanCandidate{
				branches: branches,
				group:    actionGroupEntry,
			}) {
				return
			}
		}
	}
}

func (search *ScanSearch) groupCandidates(
	candidates []scanCandidate,
	group actionGroup,
) []scanCandidate {
	grouped := make([]scanCandidate, 0, search.options.BeamWidth)

	for _, candidate := range candidates {
		if candidate.group != group {
			continue
		}

		grouped = append(grouped, candidate)

		if len(grouped) == search.options.BeamWidth {
			return grouped
		}
	}

	return grouped
}
