package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/system"
)

/*
TimingStage audits market clock synchronization, feed sequencing, and ingest latency:
 1. Exchange-to-local clock drift. A packet received before the exchange timestamp is skew.
    Late packets are counted; the stream fails only when late packets are a larger share
    of arrivals than the significance allowance.
 2. Ingress monotonicity (sequence inversions where venue time moves backwards).
 3. Strict sequence index monotonicity within each stream.
 4. Measurement timestamp validity (non-zero venue and system times).
*/
type TimingStage struct {
	*runtime.System
	ch              chan *data.Value
	violations      []*Violation
	lastAtByStream  map[string]time.Time
	lastSeqByStream map[string]uint64
	driftsByStream  map[string]*statistic.Moments
	driftLastAt     map[string]time.Time
	driftSeenByTick map[string]bool
	flaggedDrift    map[string]bool
	latency         map[string]*latencyLedger
	totalChecked    int
	inversionCount  int
	skewCount       int
	spikeCount      int
	excessCount     int
}

/*
latencyLedger counts packets that were late compared with that stream's own
usual delay. A single late packet is not a broken clock. The allowance is a
share of packets (Chebyshev: at most `significance` of them may sit that far
out). The stream is a violation only when enough packets have been seen and
the share of late ones is above that allowance.
*/
type latencyLedger struct {
	eligible int
	spikes   int
}

func NewTimingStage(ctx context.Context) *TimingStage {
	return &TimingStage{
		System:          runtime.NewSystem(ctx, "audit:timing"),
		ch:              make(chan *data.Value, 64),
		violations:      make([]*Violation, 0),
		lastAtByStream:  make(map[string]time.Time),
		lastSeqByStream: make(map[string]uint64),
		driftsByStream:  make(map[string]*statistic.Moments),
		driftLastAt:     make(map[string]time.Time),
		driftSeenByTick: make(map[string]bool),
		flaggedDrift:    make(map[string]bool),
		latency:         make(map[string]*latencyLedger),
	}
}

func (stage *TimingStage) Channel() chan *data.Value {
	return stage.ch
}

func venueTime(measurement *data.Value) time.Time {
	if measurement == nil {
		return time.Time{}
	}
	metrics := data.Metrics(measurement)
	if at, ok := metrics["at"]; ok && at > 0 {
		return time.Unix(0, int64(at))
	}
	return time.Time{}
}

func (stage *TimingStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
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

		stage.totalChecked++

		stage.validateTimestamps(measurement)
		stage.validateSequencing(measurement)
		stage.validateDrift(measurement)
	}

	stage.auditLatencyRates()

	if stage.totalChecked == 0 {
		stage.violations = append(stage.violations, insufficient("timing", "no trades observed"))
	}

	return nil
}

