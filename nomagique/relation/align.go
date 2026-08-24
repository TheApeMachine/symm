package relation

import "time"

/*
LaggedSeries is one predictor series with its own explicit alignment lag.
*/
type LaggedSeries struct {
	// Observations is the chronological series.
	Observations []Observation
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
AlignLagged aligns target observations with lagged predictor series. For a
target observation at time t and a predictor series with lag τ, the aligned
predictor is the newest observation available no later than t - τ. Future
observations never enter a row. Only target observations with every
predictor aligned are retained, in chronological target order.

Alignment is generic: it is used by the Relation estimator (TargetPast +
ControlsPast + SourcePast) and by the causal market transition model (self
history + schema-authorized parents), so event-time causality is implemented
once.
*/
func AlignLagged(targets []Observation, series []LaggedSeries) []AlignedRow {
	if len(targets) == 0 || len(series) == 0 {
		return nil
	}

	cursors := make([]int, len(series))
	rows := make([]AlignedRow, 0, len(targets))

	for _, target := range targets {
		predictors := make([]Observation, len(series))
		complete := true

		for index, predictorSeries := range series {
			cutoff := target.At.Add(-predictorSeries.Lag)
			predictor, found := newestAtOrBefore(predictorSeries.Observations, &cursors[index], cutoff)

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
