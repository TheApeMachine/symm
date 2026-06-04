package tune

import (
	"context"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/reasoning"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
TuneMeasurements grows reasoning forests from a recorded measurement tape and writes
the best one it finds to the playbook. Realized round-trip PnL on the replay is the
only objective; the tree's depth and breadth are discovered, not preset.
*/
func TuneMeasurements(
	ctx context.Context,
	rows []perspectives.Measurement,
	options types.TuneOptions,
) (types.SessionSummary, error) {
	log.TuneLog("searching reasoning forests over %d rows", len(rows))

	costs := replay.DefaultReplayCosts()
	config := reasoning.SearchConfig{
		BeamWidth:     options.BeamWidth,
		MaxRounds:     options.MaxRounds,
		MaxNodes:      options.MaxNodes,
		MinRoundTrips: options.MinRoundTrips,
	}

	result := reasoning.Search(ctx, rows, costs, config)
	best := result.Best

	strategies := strategyCount(best.Forest)
	depth := forestDepth(best.Forest)

	if options.OnCandidate != nil {
		options.OnCandidate(types.CandidateScore{
			Candidate:    result.Evaluated,
			Score:        best.Return,
			ClosedTrades: best.Trades,
			Depth:        depth,
			Strategies:   strategies,
			Thoughts:     best.Forest,
		})
	}

	if options.OnBest != nil {
		options.OnBest(types.BestTree{
			Iteration: result.Evaluated,
			Score:     best.Score,
			Return:    best.Return,
			Trades:    best.Trades,
			Nodes:     best.Nodes,
			Thoughts:  best.Forest,
		})
	}

	if options.OutputPath != "" && len(best.Forest) > 0 {
		if err := io.WriteThoughts(options.OutputPath, best.Forest); err != nil {
			return types.SessionSummary{}, err
		}
	}

	return types.SessionSummary{
		MeasurementCount: len(rows),
		Strategies:       strategies,
		Nodes:            best.Nodes,
		Trades:           best.Trades,
		Evaluated:        result.Evaluated,
		BestReturn:       best.Return,
		BestScore:        best.Score,
	}, nil
}

// forestDepth is the deepest Then-chain in the forest.
func forestDepth(forest []perspectives.Thought) int {
	deepest := 0

	var walk func(thought perspectives.Thought, depth int)
	walk = func(thought perspectives.Thought, depth int) {
		if depth > deepest {
			deepest = depth
		}

		for _, child := range thought.Then {
			walk(child, depth+1)
		}
	}

	for _, thought := range forest {
		walk(thought, 1)
	}

	return deepest
}

// strategyCount is the number of root branches that reach an entry.
func strategyCount(forest []perspectives.Thought) int {
	var reachesEntry func(thought perspectives.Thought) bool
	reachesEntry = func(thought perspectives.Thought) bool {
		if perspectives.IsEntryAction(thought.Do.Type) {
			return true
		}

		for _, child := range thought.Then {
			if reachesEntry(child) {
				return true
			}
		}

		return false
	}

	count := 0

	for _, thought := range forest {
		if reachesEntry(thought) {
			count++
		}
	}

	return count
}
