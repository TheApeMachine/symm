package optimizer

import (
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

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

	if search.haltPhaseOnStagnation &&
		search.progress != nil &&
		search.progress.Stagnant(search.options.BeamWidth) {
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

	if search.progress != nil {
		search.progress.Record(
			result.adjustedScore,
			result.closedTrades,
			search.guard.ImprovesPersistedBest,
		)
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

	search.beamScores = collapseScoreBeam(
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

	pruneLimit := search.options.BeamWidth * search.budget.BeamPruneFactor

	if len(search.beamScores) > pruneLimit {
		search.beamScores = collapseScoreBeam(
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

func (search *ScanSearch) walkForwardFinalists() []CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	pool := make(
		[]CandidateScore,
		0,
		len(search.topScores)+len(search.beamScores)+1,
	)

	for _, entry := range search.topScores {
		pool = append(pool, cloneCandidateScore(entry))
	}

	for _, entry := range search.beamScores {
		pool = append(pool, cloneCandidateScore(entry))
	}

	if len(search.bestBranch) > 0 {
		pool = append(pool, CandidateScore{
			Score:         search.bestScore,
			AdjustedScore: search.bestScore,
			ClosedTrades:  search.bestClosedTrades,
			Branches:      search.bestBranch.Clone(),
		})
	}

	return dedupeCandidatesByBranch(pool)
}

func cloneCandidateScore(entry CandidateScore) CandidateScore {
	return CandidateScore{
		Candidate:     entry.Candidate,
		Score:         entry.Score,
		AdjustedScore: entry.AdjustedScore,
		ClosedTrades:  entry.ClosedTrades,
		Branches:      entry.Branches.Clone(),
	}
}

func (search *ScanSearch) evaluateRaw(branches perspectives.BranchList) float64 {
	return NewReplaySimulationWithTape(
		search.ctx, branches, search.tape,
	).Result().Score
}
