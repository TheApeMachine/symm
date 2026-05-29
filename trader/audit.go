package trader

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
)

/*
audit emits one JSON line per event and updates the run-stats counters
so the periodic run_stats roll-up reflects every event without each
emitter having to remember to bump a counter. The event name itself
drives which counter fires; the fields map is forwarded verbatim into
the JSON line.

Two outputs per call:

 1. errnie.Info — goes into the main runs/symm-*.log alongside every
    other Info line, useful for grepping with full caller context.

 2. runs/audit-*.jsonl — a sidecar with one bare JSON object per line,
    no errnie envelope. This is what `jq -c 'select(.event=="…")'`
    consumes. The path mirrors the main log's filename so an audit
    sidecar is always findable next to its run.

Both paths are best-effort; if either write fails, the other still
proceeds, and a single Errorf is emitted into errnie so analysis tools
can detect the gap.

Conventions baked into the dispatcher below:
  - "reason" in trade_*_skip lines is recorded into skipReasons so
    post-run jq can group "why did we not enter" by cause.
  - "source" in measurement_ingest / perspective_ready / fill_applied
    is split into per-source counters.
  - error counters cover the *_error suffixes from every emitter.

If you add a new audit event, add a case in recordStats too —
otherwise the event will still appear in the log line-by-line but
won't show up in the per-window roll-up that data analysis usually
starts from.
*/
func audit(event string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}

	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	recordStats(event, fields)

	payload, err := json.Marshal(fields)

	if err != nil {
		errnie.Error(err)
		return
	}

	errnie.Info(string(payload))
	publishAuditFrame(event, fields)
	writeAuditLine(payload)
}

// auditWriter owns the sidecar file. It is opened on first use so unit
// tests that never call audit() do not create empty files. Writes are
// serialized through writeMu so concurrent emitters don't interleave
// half-lines.
var (
	auditMu      sync.Mutex
	auditFile    *os.File
	auditPath    string
	auditOnce    sync.Once
	auditInitErr error
	auditSeq     atomic.Uint64

	auditBroadcastMu sync.RWMutex
	auditBroadcast   *qpool.BroadcastGroup
)

func setAuditBroadcast(group *qpool.BroadcastGroup) {
	auditBroadcastMu.Lock()
	auditBroadcast = group
	auditBroadcastMu.Unlock()
}

func clearAuditBroadcast(group *qpool.BroadcastGroup) {
	auditBroadcastMu.Lock()
	defer auditBroadcastMu.Unlock()

	if auditBroadcast == group {
		auditBroadcast = nil
	}
}

func publishAuditFrame(event string, fields map[string]any) {
	if !realtimeAuditEvent(event) {
		return
	}

	auditBroadcastMu.RLock()
	group := auditBroadcast
	auditBroadcastMu.RUnlock()

	if group == nil {
		return
	}

	frame := make(map[string]any, len(fields)+3)
	maps.Copy(frame, fields)
	frame["event"] = "audit"
	frame["audit_event"] = event
	frame["seq"] = auditSeq.Add(1)

	group.Send(&qpool.QValue[any]{Value: frame})
}

func realtimeAuditEvent(event string) bool {
	switch event {
	case "trade_entry_fill",
		"trade_entry_error",
		"trade_exit_fill",
		"trade_exit_error":
		return true
	default:
		return false
	}
}

func writeAuditLine(payload []byte) {
	auditOnce.Do(initAuditWriter)

	if auditInitErr != nil {
		return
	}

	auditMu.Lock()
	defer auditMu.Unlock()

	if _, err := auditFile.Write(payload); err != nil {
		errnie.Error(fmt.Errorf("audit write: %w", err))
		return
	}

	if _, err := auditFile.Write([]byte{'\n'}); err != nil {
		errnie.Error(fmt.Errorf("audit newline: %w", err))
	}
}

func initAuditWriter() {
	dir := config.System.LogDir

	if dir == "" {
		dir = "runs"
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		auditInitErr = err
		errnie.Error(fmt.Errorf("audit mkdir: %w", err))
		return
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	auditPath = filepath.Join(dir, fmt.Sprintf("audit-%s.jsonl", runID))

	file, err := os.OpenFile(auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)

	if err != nil {
		auditInitErr = err
		errnie.Error(fmt.Errorf("audit open: %w", err))
		return
	}

	auditFile = file
	errnie.Info(fmt.Sprintf("audit sidecar: %s", auditPath))
}

func recordStats(event string, fields map[string]any) {
	if stats == nil {
		return
	}

	switch event {
	case "measurement_ingest":
		stats.RecordMeasurement(stringField(fields, "source"))
	case "perspective_accumulate":
		stats.perspectivesAccumulated.Add(1)
	case "perspective_ready":
		stats.perspectivesReady.Add(1)
		stats.RecordPrediction(stringField(fields, "source"))
	case "perspective_not_ready":
		stats.perspectivesNotReady.Add(1)
	case "trade_entry_eval":
		stats.entriesEvaluated.Add(1)
	case "trade_entry_fill":
		stats.entriesFilled.Add(1)
		stats.RecordFill(stringField(fields, "source"), "buy")
	case "trade_entry_skip":
		stats.entriesSkipped.Add(1)
		stats.RecordSkip(stringField(fields, "reason"))
	case "trade_entry_error":
		stats.entriesErrored.Add(1)
	case "trade_exit_eval":
		stats.exitsEvaluated.Add(1)
	case "trade_exit_fill":
		stats.exitsFilled.Add(1)
		stats.RecordFill(stringField(fields, "reason"), "sell")
	case "trade_exit_skip":
		stats.exitsSkipped.Add(1)
		stats.RecordSkip("exit:" + stringField(fields, "reason"))
	case "trade_exit_error":
		stats.exitsErrored.Add(1)
	case "prediction_settled":
		stats.predictionsSettled.Add(1)
		stats.feedbackApplied.Add(1)

		if err, ok := fields["error"].(float64); ok {
			stats.feedbackErrorSumX1K.Add(int64(math.Abs(err) * 1000))
		}
	case "fill_applied":
		stats.fillsApplied.Add(1)
	case "fill_dedupe":
		stats.fillsDeduped.Add(1)
	case "order_ack":
		if success, ok := fields["success"].(bool); ok && success {
			stats.orderAcksSuccess.Add(1)
		} else {
			stats.orderAcksFailure.Add(1)
		}
	}
}

func stringField(fields map[string]any, key string) string {
	if value, ok := fields[key].(string); ok {
		return value
	}

	return ""
}
