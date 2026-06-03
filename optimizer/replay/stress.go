package replay

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
executionStressMultiplier scales replay slippage when the tick snapshot shows
high-vorticity flow or liquidity/contagion stress categories.
*/
func executionStressMultiplier(
	snapshots []perspectives.Measurement,
) float64 {
	if len(snapshots) == 0 {
		return 1
	}

	stressSNR := 0.0
	stressReadings := 0

	for _, measurement := range snapshots {
		if !executionStressCategory(measurement.Category) {
			continue
		}

		stressReadings++

		if measurement.SNR > stressSNR {
			stressSNR = measurement.SNR
		}
	}

	if stressReadings == 0 || stressSNR <= 0 {
		return 1
	}

	coverage := float64(stressReadings) / float64(len(snapshots))
	strength := stressSNR / (stressSNR + 1)

	return 1 + coverage*strength
}

func executionStressCategory(category perspectives.CategoryType) bool {
	switch category {
	case perspectives.CategoryTurbulent,
		perspectives.CategoryFrenzy,
		perspectives.CategoryLiquidityVacuum,
		perspectives.CategoryLiquidityShock,
		perspectives.CategorySystemicHerd,
		perspectives.CategoryMechanicalCollapse,
		perspectives.CategoryDivergentStress,
		perspectives.CategorySystemicSlump:
		return true
	default:
		return false
	}
}

func executionSlippagePct(
	costs ReplayCosts,
	spreadBPS float64,
	snapshots []perspectives.Measurement,
) float64 {
	base := halfSpreadSlippagePct(costs, spreadBPS)

	return base * executionStressMultiplier(snapshots)
}

func deriveExecutionLatency(rows []perspectives.Measurement) time.Duration {
	medianInterval := medianMeasurementInterval(rows)

	if medianInterval <= 0 {
		return 0
	}

	latency := time.Duration(float64(medianInterval) * 3)

	minLatency := 50 * time.Millisecond
	maxLatency := 200 * time.Millisecond

	if latency < minLatency {
		latency = minLatency
	}

	if latency > maxLatency {
		latency = maxLatency
	}

	return latency
}

func deriveExecutionLatencyFromTape(tape ReplayTape) time.Duration {
	rows := make([]perspectives.Measurement, 0, tape.Len())

	for _, tick := range tape.Ticks {
		rows = append(rows, tick.Row)
	}

	return deriveExecutionLatency(rows)
}

func medianMeasurementInterval(rows []perspectives.Measurement) time.Duration {
	if len(rows) < 2 {
		return 0
	}

	intervals := make([]time.Duration, 0, len(rows)-1)
	lastAt := rows[0].At

	for index := 1; index < len(rows); index++ {
		at := rows[index].At

		if lastAt.IsZero() || at.IsZero() || !at.After(lastAt) {
			continue
		}

		intervals = append(intervals, at.Sub(lastAt))
		lastAt = at
	}

	if len(intervals) == 0 {
		return 0
	}

	return percentileDuration(intervals, 0.5)
}

func percentileDuration(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]time.Duration(nil), values...)
	sortDurations(sorted)

	index := int(math.Floor(float64(len(sorted)-1) * quantile))

	if index < 0 {
		index = 0
	}

	return sorted[index]
}

func sortDurations(values []time.Duration) {
	for left := 1; left < len(values); left++ {
		cursor := values[left]
		walk := left

		for walk > 0 && values[walk-1] > cursor {
			values[walk] = values[walk-1]
			walk--
		}

		values[walk] = cursor
	}
}

func executionLatencyTicks(
	latency time.Duration,
	medianInterval time.Duration,
) int {
	if latency <= 0 {
		return 0
	}

	if medianInterval <= 0 {
		return 1
	}

	ticks := int(math.Ceil(float64(latency) / float64(medianInterval)))

	if ticks < 1 {
		return 1
	}

	return ticks
}

func (costs ReplayCosts) effectiveExecutionLatency(
	rows []perspectives.Measurement,
	tape ReplayTape,
) time.Duration {
	if costs.ExecutionLatency > 0 {
		return costs.ExecutionLatency
	}

	if !costs.ExecutionStressEnabled {
		return 0
	}

	if replayLatency := public.ReplayExchangeLatency(); replayLatency > 0 {
		return replayLatency
	}

	if len(rows) > 0 {
		return deriveExecutionLatency(rows)
	}

	return deriveExecutionLatencyFromTape(tape)
}

func replayExecutionLatencyFromViper() time.Duration {
	config := viper.GetViper()
	latencyMS := config.GetFloat64("trading.replay.execution_latency_ms")

	if latencyMS <= 0 {
		return 0
	}

	return time.Duration(latencyMS * float64(time.Millisecond))
}

func replayExecutionStressEnabledFromViper() bool {
	return viper.GetViper().GetBool("trading.replay.execution_stress_enabled")
}
