package trader

import (
	"sync"
	"sync/atomic"
	"time"
)

/*
runStats is the cumulative counter set the trader emits as one
"run_stats" JSON line every statsInterval. The point is to make a run
analyzable offline: tail the log, jq the run_stats lines, and you can
plot throughput, gate hit rates, slot decisions, and PnL trajectory
without having to reconstruct counts from the per-event lines.

Every counter is monotonic for the life of the process; window deltas
are computed by subtracting two consecutive snapshots. Keeping the
counters cumulative removes the need for any reset-window
synchronization and lets analysis bin into arbitrary intervals after
the fact.

Concurrency: every field is atomic. The per-key maps (skipReasons,
sourceMeasurements, sourcePredictions, sourceFills) live behind a
sync.RWMutex; we read them under RLock for the snapshot and write
under Lock for new keys, which happens at most once per key over a
run.
*/
type runStats struct {
	startedAt time.Time

	measurementsIngested atomic.Int64
	measurementsSkipped  atomic.Int64

	perspectivesAccumulated atomic.Int64
	perspectivesReady       atomic.Int64
	perspectivesNotReady    atomic.Int64

	predictionsOpened   atomic.Int64
	predictionsSettled  atomic.Int64
	predictionsExpired  atomic.Int64
	feedbackApplied     atomic.Int64
	feedbackErrorSumX1K atomic.Int64 // |error| * 1000 accumulated for mean

	entriesEvaluated atomic.Int64
	entriesFilled    atomic.Int64
	entriesSkipped   atomic.Int64
	entriesErrored   atomic.Int64

	exitsEvaluated atomic.Int64
	exitsFilled    atomic.Int64
	exitsSkipped   atomic.Int64
	exitsErrored   atomic.Int64

	fillsApplied atomic.Int64
	fillsDeduped atomic.Int64

	orderAcksSuccess atomic.Int64
	orderAcksFailure atomic.Int64

	preTradeGateHits  atomic.Int64
	preTradeGateAllow atomic.Int64

	uiFramesSent     atomic.Int64
	uiFramesDropped  atomic.Int64
	uiFramesFiltered atomic.Int64

	leadlagThrottleHits atomic.Int64
	leadlagRecomputes   atomic.Int64

	wsConnects       atomic.Int64
	wsReconnects     atomic.Int64
	tokenRefreshes   atomic.Int64
	tokenRefreshFail atomic.Int64

	mapsMu             sync.RWMutex
	skipReasons        map[string]*atomic.Int64
	sourceMeasurements map[string]*atomic.Int64
	sourcePredictions  map[string]*atomic.Int64
	sourceFills        map[string]*atomic.Int64
}

func newRunStats() *runStats {
	return &runStats{
		startedAt:          time.Now(),
		skipReasons:        make(map[string]*atomic.Int64),
		sourceMeasurements: make(map[string]*atomic.Int64),
		sourcePredictions:  make(map[string]*atomic.Int64),
		sourceFills:        make(map[string]*atomic.Int64),
	}
}

func (stats *runStats) counter(table map[string]*atomic.Int64, key string) *atomic.Int64 {
	if stats == nil || key == "" {
		return nil
	}

	stats.mapsMu.RLock()
	counter := table[key]
	stats.mapsMu.RUnlock()

	if counter != nil {
		return counter
	}

	stats.mapsMu.Lock()
	defer stats.mapsMu.Unlock()

	if counter = table[key]; counter != nil {
		return counter
	}

	counter = &atomic.Int64{}
	table[key] = counter

	return counter
}

func (stats *runStats) RecordMeasurement(source string) {
	if stats == nil {
		return
	}

	stats.measurementsIngested.Add(1)

	if c := stats.counter(stats.sourceMeasurements, source); c != nil {
		c.Add(1)
	}
}

func (stats *runStats) RecordSkip(reason string) {
	if stats == nil {
		return
	}

	if c := stats.counter(stats.skipReasons, reason); c != nil {
		c.Add(1)
	}
}

func (stats *runStats) RecordPrediction(source string) {
	if stats == nil {
		return
	}

	stats.predictionsOpened.Add(1)

	if c := stats.counter(stats.sourcePredictions, source); c != nil {
		c.Add(1)
	}
}

