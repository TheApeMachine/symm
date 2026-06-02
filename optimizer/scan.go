package optimizer

import (
	"context"
	"math"
	"runtime"
	"sort"
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	DefaultScanBeamWidth       = 256
	DefaultScanCandidateLimit  = 10000
	DefaultScanMaxThresholds   = 128
	DefaultTuneMaxMeasurements = 250000
	DefaultBootstrapSeedBudget = 2048
	DefaultBootstrapPairBudget = 512
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
	ctx                 context.Context
	profile             *Profile
	rows                []perspectives.Measurement
	tape                ReplayTape
	coOccurrence        *CoOccurrenceIndex
	options             ScanOptions
	guard               *OverfitGuard
	bestScore           float64
	bestClosedTrades    int
	bestBranch          perspectives.BranchList
	onBest              func(BestTree)
	onCandidate         func(CandidateScore)
	candidates          int
	phaseCandidates     int
	phaseCandidateLimit int
	topK                int
	topScores           []CandidateScore
	beamScores          []CandidateScore
	pairAffinity        *PairAffinityIndex
	mu                  sync.Mutex
}

func NewScanSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	options ScanOptions,
) *ScanSearch {
	options = normalizeScanOptions(options)
	profile.PrepareCache()
	tape := PrecompileTape(rows)
	coOccurrence := NewCoOccurrenceIndex(tape)

	return &ScanSearch{
		ctx:          ctx,
		profile:      profile,
		rows:         rows,
		tape:         tape,
		coOccurrence: coOccurrence,
		options:      options,
		guard:        NewOverfitGuard(ctx, options.Guard, tape),
		pairAffinity: NewPairAffinityIndex(),
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

	actionBranches := search.actionBranches()
	deepeningBudget := search.deepeningCandidateBudget()
	survivors := make([]CandidateScore, 0)

	search.phaseCandidates = 0
	search.phaseCandidateLimit = search.bootstrapSeedBudget()
	search.runBeamPhase(func(send func(scanCandidate) bool) {
		search.emitDecisionSeeds(send)
	})
	seedSurvivors := search.beamScoresClone()

	search.phaseCandidates = 0
	search.phaseCandidateLimit = search.bootstrapPairBudget()
	search.runBeamPhase(func(send func(scanCandidate) bool) {
		search.emitSiblingBranches(send, actionBranches, search.bootstrapPairBudget())
	})
	search.mergeDeepeningSurvivors(seedSurvivors)
	survivors = search.beamScoresClone()

	for depth := 1; depth <= search.options.MaxReasoningSteps; depth++ {
		if len(survivors) == 0 {
			break
		}

		previous := survivors
		branchers := search.rankedEntryBranchers()
		deepenBudget := max(1, deepeningBudget/2)
		widenBudget := max(1, deepeningBudget-deepenBudget)

		search.phaseCandidates = 0
		search.phaseCandidateLimit = deepenBudget

		search.runBeamPhase(func(send func(scanCandidate) bool) {
			search.emitDeepeningExpansions(send, survivors, branchers)
		})
		search.mergeDeepeningSurvivors(previous)
		survivors = search.beamScoresClone()

		search.phaseCandidates = 0
		search.phaseCandidateLimit = widenBudget

		search.runBeamPhase(func(send func(scanCandidate) bool) {
			search.emitWidenExpansions(send, survivors, actionBranches)
		})
		search.mergeDeepeningSurvivors(append(previous, survivors...))
		survivors = search.beamScoresClone()
	}

	return ScanStats{
		Candidates: search.candidates,
		Workers:    search.options.Workers,
	}
}

func (search *ScanSearch) bootstrapTotalBudget() int {
	if search.options.CandidateLimit <= 0 {
		return DefaultBootstrapSeedBudget + DefaultBootstrapPairBudget
	}

	if search.options.CandidateLimit <= 4096 {
		return max(128, search.options.CandidateLimit/4)
	}

	return DefaultBootstrapSeedBudget + DefaultBootstrapPairBudget
}

func (search *ScanSearch) bootstrapSeedBudget() int {
	total := search.bootstrapTotalBudget()

	if total <= DefaultBootstrapPairBudget {
		return total
	}

	return min(DefaultBootstrapSeedBudget, total-DefaultBootstrapPairBudget)
}

func (search *ScanSearch) bootstrapPairBudget() int {
	total := search.bootstrapTotalBudget()

	return min(DefaultBootstrapPairBudget, total-search.bootstrapSeedBudget())
}

func (search *ScanSearch) deepeningCandidateBudget() int {
	if search.options.CandidateLimit <= 0 {
		return 0
	}

	remaining := search.options.CandidateLimit - search.bootstrapTotalBudget()

	if remaining <= 0 {
		return max(1, search.options.CandidateLimit/(search.options.MaxReasoningSteps+1))
	}

	budget := remaining / search.options.MaxReasoningSteps

	if budget <= 0 {
		return remaining
	}

	return budget
}

func (search *ScanSearch) beamScoresClone() []CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	cloned := make([]CandidateScore, len(search.beamScores))

	for index, entry := range search.beamScores {
		cloned[index] = CandidateScore{
			Candidate:     entry.Candidate,
			Score:         entry.Score,
			AdjustedScore: entry.AdjustedScore,
			ClosedTrades:  entry.ClosedTrades,
			Branches:      entry.Branches.Clone(),
		}
	}

	return cloned
}

