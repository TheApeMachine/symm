package trader

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

type hindsightKey struct {
	symbol      string
	source      string
	predictedAt time.Time
	dueAt       time.Time
}

type hindsightDecision struct {
	reason       string
	fields       map[string]any
	lastReason   string
	lastFields   map[string]any
	observations []map[string]any
}

type hindsightTracker struct {
	mu      sync.Mutex
	writer  *hindsightWriter
	skipped map[hindsightKey]hindsightDecision
}

func newHindsightTracker() *hindsightTracker {
	return &hindsightTracker{
		writer:  newHindsightWriter(),
		skipped: make(map[hindsightKey]hindsightDecision),
	}
}

func (hindsightTracker *hindsightTracker) RecordSkip(
	prediction engine.Prediction,
	reason string,
	fields map[string]any,
) {
	key, ok := hindsightKeyForPrediction(prediction)

	if !ok {
		return
	}

	storedFields := copyAnyFields(fields)
	storedFields["reason"] = reason

	if hindsightSkipAtOrAfterDue(storedFields, prediction.DueAt) {
		return
	}

	hindsightTracker.mu.Lock()
	defer hindsightTracker.mu.Unlock()

	decision, exists := hindsightTracker.skipped[key]

	if !exists {
		hindsightTracker.skipped[key] = hindsightDecision{
			reason:       reason,
			fields:       storedFields,
			lastReason:   reason,
			lastFields:   storedFields,
			observations: []map[string]any{storedFields},
		}
		return
	}

	decision.lastReason = reason
	decision.lastFields = storedFields
	decision.observations = append(decision.observations, storedFields)
	hindsightTracker.skipped[key] = decision
}

func (hindsightTracker *hindsightTracker) Settle(
	feedback engine.PredictionFeedback,
) error {
	key, ok := hindsightKeyForFeedback(feedback)

	if !ok {
		return nil
	}

	decision, ok := hindsightTracker.consume(key)

	if !ok {
		return nil
	}

	economics := hindsightEconomicsFor(decision)
	missedReturn := feedback.ActualReturn

	if economics.hasFriction {
		missedReturn = feedback.ActualReturn - economics.friction
	}

	if missedReturn <= 0 {
		return nil
	}

	if !economics.significant(feedback.ActualReturn) {
		return nil
	}

	return hindsightTracker.writer.Write(
		hindsightRow(feedback, decision, economics, missedReturn),
	)
}

func (hindsightTracker *hindsightTracker) consume(
	key hindsightKey,
) (hindsightDecision, bool) {
	hindsightTracker.mu.Lock()
	defer hindsightTracker.mu.Unlock()

	decision, ok := hindsightTracker.skipped[key]

	if ok {
		delete(hindsightTracker.skipped, key)
	}

	return decision, ok
}

type hindsightWriter struct {
	mu      sync.Mutex
	once    sync.Once
	file    *os.File
	path    string
	initErr error
}

func newHindsightWriter() *hindsightWriter {
	return &hindsightWriter{}
}

func (hindsightWriter *hindsightWriter) Write(fields map[string]any) error {
	hindsightWriter.once.Do(hindsightWriter.init)

	if hindsightWriter.initErr != nil {
		return hindsightWriter.initErr
	}

	payload, err := json.Marshal(fields)

	if err != nil {
		return fmt.Errorf("hindsight marshal: %w", err)
	}

	hindsightWriter.mu.Lock()
	defer hindsightWriter.mu.Unlock()

	if _, err := hindsightWriter.file.Write(payload); err != nil {
		return fmt.Errorf("hindsight write: %w", err)
	}

	if _, err := hindsightWriter.file.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("hindsight newline: %w", err)
	}

	return nil
}

