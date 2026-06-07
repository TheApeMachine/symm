package reasoning

import (
	"context"
	"runtime"
	"sync"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/optimizer/replay"
)

type scoreTask struct {
	forest []reasoning.Thought
	nodes  int
}

func (config SearchConfig) workerCount() int {
	if config.Workers > 0 {
		return config.Workers
	}

	return runtime.NumCPU()
}

func scoreForest(
	ctx context.Context,
	forest []reasoning.Thought,
	nodes int,
	tape replay.ReplayTape,
	costs replay.ReplayCosts,
	config SearchConfig,
) Candidate {
	result := replay.NewThoughtSimulation(ctx, forest, tape, costs).Result()
	credited := walletVelocityScore(result)

	if credited > 0 {
		strategies := ForestStrategyCount(forest)

		if strategies >= 2 {
			credited *= 1 + strategyBreadthBonus*float64(strategies-1)
		}

		if config.MinRoundTrips > 0 && result.ClosedTrades < config.MinRoundTrips {
			credited *= float64(result.ClosedTrades) / float64(config.MinRoundTrips)
		}

		if tape.Len() > 0 && result.FundBlocked > 0 {
			blockRate := float64(result.FundBlocked) / float64(tape.Len())

			if blockRate > 1 {
				blockRate = 1
			}

			credited *= 1 - capitalBlockWeight*blockRate
		}

		if result.TotalTicks > 0 && result.ExposureTicks > 0 {
			exposureRate := float64(result.ExposureTicks) / float64(result.TotalTicks)

			if exposureRate > 1 {
				exposureRate = 1
			}

			credited *= 1 - exposureTimeWeight*exposureRate
		}
	}

	return Candidate{
		Forest:          forest,
		Score:           credited,
		Return:          result.Score,
		RealizedEUR:     result.RealizedEUR,
		StartingCapital: result.StartingCapital,
		Trades:          result.ClosedTrades,
		Nodes:           nodes,
	}
}

func evaluateCandidates(
	ctx context.Context,
	tasks []scoreTask,
	tape replay.ReplayTape,
	costs replay.ReplayCosts,
	config SearchConfig,
) []Candidate {
	if len(tasks) == 0 {
		return nil
	}

	workers := config.workerCount()

	if workers > len(tasks) {
		workers = len(tasks)
	}

	results := make([]Candidate, len(tasks))
	taskQueue := make(chan int, len(tasks))

	var workerGroup sync.WaitGroup

	for range workers {
		workerGroup.Add(1)

		go func() {
			defer workerGroup.Done()

			for taskIndex := range taskQueue {
				task := tasks[taskIndex]
				results[taskIndex] = scoreForest(
					ctx,
					task.forest,
					task.nodes,
					tape,
					costs,
					config,
				)
			}
		}()
	}

	for taskIndex := range tasks {
		taskQueue <- taskIndex
	}

	close(taskQueue)
	workerGroup.Wait()

	return results
}
