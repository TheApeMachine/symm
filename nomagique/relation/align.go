package relation

import "time"

/*
SeriesView is one predictor series viewed as a resident ring with its own
explicit alignment lag.
*/
type SeriesView struct {
	// History is the read-locked resident ring view (chronological).
	History RingView
	// Lag is the as-of lag: the newest observation used is the newest one
	// available no later than target time minus Lag.
	Lag time.Duration
}

/*
AlignedRow is one target observation aligned with its lagged predictors.
*/
type AlignedRow struct {
	Target     Observation
	Predictors []Observation
}

/*
AlignViews aligns target observations with lagged predictor series, all read
in place from resident rings. For a target observation at time t and a
predictor series with lag τ, the aligned predictor is the newest observation
available no later than t - τ. Future observations never enter a row. Only
target observations with every predictor aligned are retained, in
chronological target order.

Preconditions: the target ring and every series ring must be in
non-decreasing chronological order (resident rings are by construction); the
per-series cursor alignment scans each series once across the whole call,
and a later call on the same series (or a continued scan) requires
non-decreasing cutoffs.
*/
func AlignViews(targets RingView, series []SeriesView) []AlignedRow {
	if targets.Len() == 0 || len(series) == 0 {
		return nil
	}

	cursors := make([]int, len(series))

	for index := range cursors {
		cursors[index] = -1
	}

	rows := make([]AlignedRow, 0, targets.Len())

	for targetIndex := 0; targetIndex < targets.Len(); targetIndex++ {
		target := targets.At(targetIndex)
		predictors := make([]Observation, len(series))
		complete := true

		for index, predictorSeries := range series {
			cutoff := target.At.Add(-predictorSeries.Lag)
			predictor, found := newestAtOrBeforeView(predictorSeries.History, &cursors[index], cutoff)

			if !found {
				complete = false
				break
			}

			predictors[index] = predictor
		}

		if !complete {
			continue
		}

		rows = append(rows, AlignedRow{
			Target:     target,
			Predictors: predictors,
		})
	}

	return rows
}

/*
newestAtOrBeforeView returns the newest observation in a resident ring view
at or before cutoff. The cursor remains positioned on the last matched
observation (a negative value means no match has ever been recorded):
repeated calls with non-decreasing cutoffs re-scan only entries after the
previous match, and a call whose cutoff reaches no newer entry returns the
previously matched observation. When no observation has ever matched, the
result is not-found. The precondition is that the ring is chronological and
cutoffs are non-decreasing across calls, which the alignment paths
guarantee.
*/
func newestAtOrBeforeView(history RingView, cursor *int, cutoff time.Time) (Observation, bool) {
	best := -1
	start := 0

	if cursor != nil && *cursor >= 0 {
		start = *cursor
	}

	for index := start; index < history.Len() && !history.TimeAt(index).After(cutoff); index++ {
		best = index
	}

	if best >= 0 {
		if cursor != nil {
			*cursor = best
		}

		return history.At(best), true
	}

	if cursor == nil || *cursor < 0 {
		return Observation{}, false
	}

	return history.At(*cursor), true
}