func (hindsightWriter *hindsightWriter) init() {
	dir := config.System.LogDir

	if dir == "" {
		dir = "runs"
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		hindsightWriter.initErr = fmt.Errorf("hindsight mkdir: %w", err)
		return
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	hindsightWriter.path = filepath.Join(dir, fmt.Sprintf("hindsight-%s.jsonl", runID))

	file, err := os.OpenFile(
		hindsightWriter.path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)

	if err != nil {
		hindsightWriter.initErr = fmt.Errorf("hindsight open: %w", err)
		return
	}

	hindsightWriter.file = file
}

func (crypto *Crypto) recordEntrySkip(
	prediction engine.Prediction,
	reason string,
	fields map[string]any,
) {
	if fields == nil {
		fields = make(map[string]any)
	}

	fields["reason"] = reason
	audit("trade_entry_skip", fields)

	if crypto.hindsight == nil {
		return
	}

	crypto.hindsight.RecordSkip(prediction, reason, fields)
}

func hindsightKeyForPrediction(
	prediction engine.Prediction,
) (hindsightKey, bool) {
	lead, ok := prediction.LeadMeasurement()

	if !ok || prediction.PredictedAt.IsZero() || prediction.DueAt.IsZero() {
		return hindsightKey{}, false
	}

	return hindsightKey{
		symbol:      lead.Pairs[0].Wsname,
		source:      engine.PerspectiveSource(prediction.Perspective.Type),
		predictedAt: canonicalHindsightTime(prediction.PredictedAt),
		dueAt:       canonicalHindsightTime(prediction.DueAt),
	}, true
}

func hindsightKeyForFeedback(
	feedback engine.PredictionFeedback,
) (hindsightKey, bool) {
	if feedback.Symbol == "" ||
		feedback.Source == "" ||
		feedback.PredictedAt.IsZero() ||
		feedback.DueAt.IsZero() {
		return hindsightKey{}, false
	}

	return hindsightKey{
		symbol:      feedback.Symbol,
		source:      feedback.Source,
		predictedAt: canonicalHindsightTime(feedback.PredictedAt),
		dueAt:       canonicalHindsightTime(feedback.DueAt),
	}, true
}

func hindsightRow(
	feedback engine.PredictionFeedback,
	decision hindsightDecision,
	economics hindsightEconomics,
	missedReturn float64,
) map[string]any {
	row := map[string]any{
		"event":            "hindsight_missed_opportunity",
		"symbol":           feedback.Symbol,
		"source":           feedback.Source,
		"sources":          append([]string(nil), feedback.Sources...),
		"contributions":    copyFloatFields(feedback.Contributions),
		"reason":           decision.reason,
		"last_reason":      decision.lastReason,
		"predicted_return": feedback.PredictedReturn,
		"actual_return":    feedback.ActualReturn,
		"missed_return":    missedReturn,
		"required_return":  economics.requiredReturn,
		"return_multiple":  economics.returnMultiple(feedback.ActualReturn),
		"error":            feedback.Error,
		"confidence":       feedback.Confidence,
		"regime":           feedback.Regime,
		"runway_ms":        feedback.Runway.Milliseconds(),
		"predicted_at":     feedback.PredictedAt.UTC().Format(time.RFC3339Nano),
		"due_at":           feedback.DueAt.UTC().Format(time.RFC3339Nano),
		"settled_at":       feedback.SettledAt.UTC().Format(time.RFC3339Nano),
		"decision":         copyAnyFields(decision.fields),
		"last_decision":    copyAnyFields(decision.lastFields),
		"decisions":        copyDecisionRows(decision.observations),
		"ts":               time.Now().UTC().Format(time.RFC3339Nano),
	}

	if economics.hasFriction {
		row["friction"] = economics.friction
		row["required_edge_return"] = economics.requiredEdgeReturn
	}

	if economics.hasStopFraction {
		row["stop_fraction"] = economics.stopFraction
		row["required_r_return"] = economics.requiredRReturn
	}

	return row
}

func canonicalHindsightTime(value time.Time) time.Time {
	return value.UTC().Round(0)
}

func copyAnyFields(fields map[string]any) map[string]any {
	if fields == nil {
		return map[string]any{}
	}

	return maps.Clone(fields)
}

func copyFloatFields(fields map[string]float64) map[string]float64 {
	if fields == nil {
		return map[string]float64{}
	}

	return maps.Clone(fields)
}

func copyDecisionRows(rows []map[string]any) []map[string]any {
	copiedRows := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		copiedRows = append(copiedRows, copyAnyFields(row))
	}

	return copiedRows
}

type hindsightEconomics struct {
	entryReturnRequirement
	hasFriction     bool
	hasStopFraction bool
}

func hindsightEconomicsFor(decision hindsightDecision) hindsightEconomics {
	economics := hindsightEconomics{}
	friction := 0.0
	stopFraction := 0.0

	for _, fields := range decision.observations {
		if fieldFriction, ok := hindsightFloatField(fields, "friction"); ok {
			economics.hasFriction = true

			if fieldFriction > friction {
				friction = fieldFriction
			}
		}

		if fieldStopFraction, ok := hindsightFloatField(fields, "stop_fraction"); ok {
			economics.hasStopFraction = true

			if fieldStopFraction > stopFraction {
				stopFraction = fieldStopFraction
			}
		}
	}

	economics.entryReturnRequirement = newEntryReturnRequirement(
		friction,
		stopFraction,
	)

	return economics
}

func (economics hindsightEconomics) significant(actualReturn float64) bool {
	return economics.entryReturnRequirement.significant(actualReturn)
}

func (economics hindsightEconomics) returnMultiple(actualReturn float64) float64 {
	return economics.entryReturnRequirement.multipleOrZero(actualReturn)
}

func hindsightSkipAtOrAfterDue(fields map[string]any, dueAt time.Time) bool {
	if dueAt.IsZero() {
		return false
	}

	value, ok := fields["ts"].(string)

	if !ok || value == "" {
		return false
	}

	skipAt, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		return false
	}

	return !skipAt.Before(dueAt)
}

func hindsightFloatField(fields map[string]any, key string) (float64, bool) {
	value, ok := fields[key]

	if !ok {
		return 0, false
	}

	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}
