package strategy

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

type StageCode int

const (
	StageModelDevelopment     StageCode = 0
	StageHistoricalValidation StageCode = 1
	StageForwardPaperLearning StageCode = 2
)

func (stage StageCode) String() string {
	switch stage {
	case StageModelDevelopment:
		return "MODEL DEVELOPMENT"
	case StageHistoricalValidation:
		return "HISTORICAL VALIDATION"
	case StageForwardPaperLearning:
		return "FORWARD PAPER LEARNING"
	default:
		return "MODEL DEVELOPMENT"
	}
}

type ReportSnapshot struct {
	Source       string
	Symbol       string
	SeqIdx       int64
	At           time.Time
	Stage        StageCode
	Blocker      string
	Action       int // 1=Enter, 2=Exit, 0=Wait
	Confidence   float64
	Contrast     float64
	Tokens       []byte
	RegionTokens [][]byte
	MarkA        int64
	MarkB        int64
	MarkC        int64
	Price        float64
	ExcursionMag float64
	Direction    string
	Clears       bool
	Event        string
	Trading      bool
	Resolved     int64   // Round trips counted into WinRate and Edge.
	WinRate      float64 // Share of round trips with positive PnL.
	Edge         float64 // Mean round-trip return on entry cost.
}

/*
Reporter writes the canonical learning telemetry onto Training's output
measurement, feeding the Learning Dashboard and ForwardLearningViz.
*/
type Reporter struct {
	steps     atomic.Int64
	decisions atomic.Int64
}

func NewReporter() *Reporter {
	return &Reporter{}
}

/*
Metadata builds all canonical provenance and metadata key-value pairs
for the report snapshot.
*/
func (reporter *Reporter) Metadata(snapshot ReportSnapshot) []data.StringEntry {
	var entries []data.StringEntry

	if snapshot.Direction != "" {
		entries = append(entries, data.StringEntry{Key: "excursion_direction", Value: snapshot.Direction})
	}

	blockerMessage := snapshot.Blocker

	if blockerMessage == "" {
		blockerMessage = "—"
	}

	entries = append(
		entries,
		data.StringEntry{Key: "stage", Value: snapshot.Stage.String()},
		data.StringEntry{Key: "stage_blocker", Value: blockerMessage},
	)

	var tokenParts []string

	if len(snapshot.RegionTokens) > 0 {
		for _, tokenBytes := range snapshot.RegionTokens {
			if len(tokenBytes) == 1 {
				tokenParts = append(tokenParts, strconv.Itoa(int(tokenBytes[0])))
			}

			if len(tokenBytes) > 1 {
				if tokenBytes[0] == 'R' {
					tokenParts = append(tokenParts, string(tokenBytes))
				}

				if tokenBytes[0] != 'R' {
					tokenParts = append(tokenParts, fmt.Sprintf("0x%x", tokenBytes))
				}
			}
		}
	}

	if len(snapshot.RegionTokens) == 0 && len(snapshot.Tokens) > 0 {
		for _, tokenByte := range snapshot.Tokens {
			if tokenByte != 0 {
				tokenParts = append(tokenParts, strconv.Itoa(int(tokenByte)))
			}
		}
	}

	if len(tokenParts) > 0 {
		serializedTokens := strings.Join(tokenParts, ",")
		entries = append(entries, data.StringEntry{Key: "precursor_tokens", Value: serializedTokens})
	}

	if snapshot.MarkA > 0 {
		entries = append(entries, data.StringEntry{Key: "excursion_start", Value: strconv.FormatInt(snapshot.MarkA, 10)})
	}

	if snapshot.MarkB > 0 {
		entries = append(entries, data.StringEntry{Key: "excursion_ignition", Value: strconv.FormatInt(snapshot.MarkB, 10)})
	}

	if snapshot.MarkC > 0 {
		entries = append(entries, data.StringEntry{Key: "excursion_exit", Value: strconv.FormatInt(snapshot.MarkC, 10)})
	}

	entries = append(
		entries,
		data.StringEntry{Key: "excursion_clears", Value: fmt.Sprintf("%t", snapshot.Clears)},
	)

	if snapshot.Event != "" {
		entries = append(entries, data.StringEntry{Key: "excursion_event", Value: snapshot.Event})
	}

	return entries
}

