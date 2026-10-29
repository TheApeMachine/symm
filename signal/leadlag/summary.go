package leadlag

import (
	"math"
	"slices"
)

/*
leadLagReading is one peer's cross-lag result: the lag between the peer
(reference) and the focal (measured) path, the correlation at that lag, and the
gain over the zero-lag baseline.
*/
type leadLagReading struct {
	lagSeconds  float64
	corr        float64
	leads       bool
	gain        float64
	gainDefined bool
}

/*
summarize reduces the focal symbol's defined pair readings to robust
cross-peer statistics, so the output has a fixed set of keys however many
peers exist. Each reading is the peer (reference) path searched against the
focal (measured) path, so a negative best lag means the focal path precedes
the peer. Gains are correlation magnitudes at the selected lag over the
readings whose lag is defined; they are absent when no reading has one. With
no defined reading there is no evidence and summarize returns no keys at all.
*/
func summarize(readings []leadLagReading) map[string]float64 {
	if len(readings) == 0 {
		return nil
	}

	lags := make([]float64, 0, len(readings))
	gains := make([]float64, 0, len(readings))
	led := 0.0

	for _, reading := range readings {
		lags = append(lags, reading.lagSeconds)

		if reading.gainDefined {
			gains = append(gains, reading.gain)
		}

		if reading.leads && reading.lagSeconds < 0 {
			led++
		}
	}

	peers := float64(len(readings))
	lagMedian := median(lags)
	deviations := make([]float64, 0, len(lags))

	for _, lag := range lags {
		deviations = append(deviations, math.Abs(lag-lagMedian))
	}

	summary := map[string]float64{
		"defined_peer_count":      peers,
		"best_lag_seconds_median": lagMedian,
		"best_lag_seconds_mad":    median(deviations),
		"led_peer_share":          led / peers,
	}

	if len(gains) > 0 {
		summary["covariance_score_gain_median"] = median(gains)
	}

	return summary
}

/*
median is the middle order statistic of values, or the mean of the two middle
ones when the count is even. values must be non-empty; it is sorted in place.
*/
func median(values []float64) float64 {
	slices.Sort(values)
	middle := len(values) / 2

	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}
