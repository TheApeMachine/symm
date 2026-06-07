package reasoning

import (
	"context"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/replay"
)

// capitalBlockWeight is how hard the score discounts a profitable forest for tying
// up the wallet: at most this fraction is shaved when every tick blocks a wanting
// entry. It prices the opportunity cost of capital without overriding genuine edge.
const (
	capitalBlockWeight = 0.5
	exposureTimeWeight = 0.25
	velocityWeight     = 1.0
	mergedSeedCount    = 3
)

/*
SearchConfig tunes the beam search. Zero fields fall back to defaults, so a caller
can set only what it cares about.
*/
type SearchConfig struct {
	BeamWidth     int // forests carried between expansion rounds
	MaxRounds     int // hard cap on rounds
	Patience      int // stop after this many rounds with no new best
	MaxNodes      int // forests larger than this are not explored (bounds runaway depth)
	MinRoundTrips int // a profitable forest closing fewer trades than this is discounted toward 0
	Workers       int // parallel candidate scoring workers; 0 uses runtime.NumCPU()
	OnProgress    func(SearchProgress)
	OnNewBest     func(Candidate)
}

func (config SearchConfig) withDefaults() SearchConfig {
	if config.BeamWidth <= 0 {
		config.BeamWidth = 8
	}

	if config.MaxRounds <= 0 {
		config.MaxRounds = 20
	}

	if config.Patience <= 0 {
		config.Patience = 4
	}

	if config.MaxNodes <= 0 {
		config.MaxNodes = 24
	}

	return config
}

/*
Candidate is a scored reasoning forest. Score is the value the search maximises:
realized return, discounted when it rests on too few trades. Depth is NOT penalised
in the score — a deeper tree with the same return ranks equal and is kept for
further growth, with node count used only as a tie-break (prefer the simpler of two
equals) and a hard MaxNodes cap to stop runaway bloat. This is what lets the search
explore the temporal chains the playbook is meant to express.
*/
type Candidate struct {
	Forest          []reasoning.Thought
	Score           float64
	Return          float64 // realized P&L / starting capital
	RealizedEUR     float64
	StartingCapital float64
	Trades          int
	Nodes           int
}

// Result is the outcome of a search: the best forest found and how much was tried.
type Result struct {
	Best      Candidate
	Evaluated int
}

