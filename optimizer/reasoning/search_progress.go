package reasoning

import (
	"fmt"
	"time"
)

/*
SearchProgress reports one phase of a reasoning forest search for stderr logging.
*/
type SearchProgress struct {
	Phase         string
	RowCount      int
	CategoryCount int
	SeedCount     int
	Round         int
	MaxRounds     int
	MaxNodes      int
	MinRoundTrips int
	Workers       int
	Evaluated     int
	RoundAdded    int
	BeamSize      int
	BestScore     float64
	BestReturn    float64
	BestTrades    int
	Stagnation    int
	Patience      int
	Elapsed       time.Duration
}

/*
Message formats one progress line for TuneLog.
*/
func (progress SearchProgress) Message() string {
	switch progress.Phase {
	case "config":
		return fmt.Sprintf(
			"search config: beam=%d max_rounds=%d max_nodes=%d patience=%d min_round_trips=%d workers=%d",
			progress.BeamSize,
			progress.MaxRounds,
			progress.MaxNodes,
			progress.Patience,
			progress.MinRoundTrips,
			progress.Workers,
		)
	case "vocabulary":
		return fmt.Sprintf(
			"derived vocabulary: %d signal categories from %d rows",
			progress.CategoryCount,
			progress.RowCount,
		)
	case "precompile_start":
		return fmt.Sprintf("precompiling replay tape (%d rows)...", progress.RowCount)
	case "precompile_done":
		return fmt.Sprintf(
			"precompiled replay tape: %d ticks, %d categories (%.1fs)",
			progress.RowCount,
			progress.CategoryCount,
			progress.Elapsed.Seconds(),
		)
	case "seeds_start":
		return fmt.Sprintf("scoring %d seed forests...", progress.SeedCount)
	case "seeds_done":
		return fmt.Sprintf(
			"seeds complete: evaluated=%d best_score=%.6f best_return=%.6f best_trades=%d (%.1fs)",
			progress.Evaluated,
			progress.BestScore,
			progress.BestReturn,
			progress.BestTrades,
			progress.Elapsed.Seconds(),
		)
	case "round":
		return fmt.Sprintf(
			"round %d/%d: evaluated=%d (+%d) beam=%d best_score=%.6f best_return=%.6f best_trades=%d stagnation=%d/%d (%.1fs)",
			progress.Round,
			progress.MaxRounds,
			progress.Evaluated,
			progress.RoundAdded,
			progress.BeamSize,
			progress.BestScore,
			progress.BestReturn,
			progress.BestTrades,
			progress.Stagnation,
			progress.Patience,
			progress.Elapsed.Seconds(),
		)
	case "done":
		return fmt.Sprintf(
			"search complete: evaluated=%d best_score=%.6f best_return=%.6f best_trades=%d (%.1fs)",
			progress.Evaluated,
			progress.BestScore,
			progress.BestReturn,
			progress.BestTrades,
			progress.Elapsed.Seconds(),
		)
	default:
		return progress.Phase
	}
}

func (config SearchConfig) reportProgress(progress SearchProgress) {
	if config.OnProgress == nil {
		return
	}

	config.OnProgress(progress)
}
