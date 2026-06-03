package scan

import (
	"context"
	"sort"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/beam"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

func (search *ScanSearch) BeamScoresClone() []types.CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	cloned := make([]types.CandidateScore, len(search.beamScores))

	for index, entry := range search.beamScores {
		cloned[index] = types.CandidateScore{
			Candidate:     entry.Candidate,
			Score:         entry.Score,
			AdjustedScore: entry.AdjustedScore,
			ClosedTrades:  entry.ClosedTrades,
			Branches:      entry.Branches.Clone(),
		}
	}

	return cloned
}

func (search *ScanSearch) topScoresClone() []types.CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	cloned := make([]types.CandidateScore, len(search.topScores))

	for index, entry := range search.topScores {
		cloned[index] = types.CandidateScore{
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
	if search.pool != nil {
		search.scoreWithPool(generate)

		return
	}

	search.scoreWithWorkers(generate)
}

func (search *ScanSearch) scoreWithPool(
	generate func(send func(scanCandidate) bool),
) {
	inFlight := search.options.Workers * 2
	pending := make(chan chan *qpool.QValue[any], inFlight)
	var collector sync.WaitGroup

	collector.Add(1)

	go func() {
		defer collector.Done()

		for task := range pending {
			value := <-task

			if value.Error != nil {
				continue
			}

			result, ok := value.Value.(scanResult)

			if !ok {
				continue
			}

			search.accept(result)
		}
	}()

	generate(func(candidate scanCandidate) bool {
		index, ok := search.reserveCandidate()

		if !ok {
			return false
		}

		candidate.index = index
		task := search.pool.ScheduleFast(search.ctx, func(context.Context) (any, error) {
			return search.scoreCandidate(candidate), nil
		})

		select {
		case pending <- task:
			return true
		case <-search.ctx.Done():
			return false
		}
	})

	close(pending)
	collector.Wait()
}

func (search *ScanSearch) scoreWithWorkers(
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
				results <- search.scoreCandidate(candidate)
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

func (search *ScanSearch) scoreCandidate(candidate scanCandidate) scanResult {
	canonical := perspectives.CanonicalPlaybookBranches(candidate.branches)
	replayResult := replay.NewReplaySimulationWithTape(
		search.ctx, canonical, search.tape,
	).Result()

	return scanResult{
		candidate:     candidate,
		branches:      canonical,
		rawScore:      replayResult.Score,
		adjustedScore: search.guard.AdjustedScore(replayResult.Score, canonical),
		closedTrades:  replayResult.ClosedTrades,
	}
}

func (search *ScanSearch) canScoreMore() bool {
	search.mu.Lock()
	defer search.mu.Unlock()

	return search.canScoreMoreLocked()
}

func (search *ScanSearch) canScoreMoreLocked() bool {
	if search.options.CandidateLimit > 0 &&
		search.candidates >= search.options.CandidateLimit {
		return false
	}

	if search.haltPhaseOnStagnation &&
		search.progress != nil &&
		search.progress.Stagnant(search.options.BeamWidth) {
		return false
	}

	if search.phaseCandidateLimit > 0 &&
		search.phaseCandidates >= search.phaseCandidateLimit {
		return false
	}

	return true
}

func (search *ScanSearch) reserveCandidate() (int, bool) {
	search.mu.Lock()
	defer search.mu.Unlock()

	if !search.canScoreMoreLocked() {
		return 0, false
	}

	search.candidates++
	search.phaseCandidates++

	return search.candidates, true
}

func (search *ScanSearch) accept(result scanResult) {
	canonical := result.branches
	entry := types.CandidateScore{
		Candidate:     result.candidate.index,
		Score:         result.rawScore,
		AdjustedScore: result.adjustedScore,
		ClosedTrades:  result.closedTrades,
		Branches:      canonical,
	}

	if search.OnCandidate != nil && beam.BeamEligible(entry) {
		search.OnCandidate(entry)
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

	if search.guard.AcceptTrainCandidate(canonical) || beam.TrainSeedEligible(entry) {
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

	if search.OnBest != nil {
		search.OnBest(types.BestTree{
			Iteration: result.candidate.index,
			Score:     result.adjustedScore,
			Branches:  canonical.Clone(),
		})
	}
}

func (search *ScanSearch) mergeDeepeningSurvivors(previous []types.CandidateScore) {
	search.mu.Lock()
	defer search.mu.Unlock()

	search.beamScores = beam.CollapseScoreBeam(
		append(search.beamScores, previous...),
		search.options.BeamWidth,
	)
}

func (search *ScanSearch) recordBeam(entry types.CandidateScore) {
	if !beam.BeamEligible(entry) {
		return
	}

	search.mu.Lock()
	defer search.mu.Unlock()

	search.beamScores = append(search.beamScores, entry)

	pruneLimit := search.options.BeamWidth * search.budget.BeamPruneFactor

	if len(search.beamScores) > pruneLimit {
		search.beamScores = beam.CollapseScoreBeam(
			search.beamScores, search.options.BeamWidth,
		)
	}
}

func (search *ScanSearch) recordPairAffinity(entry types.CandidateScore) {
	if search.pairAffinity == nil {
		return
	}

	entryCategory, exitCategory, ok := flatPairCategories(entry.Branches)

	if ok {
		search.pairAffinity.RecordFlatPair(entryCategory, exitCategory, entry.Score)
	}
}

func (search *ScanSearch) recordTopK(entry types.CandidateScore) {
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

func (search *ScanSearch) WalkForwardFinalists() []types.CandidateScore {
	return search.walkForwardFinalists()
}

func (search *ScanSearch) walkForwardFinalists() []types.CandidateScore {
	search.mu.Lock()
	defer search.mu.Unlock()

	pool := make(
		[]types.CandidateScore,
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
		pool = append(pool, types.CandidateScore{
			Score:         search.bestScore,
			AdjustedScore: search.bestScore,
			ClosedTrades:  search.bestClosedTrades,
			Branches:      search.bestBranch.Clone(),
		})
	}

	return beam.DedupeCandidatesByBranch(pool)
}

func cloneCandidateScore(entry types.CandidateScore) types.CandidateScore {
	return types.CandidateScore{
		Candidate:     entry.Candidate,
		Score:         entry.Score,
		AdjustedScore: entry.AdjustedScore,
		ClosedTrades:  entry.ClosedTrades,
		Branches:      entry.Branches.Clone(),
	}
}

func insertTopK(
	top []types.CandidateScore, entry types.CandidateScore, limit int,
) []types.CandidateScore {
	top = append(top, entry)
	sort.Slice(top, func(leftIndex, rightIndex int) bool {
		left := top[leftIndex]
		right := top[rightIndex]

		if left.AdjustedScore != right.AdjustedScore {
			return left.AdjustedScore > right.AdjustedScore
		}

		return left.Candidate < right.Candidate
	})

	if len(top) > limit {
		top = top[:limit]
	}

	return top
}

func (search *ScanSearch) evaluateRaw(branches perspectives.BranchList) float64 {
	return replay.NewReplaySimulationWithTape(
		search.ctx, branches, search.tape,
	).Result().Score
}