/*
Search grows reasoning forests from the data and returns the best one it can score
on the replay tape. It is deterministic: the vocabulary, seeds, and neighbours are
all generated in a fixed order, so the same rows and config yield the same forest.
*/
func Search(
	ctx context.Context,
	rows []types.Measurement,
	costs replay.ReplayCosts,
	config SearchConfig,
) (Result, error) {
	config = config.withDefaults()
	started := time.Now()
	rowCount := len(rows)

	config.reportProgress(SearchProgress{
		Phase:         "config",
		BeamSize:      config.BeamWidth,
		MaxRounds:     config.MaxRounds,
		MaxNodes:      config.MaxNodes,
		Patience:      config.Patience,
		MinRoundTrips: config.MinRoundTrips,
		Workers:       config.workerCount(),
	})

	vocab := DeriveVocabulary(rows)
	config.reportProgress(SearchProgress{
		Phase:         "vocabulary",
		RowCount:      rowCount,
		CategoryCount: len(vocab.Categories),
	})

	config.reportProgress(SearchProgress{
		Phase:    "precompile_start",
		RowCount: rowCount,
	})

	precompileStarted := time.Now()
	tape, err := replay.PrecompileTapeWorkers(rows, config.workerCount())

	if err != nil {
		return Result{}, errnie.Error(err, "optimizer/search: precompile")
	}

	config.reportProgress(SearchProgress{
		Phase:         "precompile_done",
		RowCount:      tape.Len(),
		CategoryCount: len(vocab.Categories),
		Elapsed:       time.Since(precompileStarted),
	})

	evaluated := 0
	seen := newForestDedup()

	queueTasks := func(forests [][]reasoning.Thought) []scoreTask {
		tasks := make([]scoreTask, 0, len(forests))

		for _, forest := range forests {
			nodes := countNodes(forest)

			if nodes > config.MaxNodes {
				continue
			}

			if seen.insert(forest) {
				continue
			}

			evaluated++
			tasks = append(tasks, scoreTask{forest: forest, nodes: nodes})
		}

		return tasks
	}

	scoreTasks := func(tasks []scoreTask) []Candidate {
		return evaluateCandidates(ctx, tasks, tape, costs, config)
	}

	beam := make([]Candidate, 0, config.BeamWidth)
	seeds := Seeds(vocab)

	config.reportProgress(SearchProgress{
		Phase:     "seeds_start",
		SeedCount: len(seeds),
	})

	seedScored := scoreTasks(queueTasks(seeds))
	beam = topCandidates(seedScored, config.BeamWidth)

	if len(seedScored) > 1 {
		sort.Slice(seedScored, func(leftIndex, rightIndex int) bool {
			return seedScored[leftIndex].Score > seedScored[rightIndex].Score
		})

		mergeCount := mergedSeedCount

		if mergeCount > len(seedScored) {
			mergeCount = len(seedScored)
		}

		mergeSources := make([][]reasoning.Thought, mergeCount)

		for index := 0; index < mergeCount; index++ {
			mergeSources[index] = seedScored[index].Forest
		}

		mergedForest := MergeSeedForests(mergeSources)

		if ForestStrategyCount(mergedForest) > 1 {
			for _, candidate := range scoreTasks(queueTasks([][]reasoning.Thought{mergedForest})) {
				beam = append(beam, candidate)
			}

			beam = topCandidates(beam, config.BeamWidth)
		}
	}

	if len(beam) == 0 {
		config.reportProgress(SearchProgress{
			Phase:     "done",
			Evaluated: evaluated,
			Elapsed:   time.Since(started),
		})

		return Result{Evaluated: evaluated}, nil
	}

	best := beam[0]

	config.reportProgress(SearchProgress{
		Phase:      "seeds_done",
		Evaluated:  evaluated,
		BestScore:  best.Score,
		BestReturn: best.Return,
		BestTrades: best.Trades,
		Elapsed:    time.Since(started),
	})

	stagnation := 0

	for round := 0; round < config.MaxRounds; round++ {
		roundEvaluated := evaluated
		neighbors := make([][]reasoning.Thought, 0, len(beam)*16)

		for _, member := range beam {
			neighbors = append(neighbors, Neighbors(member.Forest, vocab)...)
		}

		grown := scoreTasks(queueTasks(neighbors))

		if len(grown) == 0 {
			break
		}

		beam = topCandidates(append(beam, grown...), config.BeamWidth)

		if beam[0].Score > best.Score {
			best = beam[0]
			stagnation = 0

			if config.OnNewBest != nil {
				config.OnNewBest(best)
			}

			config.reportProgress(SearchProgress{
				Phase:      "round",
				Round:      round + 1,
				MaxRounds:  config.MaxRounds,
				Evaluated:  evaluated,
				RoundAdded: evaluated - roundEvaluated,
				BeamSize:   len(beam),
				BestScore:  best.Score,
				BestReturn: best.Return,
				BestTrades: best.Trades,
				Stagnation: stagnation,
				Patience:   config.Patience,
				Elapsed:    time.Since(started),
			})

			continue
		}

		stagnation++

		config.reportProgress(SearchProgress{
			Phase:      "round",
			Round:      round + 1,
			MaxRounds:  config.MaxRounds,
			Evaluated:  evaluated,
			RoundAdded: evaluated - roundEvaluated,
			BeamSize:   len(beam),
			BestScore:  best.Score,
			BestReturn: best.Return,
			BestTrades: best.Trades,
			Stagnation: stagnation,
			Patience:   config.Patience,
			Elapsed:    time.Since(started),
		})

		if stagnation >= config.Patience {
			break
		}
	}

	config.reportProgress(SearchProgress{
		Phase:      "done",
		Evaluated:  evaluated,
		BestScore:  best.Score,
		BestReturn: best.Return,
		BestTrades: best.Trades,
		Elapsed:    time.Since(started),
	})

	return Result{Best: best, Evaluated: evaluated}, nil
}

// topCandidates sorts by score (then simpler, then stable by encoding) and keeps the
// best width.
func topCandidates(candidates []Candidate, width int) []Candidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}

		return candidates[i].Nodes < candidates[j].Nodes
	})

	if len(candidates) > width {
		candidates = candidates[:width]
	}

	return candidates
}

func countNodes(forest []reasoning.Thought) int {
	total := 0

	var walk func(nodes []reasoning.Thought)
	walk = func(nodes []reasoning.Thought) {
		for index := range nodes {
			total++
			walk(nodes[index].Then)
		}
	}

	walk(forest)

	return total
}
