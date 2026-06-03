package mcts

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/progress"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
Seeds scores beam survivors and attaches them as MCTS root children.
*/
type Seeds struct {
	search *TreeSearch
}

func NewSeeds(search *TreeSearch) *Seeds {
	return &Seeds{search: search}
}

func (seeds *Seeds) Run(candidates []types.CandidateScore, priorVisits int) {
	if len(candidates) == 0 {
		seeds.search.tree.Root.untried = seeds.search.moves.Available(
			seeds.search.tree.Root.branches,
		)

		return
	}

	if seeds.search.pool != nil && len(candidates) > 1 {
		seeds.runWithPool(candidates, priorVisits)

		return
	}

	reporter := progress.NewTuneProgressReporter(len(candidates))
	log.TuneLog("mcts seeding %d roots", len(candidates))

	viable := 0

	for index, candidate := range candidates {
		beforeChildren := len(seeds.search.tree.Root.children)
		seeds.apply(candidate, priorVisits)

		if len(seeds.search.tree.Root.children) > beforeChildren {
			viable++
		}

		if reporter.ShouldLog(index + 1) {
			reporter.MarkLogged()
			logSeedProgress(
				seeds.search,
				index+1,
				len(candidates),
				viable,
				reporter.Elapsed(),
			)
		}
	}

	seeds.finish(len(candidates), viable, reporter.Elapsed())
}

type seedResult struct {
	child        *Node
	priorVisits  int
	score        float64
	closedTrades int
	canonical    perspectives.BranchList
}

func (seeds *Seeds) runWithPool(candidates []types.CandidateScore, priorVisits int) {
	reporter := progress.NewTuneProgressReporter(len(candidates))
	log.TuneLog("mcts seeding %d roots", len(candidates))

	tasks := make([]chan *qpool.QValue[any], 0, len(candidates))

	for _, candidate := range candidates {
		tasks = append(tasks, seeds.search.pool.ScheduleFast(seeds.search.ctx, func(context.Context) (any, error) {
			return seeds.score(candidate, priorVisits), nil
		}))
	}

	viable := 0

	for index, task := range tasks {
		value := <-task

		if value.Error != nil {
			if reporter.ShouldLog(index + 1) {
				reporter.MarkLogged()
				logSeedProgress(seeds.search, index+1, len(candidates), viable, reporter.Elapsed())
			}

			continue
		}

		result, ok := value.Value.(seedResult)

		if !ok || result.child == nil {
			if reporter.ShouldLog(index + 1) {
				reporter.MarkLogged()
				logSeedProgress(seeds.search, index+1, len(candidates), viable, reporter.Elapsed())
			}

			continue
		}

		seeds.attach(result)
		viable++

		if reporter.ShouldLog(index + 1) {
			reporter.MarkLogged()
			logSeedProgress(seeds.search, index+1, len(candidates), viable, reporter.Elapsed())
		}
	}

	seeds.finish(len(candidates), viable, reporter.Elapsed())
}

func (seeds *Seeds) apply(candidate types.CandidateScore, priorVisits int) {
	result := seeds.score(candidate, priorVisits)

	if result.child == nil {
		return
	}

	seeds.attach(result)
}

func (seeds *Seeds) attach(result seedResult) {
	seeds.search.tree.Root.children = append(seeds.search.tree.Root.children, result.child)
	seeds.search.tree.Root.visits += result.priorVisits

	if seeds.search.guard.ImprovesPersistedBest(
		result.score, result.closedTrades, seeds.search.bestScore, seeds.search.bestClosedTrades,
	) && seeds.search.guard.PersistCandidate(result.canonical) {
		seeds.search.bestScore = result.score
		seeds.search.bestClosedTrades = result.closedTrades
		seeds.search.best = result.canonical.Clone()
		seeds.search.emitBest(0, result.canonical, result.score)
	}
}

func (seeds *Seeds) score(candidate types.CandidateScore, priorVisits int) seedResult {
	score := seeds.search.evaluate(candidate.Branches)

	if perspectives.HasInvalidTopLevelDenySiblings(candidate.Branches) {
		return seedResult{}
	}

	canonical := perspectives.CanonicalPlaybookBranches(candidate.Branches)
	reward := normalizeReward(score, seeds.search.rewardScale)
	replay := replay.NewReplaySimulationWithTape(
		seeds.search.ctx, canonical, seeds.search.tape,
	).Result()

	return seedResult{
		child: &Node{
			branches: canonical.Clone(),
			parent:   seeds.search.tree.Root,
			visits:   priorVisits,
			value:    reward * float64(priorVisits),
			untried:  seeds.search.moves.Available(canonical),
		},
		priorVisits:  priorVisits,
		score:        score,
		closedTrades: replay.ClosedTrades,
		canonical:    canonical,
	}
}

func (seeds *Seeds) finish(total int, viable int, elapsed time.Duration) {
	seeds.search.tree.ensureUntried()

	log.TuneLog(
		"mcts seeding finished %d/%d viable roots (%s)",
		viable,
		total,
		elapsed,
	)
}

func logSeedProgress(
	search *TreeSearch,
	completed int,
	total int,
	viable int,
	elapsed time.Duration,
) {
	bestScore := search.bestScore

	if isInf(bestScore) {
		log.TuneLog(
			"mcts seeding %d/%d (%d viable, %s)",
			completed,
			total,
			viable,
			elapsed,
		)

		return
	}

	log.TuneLog(
		"mcts seeding %d/%d (%d viable, %s, best %.6f)",
		completed,
		total,
		viable,
		elapsed,
		bestScore,
	)
}