func (stats *runStats) RecordFill(source, side string) {
	if stats == nil {
		return
	}

	key := source + ":" + side

	if c := stats.counter(stats.sourceFills, key); c != nil {
		c.Add(1)
	}
}

/*
Snapshot returns a JSON-friendly view of the counters. Maps are copied
so the returned value can be marshalled while other goroutines continue
to update the source counters.
*/
func (stats *runStats) Snapshot() map[string]any {
	if stats == nil {
		return nil
	}

	stats.mapsMu.RLock()
	defer stats.mapsMu.RUnlock()

	skipReasons := copyCounters(stats.skipReasons)
	sourceMeasurements := copyCounters(stats.sourceMeasurements)
	sourcePredictions := copyCounters(stats.sourcePredictions)
	sourceFills := copyCounters(stats.sourceFills)

	feedbackCount := stats.feedbackApplied.Load()
	meanAbsError := 0.0

	if feedbackCount > 0 {
		meanAbsError = float64(stats.feedbackErrorSumX1K.Load()) / 1000.0 / float64(feedbackCount)
	}

	return map[string]any{
		"uptime_sec":               time.Since(stats.startedAt).Seconds(),
		"measurements_ingested":    stats.measurementsIngested.Load(),
		"measurements_skipped":     stats.measurementsSkipped.Load(),
		"perspectives_accumulated": stats.perspectivesAccumulated.Load(),
		"perspectives_ready":       stats.perspectivesReady.Load(),
		"perspectives_not_ready":   stats.perspectivesNotReady.Load(),
		"predictions_opened":       stats.predictionsOpened.Load(),
		"predictions_settled":      stats.predictionsSettled.Load(),
		"predictions_expired":      stats.predictionsExpired.Load(),
		"feedback_applied":         feedbackCount,
		"feedback_mean_abs_error":  meanAbsError,
		"entries_evaluated":        stats.entriesEvaluated.Load(),
		"entries_filled":           stats.entriesFilled.Load(),
		"entries_skipped":          stats.entriesSkipped.Load(),
		"entries_errored":          stats.entriesErrored.Load(),
		"exits_evaluated":          stats.exitsEvaluated.Load(),
		"exits_filled":             stats.exitsFilled.Load(),
		"exits_skipped":            stats.exitsSkipped.Load(),
		"exits_errored":            stats.exitsErrored.Load(),
		"fills_applied":            stats.fillsApplied.Load(),
		"fills_deduped":            stats.fillsDeduped.Load(),
		"order_acks_success":       stats.orderAcksSuccess.Load(),
		"order_acks_failure":       stats.orderAcksFailure.Load(),
		"pre_trade_gate_hits":      stats.preTradeGateHits.Load(),
		"pre_trade_gate_allow":     stats.preTradeGateAllow.Load(),
		"ui_frames_sent":           stats.uiFramesSent.Load(),
		"ui_frames_dropped":        stats.uiFramesDropped.Load(),
		"ui_frames_filtered":       stats.uiFramesFiltered.Load(),
		"leadlag_throttle_hits":    stats.leadlagThrottleHits.Load(),
		"leadlag_recomputes":       stats.leadlagRecomputes.Load(),
		"ws_connects":              stats.wsConnects.Load(),
		"ws_reconnects":            stats.wsReconnects.Load(),
		"token_refreshes":          stats.tokenRefreshes.Load(),
		"token_refresh_failures":   stats.tokenRefreshFail.Load(),
		"skip_reasons":             skipReasons,
		"source_measurements":      sourceMeasurements,
		"source_predictions":       sourcePredictions,
		"source_fills":             sourceFills,
	}
}

func copyCounters(source map[string]*atomic.Int64) map[string]int64 {
	out := make(map[string]int64, len(source))

	for key, counter := range source {
		out[key] = counter.Load()
	}

	return out
}

// stats is a package-level singleton so every audit() call can record
// without threading a stats pointer through every helper. Production
// code instantiates exactly one trader; tests that need isolation can
// call resetStatsForTest.
var stats = newRunStats()
