package mcts

import (
	"math"
	"time"

	"github.com/theapemachine/symm/optimizer/log"
)

func logRolloutProgress(
	search *TreeSearch,
	completed int,
	elapsed time.Duration,
) {
	bestScore := search.bestScore

	if isInf(bestScore) {
		log.TuneLog(
			"mcts rollouts %d/%d (no persistable best yet, %s)",
			completed,
			search.iterations,
			elapsed,
		)

		return
	}

	log.TuneLog(
		"mcts rollouts %d/%d best realized score %.6f (%s)",
		completed,
		search.iterations,
		bestScore,
		elapsed,
	)
}

func isInf(value float64) bool {
	return math.IsInf(value, 0)
}
