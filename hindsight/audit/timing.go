package audit

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
AnalyzeTiming audits market clock synchronization and feed sequencing:
 1. Exchange-to-local clock drift (Timestamp - At) and jitter spikes.
 2. Ingress monotonicity (detects feed timestamp inversions).
*/
func AnalyzeTiming(measurements []*data.Measurement, thresholds Thresholds) Stage0Timing {
	if len(measurements) < 10 {
		return Stage0Timing{
			TotalChecked: len(measurements),
			SummaryText:  "Timing & Synchronization: INSUFFICIENT_DATA (fewer than 10 measurements)",
			Status:       "INSUFFICIENT_DATA",
			Passed:       false,
		}
	}

	drifts := make([]float64, 0, len(measurements))
	// Event time must be monotonic within one producer's stream for one
	// symbol. Sources stamp different events (a book update and a trade), so
	// interleaving two sources' times is not an inversion.
	lastAtByStream := make(map[string]time.Time)
	inversions := 0

	for _, meas := range measurements {
		if meas == nil {
			continue
		}

		if !meas.At.IsZero() && meas.Timestamp > 0 {
			atNs := meas.At.UnixNano()
			driftNs := meas.Timestamp - atNs
			driftMs := float64(driftNs) / 1e6
			drifts = append(drifts, driftMs)
		}

		if !meas.At.IsZero() && meas.Label != "" {
			stream := meas.Source + "|" + meas.Label
			lastAt, exists := lastAtByStream[stream]

			if exists && meas.At.Before(lastAt) {
				inversions++
			}

			lastAtByStream[stream] = meas.At
		}
	}

	if len(drifts) == 0 {
		return Stage0Timing{
			TotalChecked: len(measurements),
			SummaryText:  "Timing & Synchronization: INSUFFICIENT_DATA (no paired local/venue timestamps)",
			Status:       "INSUFFICIENT_DATA",
			Passed:       false,
		}
	}

	sort.Float64s(drifts)
	sumDrift := 0.0
	maxDrift := drifts[len(drifts)-1]

	for _, d := range drifts {
		sumDrift += d
	}

	meanDrift := sumDrift / float64(len(drifts))
	p95Drift := empiricalQuantile(drifts, 0.95)
	spikeThreshold := thresholds.SpikeLatencyMs
	spikes := 0

	for _, d := range drifts {
		if math.Abs(d) > spikeThreshold {
			spikes++
		}
	}

	passed := inversions == 0 && float64(spikes)/float64(len(drifts)) <= thresholds.SpikeFraction
	verdict := VerdictBreach

	if passed {
		verdict = VerdictValid
	}

	summaryText := fmt.Sprintf(
		"Timing & Sync: %d observations. Mean clock drift = %.1fms (p95 = %.1fms, max = %.1fms). Latency spikes (>%.0fms): %d. Sequence inversions: %d.",
		len(drifts), meanDrift, p95Drift, maxDrift, spikeThreshold, spikes, inversions,
	)

	return Stage0Timing{
		TotalChecked:       len(drifts),
		MeanDriftMs:        meanDrift,
		MaxDriftMs:         maxDrift,
		P95DriftMs:         p95Drift,
		LatencySpikes:      spikes,
		SequenceInversions: inversions,
		SummaryText:        summaryText,
		Status:             verdict,
		Passed:             passed,
	}
}