func (search *ScanSearch) topScoresClone() []CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	cloned := make([]CandidateScore, len(search.topScores))

	for index, entry := range search.topScores {
		cloned[index] = CandidateScore{
			Candidate:     entry.Candidate,
			Score:         entry.Score,
			AdjustedScore: entry.AdjustedScore,
			ClosedTrades:  entry.ClosedTrades,
			Branches:      entry.Branches.Clone(),
		}
	}

	return cloned
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

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = DefaultMaxReasoningSteps
	}

	options.Guard = normalizeGuardOptions(options.Guard)

	if options.Guard.MaxReasoningSteps <= 0 {
		options.Guard.MaxReasoningSteps = options.MaxReasoningSteps
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
				canonical := perspectives.CanonicalPlaybookBranches(
					candidate.branches,
				)
				replay := NewReplaySimulationWithTape(
					search.ctx, canonical, search.tape,
				).Result()
				rawScore := replay.Score

				results <- scanResult{
					candidate:     candidate,
					branches:      canonical,
					rawScore:      rawScore,
					adjustedScore: search.guard.AdjustedScore(rawScore, canonical),
					closedTrades:  replay.ClosedTrades,
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

	if search.phaseCandidateLimit > 0 &&
		search.phaseCandidates >= search.phaseCandidateLimit {
		return 0, false
	}

	search.candidates++
	search.phaseCandidates++

	return search.candidates, true
}

func (search *ScanSearch) accept(result scanResult) {
	canonical := result.branches
	entry := CandidateScore{
		Candidate:     result.candidate.index,
		Score:         result.rawScore,
		AdjustedScore: result.adjustedScore,
		ClosedTrades:  result.closedTrades,
		Branches:      canonical,
	}

	if search.onCandidate != nil && beamEligible(entry) {
		search.onCandidate(entry)
	}

	search.recordPairAffinity(entry)
	search.recordBeam(entry)

	if search.guard.AcceptTrainCandidate(canonical) || trainSeedEligible(entry) {
		search.recordTopK(entry)
	}

	if !search.guard.ImprovesPersistedBest(
		result.adjustedScore,
		result.closedTrades,
		search.bestScore,
		search.bestClosedTrades,
	) {
		return
	}

	if !search.guard.PersistCandidate(canonical) {
		return
	}

	search.mu.Lock()
	defer search.mu.Unlock()

	if !search.guard.ImprovesPersistedBest(
		result.adjustedScore,
		result.closedTrades,
		search.bestScore,
		search.bestClosedTrades,
	) {
		return
	}

	search.bestScore = result.adjustedScore
	search.bestClosedTrades = result.closedTrades
	search.bestBranch = canonical.Clone()

	if search.onBest != nil {
		search.onBest(BestTree{
			Iteration: result.candidate.index,
			Score:     result.adjustedScore,
			Branches:  canonical.Clone(),
		})
	}
}

func (search *ScanSearch) mergeDeepeningSurvivors(previous []CandidateScore) {
	search.mu.Lock()
	defer search.mu.Unlock()

	search.beamScores = collapseDepthStratifiedBeam(
		append(search.beamScores, previous...),
		search.options.BeamWidth,
	)
}

func (search *ScanSearch) recordBeam(entry CandidateScore) {
	if !beamEligible(entry) {
		return
	}

	search.mu.Lock()
	defer search.mu.Unlock()

	search.beamScores = append(search.beamScores, entry)

	pruneLimit := search.options.BeamWidth * DefaultBeamPruneFactor

	if len(search.beamScores) > pruneLimit {
		search.beamScores = collapseDepthStratifiedBeam(
			search.beamScores, search.options.BeamWidth,
		)
	}
}

func (search *ScanSearch) recordPairAffinity(entry CandidateScore) {
	if search.pairAffinity == nil {
		return
	}

	entryCategory, exitCategory, ok := flatPairCategories(entry.Branches)

	if ok {
		search.pairAffinity.RecordFlatPair(entryCategory, exitCategory, entry.Score)
	}
}

func (search *ScanSearch) recordTopK(entry CandidateScore) {
	if search.topK <= 0 {
		return
	}

	search.mu.Lock()
	defer search.mu.Unlock()

	search.topScores = insertTopK(search.topScores, entry, search.topK)
}

func (search *ScanSearch) best() perspectives.BranchList {
	search.mu.Lock()
	defer search.mu.Unlock()

	return search.bestBranch.Clone()
}

func (search *ScanSearch) evaluateRaw(branches perspectives.BranchList) float64 {
	return NewReplaySimulationWithTape(
		search.ctx, branches, search.tape,
	).Result().Score
}

func (search *ScanSearch) evaluateAdjusted(branches perspectives.BranchList) float64 {
	return search.guard.AdjustedScore(
		search.evaluateRaw(branches), branches,
	)
}

func (search *ScanSearch) rankedEntryBranchers() []perspectives.Branch {
	branchers := search.branchers()

	if len(branchers) <= search.options.BeamWidth {
		return branchers
	}

	type rankedBrancher struct {
		branch perspectives.Branch
		score  float64
		passes int
	}

	ranked := make([]rankedBrancher, 0, len(branchers))
	byCategory := make(map[perspectives.CategoryType][]rankedBrancher)

	for _, brancher := range branchers {
		passes := search.profile.GatePassCount(
			brancher.Category,
			brancher.Unit,
			brancher.Condition,
			brancher.Value,
		)
		entry := rankedBrancher{
			branch: brancher,
			score: search.profile.GateSelectivityScore(
				brancher.Category,
				brancher.Unit,
				brancher.Condition,
				brancher.Value,
			),
			passes: passes,
		}

		ranked = append(ranked, entry)
		byCategory[brancher.Category] = append(byCategory[brancher.Category], entry)
	}

	less := func(left, right rankedBrancher) bool {
		if left.score != right.score {
			return left.score > right.score
		}

		return left.passes > right.passes
	}

	for category := range byCategory {
		sort.Slice(byCategory[category], func(leftIndex, rightIndex int) bool {
			return less(byCategory[category][leftIndex], byCategory[category][rightIndex])
		})
	}

	sort.Slice(ranked, func(leftIndex, rightIndex int) bool {
		return less(ranked[leftIndex], ranked[rightIndex])
	})

	limited := make([]perspectives.Branch, 0, search.options.BeamWidth)
	seen := make(map[string]struct{}, search.options.BeamWidth)
	categories := search.profile.Categories()

	for layer := 0; len(limited) < search.options.BeamWidth; layer++ {
		progress := false

		for _, category := range categories {
			candidates := byCategory[category]

			if layer >= len(candidates) {
				continue
			}

			key := branchFingerprint(candidates[layer].branch)

			if _, ok := seen[key]; ok {
				continue
			}

			limited = append(limited, candidates[layer].branch)
			seen[key] = struct{}{}
			progress = true

			if len(limited) == search.options.BeamWidth {
				return limited
			}
		}

		if !progress {
			break
		}
	}

	for _, candidate := range ranked {
		if len(limited) == search.options.BeamWidth {
			break
		}

		key := branchFingerprint(candidate.branch)

		if _, ok := seen[key]; ok {
			continue
		}

		limited = append(limited, candidate.branch)
		seen[key] = struct{}{}
	}

	return limited
}

func (search *ScanSearch) emitDecisionSeeds(send func(scanCandidate) bool) {
	for _, playbook := range BuildDecisionSeedPlaybooks(search.profile, search.coOccurrence) {
		if !send(scanCandidate{
			branches: playbook,
			group:    actionGroupEntry,
		}) {
			return
		}
	}
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

func (search *ScanSearch) emitSiblingBranches(
	send func(scanCandidate) bool,
	actions []scanCandidate,
	maxPairs int,
) {
	entries := search.groupCandidates(actions, actionGroupEntry)
	exits := search.groupCandidates(actions, actionGroupExit)
	emitted := 0

	for _, entry := range entries {
		entryCategory := primaryEntryCategory(entry.branches)
		rankedExits := rankExitsByAffinity(search.pairAffinity, entryCategory, exits)

		for _, exit := range rankedExits {
			if maxPairs > 0 && emitted >= maxPairs {
				return
			}

			if search.coOccurrence != nil {
				if !entryExitPairReachable(
					search.coOccurrence, entry.branches, exit.branches,
				) {
					continue
				}
			}

			branches := entry.branches.Clone()
			branches = append(branches, exit.branches.Clone()...)

			if !send(scanCandidate{
				branches: branches,
				group:    actionGroupEntry,
			}) {
				return
			}

			emitted++
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
