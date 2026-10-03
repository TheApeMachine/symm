package strategy

import (
	"fmt"
	"math"
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
Populate writes all canonical learning and evaluation metrics, provenance,
and metadata onto an existing measurement without extra heap allocations.
*/
func (reporter *Reporter) Populate(
	out *data.Measurement[float64],
	snapshot ReportSnapshot,
) {
	if out == nil {
		return
	}

	stepCount := reporter.steps.Add(1)

	if snapshot.Action != 0 {
		reporter.decisions.Add(1)
	}

	out.SeqIdx = snapshot.SeqIdx

	if snapshot.Symbol != "" {
		out.Label = snapshot.Symbol
	}

	if !snapshot.At.IsZero() {
		out.At = snapshot.At
	}

	stepFloat := float64(stepCount)
	decisionFloat := float64(reporter.decisions.Load())

	out.SetMetric("steps", data.NewMetric[float64](
		"steps",
		data.UnitCount,
		data.TimescaleSession,
		stepFloat,
		math.Max(stepFloat, 1.0),
	).Write(stepFloat))

	out.SetMetric("decisions", data.NewMetric[float64](
		"decisions",
		data.UnitCount,
		data.TimescaleSession,
		decisionFloat,
		math.Max(decisionFloat, 1.0),
	).Write(decisionFloat))

	resolvedCount := float64(snapshot.Resolved)
	resolvedScale := math.Max(resolvedCount, 1.0)

	out.SetMetric("resolved", data.NewMetric[float64](
		"resolved",
		data.UnitCount,
		data.TimescaleSession,
		resolvedCount,
		resolvedScale,
	).Write(resolvedCount))

	out.SetMetric("edge_sample_count", data.NewMetric[float64](
		"edge_sample_count",
		data.UnitCount,
		data.TimescaleSession,
		resolvedCount,
		resolvedScale,
	).Write(resolvedCount))

	out.SetMetric("win_rate", data.NewMetric[float64](
		"win_rate",
		data.UnitProbability,
		data.TimescaleRollingWindow,
		0.5,
		0.5,
	).Write(snapshot.WinRate))

	out.SetMetric("accuracy", data.NewMetric[float64](
		"accuracy",
		data.UnitProbability,
		data.TimescaleRollingWindow,
		0.5,
		0.5,
	).Write(snapshot.WinRate))

	out.SetMetric("edge", data.NewMetric[float64](
		"edge",
		data.UnitPercent,
		data.TimescaleRollingWindow,
		0.0,
		0.01,
	).Write(snapshot.Edge))

	out.SetMetric("confidence", data.NewMetric[float64](
		"confidence",
		data.UnitConfidence,
		data.TimescaleRollingWindow,
		0.5,
		0.5,
	).Write(snapshot.Confidence))

	out.SetMetric("contrast", data.NewMetric[float64](
		"contrast",
		data.UnitRatio,
		data.TimescaleRollingWindow,
		0.0,
		1.0,
	).Write(snapshot.Contrast))

	out.SetMetric("stage_code", data.NewMetric[float64](
		"stage_code",
		data.UnitCount,
		data.TimescaleSession,
		0.0,
		1.0,
	).Write(float64(snapshot.Stage)))

	tradingValue := 0.0

	if snapshot.Trading {
		tradingValue = 1.0
	}

	out.SetMetric("trading", data.NewMetric[float64](
		"trading",
		data.UnitProbability,
		data.TimescaleInstantaneous,
		0.5,
		0.5,
	).Write(tradingValue))

	out.SetMetric("action", data.NewMetric[float64](
		"action",
		data.UnitRatio,
		data.TimescaleTick,
		0.0,
		1.0,
	).Write(float64(snapshot.Action)))

	out.SetMetric("frozen_prediction", data.NewMetric[float64](
		"frozen_prediction",
		data.UnitRatio,
		data.TimescaleTick,
		0.0,
		1.0,
	).Write(float64(snapshot.Action)))

	if snapshot.Clears {
		out.SetMetric("delayed_target", data.NewMetric[float64](
			"delayed_target",
			data.UnitRatio,
			data.TimescaleTick,
			0.0,
			1.0,
		).Write(1.0))
	}

	if !snapshot.Clears && snapshot.Action != 0 {
		out.SetMetric("delayed_target", data.NewMetric[float64](
			"delayed_target",
			data.UnitRatio,
			data.TimescaleTick,
			0.0,
			1.0,
		).Write(float64(snapshot.Action)))
	}

	if snapshot.Direction == "up" {
		out.SetMetric("excursion_type", data.NewMetric[float64](
			"excursion_type",
			data.UnitCount,
			data.TimescaleEvent,
			1.5,
			0.5,
		).Write(1.0))
		out.SetProvenance("excursion_direction", "up")
		out.SetMetadata("excursion_direction", "up")
	}

	if snapshot.Direction == "down" {
		out.SetMetric("excursion_type", data.NewMetric[float64](
			"excursion_type",
			data.UnitCount,
			data.TimescaleEvent,
			1.5,
			0.5,
		).Write(2.0))
		out.SetProvenance("excursion_direction", "down")
		out.SetMetadata("excursion_direction", "down")
	}

	seqSpan := math.Max(float64(snapshot.MarkC-snapshot.MarkA), 1.0)
	seqCenter := float64(snapshot.MarkB)

	out.SetMetric("mark_a", data.NewMetric[float64](
		"mark_a",
		data.UnitCount,
		data.TimescaleEvent,
		seqCenter,
		seqSpan,
	).Write(float64(snapshot.MarkA)))

	out.SetMetric("mark_b", data.NewMetric[float64](
		"mark_b",
		data.UnitCount,
		data.TimescaleEvent,
		seqCenter,
		seqSpan,
	).Write(float64(snapshot.MarkB)))

	out.SetMetric("mark_c", data.NewMetric[float64](
		"mark_c",
		data.UnitCount,
		data.TimescaleEvent,
		seqCenter,
		seqSpan,
	).Write(float64(snapshot.MarkC)))

	tokenLength := len(snapshot.RegionTokens)

	if tokenLength == 0 {
		tokenLength = len(snapshot.Tokens)
	}

	out.SetMetric("precursor_length", data.NewMetric[float64](
		"precursor_length",
		data.UnitCount,
		data.TimescaleEvent,
		3.0,
		3.0,
	).Write(float64(tokenLength)))

	if snapshot.Price > 0 {
		out.SetMetric("price", data.NewMetric[float64](
			"price",
			data.UnitPrice,
			data.TimescaleTick,
			snapshot.Price,
			math.Max(snapshot.Price*0.001, 1e-6),
		).Write(snapshot.Price))
	}

	excursionMag := snapshot.ExcursionMag
	out.SetMetric("excursion_mag", data.NewMetric[float64](
		"excursion_mag",
		data.UnitSpread,
		data.TimescaleEvent,
		excursionMag,
		math.Max(excursionMag, 1e-6),
	).Write(excursionMag))

	out.SetProvenance("stage", snapshot.Stage.String())

	blockerMessage := snapshot.Blocker

	if blockerMessage == "" {
		blockerMessage = "—"
	}

	out.SetProvenance("stage_blocker", blockerMessage)

	var tokenParts []string

	if len(snapshot.RegionTokens) > 0 {
		for _, tokenBytes := range snapshot.RegionTokens {
			if len(tokenBytes) == 1 {
				tokenParts = append(tokenParts, strconv.Itoa(int(tokenBytes[0])))
			}

			if len(tokenBytes) > 1 {
				if tokenBytes[0] == 'R' {
					tokenParts = append(tokenParts, string(tokenBytes))
				} else {
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
		out.SetProvenance("precursor_tokens", serializedTokens)
		out.SetMetadata("precursor_tokens", serializedTokens)
	}

	if snapshot.MarkA > 0 {
		out.SetMetadata("excursion_start", strconv.FormatInt(snapshot.MarkA, 10))
	}

	if snapshot.MarkB > 0 {
		out.SetMetadata("excursion_ignition", strconv.FormatInt(snapshot.MarkB, 10))
	}

	if snapshot.MarkC > 0 {
		out.SetMetadata("excursion_exit", strconv.FormatInt(snapshot.MarkC, 10))
	}

	out.SetMetadata("excursion_clears", fmt.Sprintf("%t", snapshot.Clears))

	if snapshot.Event != "" {
		out.SetMetadata("excursion_event", snapshot.Event)
	}
}
