package leadlag

import (
	"math"
	"slices"

	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
)

/*
summarize reduces the focal symbol's defined pair readings to robust
cross-peer statistics, so the output has a fixed set of keys however many
peers exist. Each reading is the peer (reference) path searched against the
focal (measured) path, so a negative best lag means the focal path precedes
the peer. Gains are covariance-score gains (|score at the selected lag| -
|score at zero lag|) over the readings whose zero-lag score is defined; they
are absent when no reading has one. With no defined reading there is no
evidence and summarize returns no keys at all.
*/
func summarize(readings []nmcorrelation.LeadLagReading) map[string]float64 {
	if len(readings) == 0 {
		return nil
	}

	lags := make([]float64, 0, len(readings))
	gains := make([]float64, 0, len(readings))
	gainSum := 0.0
	led := 0.0

	for _, reading := range readings {
		lags = append(lags, reading.X)

		if reading.GainDefined {
			gains = append(gains, reading.AbsoluteGain)
			gainSum += reading.AbsoluteGain
		}

		if reading.Leads && reading.X < 0 {
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
		summary["covariance_score_gain_mean"] = gainSum / float64(len(gains))
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
