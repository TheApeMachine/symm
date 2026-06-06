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
	credited := result.Score

	if credited > 0 {
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
	}

	return Candidate{
		Forest: forest,
		Score:  credited,
		Return: result.Score,
		Trades: result.ClosedTrades,
		Nodes:  nodes,
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
