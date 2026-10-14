package audit

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/system"
)

/*
TimingStage audits market clock synchronization, feed sequencing, and ingest latency:
 1. Exchange-to-local clock drift and statistically significant jitter spikes.
 2. Ingress monotonicity (sequence inversions where venue time moves backwards).
 3. Strict sequence index monotonicity within each stream.
 4. Measurement timestamp validity (non-zero venue and system times).
*/
type TimingStage struct {
	*runtime.System
	ch              chan *data.Measurement
	violations      []*Violation
	lastAtByStream  map[string]time.Time
	lastSeqByStream map[string]int64
	driftsByStream  map[string]*statistic.Moments
	totalChecked    int
}

func NewTimingStage(ctx context.Context) *TimingStage {
	return &TimingStage{
		System:          runtime.NewSystem(ctx, "audit:timing"),
		ch:              make(chan *data.Measurement, 64),
		violations:      make([]*Violation, 0),
		lastAtByStream:  make(map[string]time.Time),
		lastSeqByStream: make(map[string]int64),
		driftsByStream:  make(map[string]*statistic.Moments),
	}
}

func (stage *TimingStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *TimingStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		stage.totalChecked++

		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))

			continue
		}

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.validateTimestamps(measurement)
		stage.validateSequencing(measurement)
		stage.validateDrift(measurement)
	}

	return nil
}

/*
validateTimestamps ensures every measurement has valid, non-zero timestamps.
Venue time (At) tells us when the event occurred on the exchange.
Local time (Timestamp) tells us when our system received and recorded it.
Without both, event ordering and latency calculations are impossible.
*/
func (stage *TimingStage) validateTimestamps(measurement *data.Measurement) {
	if measurement.At.IsZero() {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Label,
			"timing.zero_venue_time",
			"Measurement venue timestamp At is zero",
			measurement,
			nil,
		))
	}

	if measurement.Timestamp <= 0 {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Label,
			"timing.zero_local_time",
			"Measurement local timestamp is zero or negative",
			measurement,
			nil,
		))
	}
}

/*
validateSequencing checks feed monotonicity within a single producer stream.
Market events from one source must always move forward in time (venue time cannot
travel backwards) and event sequence numbers (SeqIdx) must strictly increase.
Any drop or backwards jump indicates corrupted feeds or out-of-order delivery.
*/
func (stage *TimingStage) validateSequencing(measurement *data.Measurement) {
	stream := measurement.Source + "|" + measurement.Label

	if !measurement.At.IsZero() {
		lastAt, exists := stage.lastAtByStream[stream]

		if exists && measurement.At.Before(lastAt) {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"timing.sequence_inversion",
				fmt.Sprintf("Venue time inverted: %s before last %s in stream %s", measurement.At, lastAt, stream),
				measurement,
				nil,
			))
		}

		stage.lastAtByStream[stream] = measurement.At
	}

	if measurement.SeqIdx > 0 {
		lastSeq, exists := stage.lastSeqByStream[stream]

		if exists && measurement.SeqIdx <= lastSeq {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"timing.seq_idx_inversion",
				fmt.Sprintf("Sequence index non-increasing: %d <= %d in stream %s", measurement.SeqIdx, lastSeq, stream),
				measurement,
				nil,
			))
		}

		stage.lastSeqByStream[stream] = measurement.SeqIdx
	}
}

/*
validateDrift tracks transit latency and detects desynchronization or jitter spikes.
 1. Clock Skew: Local receive time minus exchange event time must not be negative.
    If local time is earlier than exchange time, our server clock is behind the exchange clock.
 2. Latency Jitter Spikes: Rather than guessing an arbitrary threshold like 200ms,
    we maintain running statistics (Welford's moments: average and spread) for each stream.
    Using Chebyshev's inequality, any observation that deviates from the average by more
    than 1/sqrt(alpha) standard deviations (e.g. ~4.47 sigma at 5% significance) is
    statistically an extreme outlier / network stall rather than routine noise.
*/
func (stage *TimingStage) validateDrift(measurement *data.Measurement) {
	if measurement.At.IsZero() || measurement.Timestamp <= 0 {
		return
	}

	// Calculate latency: local ingest timestamp minus exchange event timestamp.
	stream := measurement.Source + "|" + measurement.Label
	driftNs := measurement.Timestamp - measurement.At.UnixNano()
	driftMs := float64(driftNs) / 1e6

	// Physical causality: an event cannot be received before it was produced.
	if driftNs < 0 {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Label,
			"timing.negative_drift",
			fmt.Sprintf("Negative latency detected (clock skew: %.2f ms) in stream %s", driftMs, stream),
			measurement,
			nil,
		))
	}

	moments, exists := stage.driftsByStream[stream]

	if !exists {
		moments = &statistic.Moments{}
		stage.driftsByStream[stream] = moments
	}

	// Once we have at least 2 observations, we can calculate variance and dispersion (spread).
	if moments.Count > 1 {
		variance := moments.M2 / (moments.Count - 1)
		dispersion := math.Sqrt(variance)
		significance := system.Cfg.Audit.Significance

		if significance <= 0 || significance >= 1 {
			significance = 0.05
		}

		// Chebyshev bound: bound = 1 / sqrt(alpha).
		// For alpha = 0.05 (5%), bound is sqrt(20) ~ 4.47 standard deviations.
		bound := 1.0 / math.Sqrt(significance)
		residual := math.Abs(driftMs - moments.Mean)

		// Flag if latency deviation exceeds the statistical Chebyshev bound.
		if dispersion > 0 && residual > bound*dispersion {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"timing.latency_spike",
				fmt.Sprintf("Latency spike: drift %.2f ms deviates by %.2f sigma from mean %.2f ms (bound %.2f)",
					driftMs, residual/dispersion, moments.Mean, bound),
				measurement,
				nil,
			))
		}

		// If prior dispersion was zero (constant latency), any noticeable shift is a spike.
		if dispersion == 0 && residual > core.Resolution*math.Max(math.Abs(driftMs), math.Abs(moments.Mean)) {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"timing.latency_spike",
				fmt.Sprintf("Latency spike: drift %.2f ms deviates from zero-dispersion mean %.2f ms",
					driftMs, moments.Mean),
				measurement,
				nil,
			))
		}
	}

	// Update running Welford moments with the new observation.
	moments.Update(driftMs)
}

func (stage *TimingStage) Violations() []*Violation {
	return stage.violations
}

func (stage *TimingStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *TimingStage) Data() any {
	drifts := make(map[string]map[string]any)

	for stream, moments := range stage.driftsByStream {
		mean := 0.0
		variance := 0.0

		if moments.Count > 0 {
			mean = moments.Mean
		}

		if moments.Count > 1 {
			variance = moments.M2 / (moments.Count - 1)
		}

		drifts[stream] = map[string]any{
			"count":   moments.Count,
			"mean_ms": mean,
			"std_ms":  math.Sqrt(variance),
		}
	}

	return map[string]any{
		"passed":        len(stage.violations) == 0,
		"total_checked": stage.totalChecked,
		"breach_count":  len(stage.violations),
		"drifts":        drifts,
	}
}