/*
Metrics constructs the telemetry metrics for the report snapshot.
*/
func (reporter *Reporter) Metrics(snapshot ReportSnapshot) []data.Metric {
	stepCount := reporter.steps.Add(1)

	if snapshot.Action != 0 {
		reporter.decisions.Add(1)
	}

	stepFloat := float64(stepCount)
	decisionFloat := float64(reporter.decisions.Load())
	resolvedCount := float64(snapshot.Resolved)

	metrics := make([]data.Metric, 0, 24)

	stepMetric := data.NewMetric("steps", stepFloat, data.UnitCount, data.TimescaleSession)
	stepMetric.Standardized = stepFloat
	metrics = append(metrics, stepMetric)

	decisionMetric := data.NewMetric("decisions", decisionFloat, data.UnitCount, data.TimescaleSession)
	decisionMetric.Standardized = decisionFloat
	metrics = append(metrics, decisionMetric)

	resolvedMetric := data.NewMetric("resolved", resolvedCount, data.UnitCount, data.TimescaleSession)
	resolvedMetric.Standardized = resolvedCount
	metrics = append(metrics, resolvedMetric)

	edgeSampleMetric := data.NewMetric("edge_sample_count", resolvedCount, data.UnitCount, data.TimescaleSession)
	edgeSampleMetric.Standardized = resolvedCount
	metrics = append(metrics, edgeSampleMetric)

	winRateMetric := data.NewMetric("win_rate", snapshot.WinRate, data.UnitProbability, data.TimescaleRollingWindow)
	winRateMetric.Standardized = snapshot.WinRate
	metrics = append(metrics, winRateMetric)

	accuracyMetric := data.NewMetric("accuracy", snapshot.WinRate, data.UnitProbability, data.TimescaleRollingWindow)
	accuracyMetric.Standardized = snapshot.WinRate
	metrics = append(metrics, accuracyMetric)

	edgeMetric := data.NewMetric("edge", snapshot.Edge, data.UnitPercent, data.TimescaleRollingWindow)
	edgeMetric.Standardized = snapshot.Edge
	metrics = append(metrics, edgeMetric)

	confidenceMetric := data.NewMetric("confidence", snapshot.Confidence, data.UnitConfidence, data.TimescaleRollingWindow)
	confidenceMetric.Standardized = snapshot.Confidence
	metrics = append(metrics, confidenceMetric)

	contrastMetric := data.NewMetric("contrast", snapshot.Contrast, data.UnitRatio, data.TimescaleRollingWindow)
	contrastMetric.Standardized = snapshot.Contrast
	metrics = append(metrics, contrastMetric)

	stageMetric := data.NewMetric("stage_code", float64(snapshot.Stage), data.UnitCount, data.TimescaleSession)
	stageMetric.Standardized = float64(snapshot.Stage)
	metrics = append(metrics, stageMetric)

	tradingValue := 0.0

	if snapshot.Trading {
		tradingValue = 1.0
	}

	tradingMetric := data.NewMetric("trading", tradingValue, data.UnitProbability, data.TimescaleInstantaneous)
	tradingMetric.Standardized = tradingValue
	metrics = append(metrics, tradingMetric)

	actionMetric := data.NewMetric("action", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
	actionMetric.Standardized = float64(snapshot.Action)
	metrics = append(metrics, actionMetric)

	frozenMetric := data.NewMetric("frozen_prediction", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
	frozenMetric.Standardized = float64(snapshot.Action)
	metrics = append(metrics, frozenMetric)

	if snapshot.Clears {
		delayedMetric := data.NewMetric("delayed_target", 1.0, data.UnitRatio, data.TimescaleTick)
		delayedMetric.Standardized = 1.0
		metrics = append(metrics, delayedMetric)
	}

	if !snapshot.Clears && snapshot.Action != 0 {
		delayedMetric := data.NewMetric("delayed_target", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
		delayedMetric.Standardized = float64(snapshot.Action)
		metrics = append(metrics, delayedMetric)
	}

	if snapshot.Direction == "up" {
		excursionMetric := data.NewMetric("excursion_type", 1.0, data.UnitCount, data.TimescaleEvent)
		excursionMetric.Standardized = 1.0
		metrics = append(metrics, excursionMetric)
	}

	if snapshot.Direction == "down" {
		excursionMetric := data.NewMetric("excursion_type", 2.0, data.UnitCount, data.TimescaleEvent)
		excursionMetric.Standardized = 2.0
		metrics = append(metrics, excursionMetric)
	}

	markAMetric := data.NewMetric("mark_a", float64(snapshot.MarkA), data.UnitCount, data.TimescaleEvent)
	markAMetric.Standardized = float64(snapshot.MarkA)
	metrics = append(metrics, markAMetric)

	markBMetric := data.NewMetric("mark_b", float64(snapshot.MarkB), data.UnitCount, data.TimescaleEvent)
	markBMetric.Standardized = float64(snapshot.MarkB)
	metrics = append(metrics, markBMetric)

	markCMetric := data.NewMetric("mark_c", float64(snapshot.MarkC), data.UnitCount, data.TimescaleEvent)
	markCMetric.Standardized = float64(snapshot.MarkC)
	metrics = append(metrics, markCMetric)

	tokenLength := len(snapshot.RegionTokens)

	if tokenLength == 0 {
		tokenLength = len(snapshot.Tokens)
	}

	precursorMetric := data.NewMetric("precursor_length", float64(tokenLength), data.UnitCount, data.TimescaleEvent)
	precursorMetric.Standardized = float64(tokenLength)
	metrics = append(metrics, precursorMetric)

	if snapshot.Price > 0 {
		priceMetric := data.NewMetric("price", snapshot.Price, data.UnitPrice, data.TimescaleTick)
		priceMetric.Standardized = snapshot.Price
		metrics = append(metrics, priceMetric)
	}

	magMetric := data.NewMetric("excursion_mag", snapshot.ExcursionMag, data.UnitSpread, data.TimescaleEvent)
	magMetric.Standardized = snapshot.ExcursionMag
	metrics = append(metrics, magMetric)

	return metrics
}

/*
Populate writes all canonical learning and evaluation metrics onto the measurement,
finalizing it into WORM locked state.
*/
func (reporter *Reporter) Populate(
	out *data.Measurement,
	snapshot ReportSnapshot,
	extraMetrics ...data.Metric,
) *data.Measurement {
	if out == nil {
		return nil
	}

	out.SeqIdx = snapshot.SeqIdx

	if snapshot.Symbol != "" {
		out.Label = snapshot.Symbol
	}

	if !snapshot.At.IsZero() {
		out.At = snapshot.At
	}

	metrics := reporter.Metrics(snapshot)

	if len(extraMetrics) > 0 {
		metrics = append(metrics, extraMetrics...)
	}

	return out.Write(metrics...)
}