/*
validateTimestamps ensures every measurement has valid, non-zero timestamps.
Venue time (At) tells us when the event occurred on the exchange.
Local time (Timestamp) tells us when our system received and recorded it.
Without both, event ordering and latency calculations are impossible.
*/
func (stage *TimingStage) validateTimestamps(measurement *data.Value) {
	vt := venueTime(measurement)
	if vt.IsZero() {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"timing.zero_venue_time",
			"Measurement venue timestamp At is zero",
			measurement,
			nil,
		))
	}

	if measurement.Timestamp <= 0 {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
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
func (stage *TimingStage) validateSequencing(measurement *data.Value) {
	stream := measurement.Origin + "|" + measurement.Scope
	vt := venueTime(measurement)

	if !vt.IsZero() {
		lastAt, exists := stage.lastAtByStream[stream]

		if exists && vt.Before(lastAt) {
			stage.inversionCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"timing.sequence_inversion",
				fmt.Sprintf("Venue time inverted: %s before last %s in stream %s", vt, lastAt, stream),
				measurement,
				nil,
			))
		}

		stage.lastAtByStream[stream] = vt
	}

	if measurement.SeqIdx > 0 {
		lastSeq, exists := stage.lastSeqByStream[stream]

		if exists && measurement.SeqIdx <= lastSeq {
			stage.inversionCount++
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
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
validateDrift tracks how long a packet took to arrive.
 1. Clock skew: local receive time minus the exchange time must not be negative.
    An event cannot be received before the exchange produced it.
 2. Late packets are counted per stream. Chebyshev's inequality says that at
    most `significance` of them can sit further than 1/sqrt(significance)
    standard deviations from the stream's own average. One late packet does
    not break that statement. auditLatencyRates reports the stream only when
    the share of late packets is higher than the allowance.
*/
func (stage *TimingStage) validateDrift(measurement *data.Value) {
	vt := venueTime(measurement)
	if vt.IsZero() || measurement.Timestamp <= 0 {
		return
	}

	if measurement.Tick > 0 {
		tickKey := fmt.Sprintf("%s|%d", measurement.Scope, measurement.Tick)

		if stage.driftSeenByTick[tickKey] {
			return
		}

		stage.driftSeenByTick[tickKey] = true
	}

	// Calculate latency: local ingest timestamp minus exchange event timestamp.
	stream := measurement.Scope
	driftNs := int64(measurement.Timestamp) - vt.UnixNano()
	driftMs := float64(driftNs) / 1e6

	// Physical causality: an event cannot be received before it was produced.
	// Latch clock skew once per stream to avoid redundant violations.
	if driftNs < 0 && !stage.flaggedDrift[stream] {
		stage.flaggedDrift[stream] = true
		stage.skewCount++
		stage.violations = append(stage.violations, NewViolation(
			measurement.Scope,
			"timing.negative_drift",
			fmt.Sprintf("Negative latency detected (clock skew: %.2f ms) in stream %s", driftMs, stream),
			measurement,
			nil,
		))
	}

	// Only update Welford drift moments when venue time advances.
	// Multi-fill bursts sharing a single exchange timestamp collapse variance to zero.
	lastAt, hasLastAt := stage.driftLastAt[stream]

	if hasLastAt && vt.Equal(lastAt) {
		return
	}

	stage.driftLastAt[stream] = vt

	moments, exists := stage.driftsByStream[stream]

	if !exists {
		moments = &statistic.Moments{}
		stage.driftsByStream[stream] = moments
	}

	// The first three arrivals only establish the stream's usual delay.
	if moments.Count >= 3 {
		stage.noteLatency(stream, driftMs, moments)
	}

	// Update running Welford moments with the new observation.
	moments.Update(driftMs)
}

func (stage *TimingStage) noteLatency(stream string, driftMs float64, moments *statistic.Moments) {
	variance := moments.M2 / (moments.Count - 1)
	dispersion := math.Sqrt(variance)
	cfg := system.Cfg.Audit
	bound := math.Inf(1)

	if cfg.Significance > 0 {
		bound = 1.0 / math.Sqrt(cfg.Significance)
	}

	residual := math.Abs(driftMs - moments.Mean)
	late := false

	if dispersion > cfg.LatencyDispersionFloorMs && residual > bound*dispersion {
		late = true
	}

	if dispersion <= cfg.LatencyDispersionFloorMs && residual > cfg.LatencySpikeFloorMs {
		late = true
	}

	ledger, exists := stage.latency[stream]

	if !exists {
		ledger = &latencyLedger{}
		stage.latency[stream] = ledger
	}

	ledger.eligible++

	if late {
		ledger.spikes++
		stage.spikeCount++
	}
}

/*
auditLatencyRates fails a stream only when late packets are a larger share of
its arrivals than the significance allowance. Streams with too few packets to
host that share are reported in the spike count and are not failed: one slow
packet cannot exceed a 5% allowance until 20 packets have been compared.
*/
func (stage *TimingStage) auditLatencyRates() {
	cfg := system.Cfg.Audit

	if cfg.Significance <= 0 || cfg.Significance >= 1 {
		return
	}

	minimum := int(math.Ceil(1 / cfg.Significance))
	streams := make([]string, 0, len(stage.latency))

	for stream := range stage.latency {
		streams = append(streams, stream)
	}

	slices.Sort(streams)

	for _, stream := range streams {
		ledger := stage.latency[stream]

		if ledger.eligible < minimum || ledger.spikes == 0 {
			continue
		}

		rate := float64(ledger.spikes) / float64(ledger.eligible)

		if rate <= cfg.Significance {
			continue
		}

		stage.excessCount++
		stage.violations = append(stage.violations, NewViolation(
			stream,
			"timing.latency_excess",
			fmt.Sprintf(
				"Delay on %s is unsteady: %.1f%% of %d packets were far from its usual delay, above the %.0f%% allowance",
				stream, rate*100, ledger.eligible, cfg.Significance*100,
			),
			nil,
			nil,
		))
	}
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
		"passed":          len(stage.violations) == 0,
		"total_checked":   stage.totalChecked,
		"breach_count":    len(stage.violations),
		"inversion_count": stage.inversionCount,
		"skew_count":      stage.skewCount,
		"spike_count":     stage.spikeCount,
		"excess_count":    stage.excessCount,
		"drifts":          drifts,
	}
}
