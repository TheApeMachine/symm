package strategy

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type StageCode int

const (
	StageModelDevelopment      StageCode = 0
	StageHistoricalValidation  StageCode = 1
	StageForwardPaperLearning  StageCode = 2
	StageForwardSkillValidated StageCode = 3
)

func (stage StageCode) String() string {
	switch stage {
	case StageModelDevelopment:
		return "MODEL DEVELOPMENT"
	case StageHistoricalValidation:
		return "HISTORICAL VALIDATION"
	case StageForwardPaperLearning:
		return "FORWARD PAPER LEARNING"
	case StageForwardSkillValidated:
		return "FORWARD SKILL DEMONSTRATED"
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
}

/*
Reporter publishes real-time telemetry frames through the UI tee off-ramp,
feeding the Learning Dashboard, ForwardLearningViz, and AgentSkill components.
*/
type Reporter struct {
	mu        sync.Mutex
	arena     *data.ArenaOwner
	tee       runtime.Tee
	steps     atomic.Int64
	decisions atomic.Int64
}

func NewReporter(arena *data.ArenaOwner, tee runtime.Tee) *Reporter {
	return &Reporter{
		arena: arena,
		tee:   tee,
	}
}

func (reporter *Reporter) newMeasurement(source string) *data.Measurement[float64] {
	reporter.mu.Lock()
	defer reporter.mu.Unlock()

	return reporter.arena.NewMeasurement(source)
}

/*
Populate writes all canonical learning and evaluation metrics, provenance,
and metadata onto an existing measurement without extra heap allocations.
*/
func (reporter *Reporter) Populate(
	out *data.Measurement[float64],
	snapshot ReportSnapshot,
	skill *Skill,
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

	if skill != nil {
		resolvedCount := float64(skill.Resolved())
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

		winRate := skill.WinRate()
		out.SetMetric("win_rate", data.NewMetric[float64](
			"win_rate",
			data.UnitProbability,
			data.TimescaleRollingWindow,
			0.5,
			0.5,
		).Write(winRate))

		out.SetMetric("accuracy", data.NewMetric[float64](
			"accuracy",
			data.UnitProbability,
			data.TimescaleRollingWindow,
			0.5,
			0.5,
		).Write(winRate))

		edge := skill.Edge()
		out.SetMetric("edge", data.NewMetric[float64](
			"edge",
			data.UnitPercent,
			data.TimescaleRollingWindow,
			0.0,
			0.01,
		).Write(edge))

		opps := float64(skill.HistOpportunities())
		oppsScale := math.Max(opps, 1.0)

		out.SetMetric("hist_opportunities", data.NewMetric[float64](
			"hist_opportunities",
			data.UnitCount,
			data.TimescaleRollingWindow,
			opps,
			oppsScale,
		).Write(opps))

		out.SetMetric("hist_correct_enter", data.NewMetric[float64](
			"hist_correct_enter",
			data.UnitCount,
			data.TimescaleRollingWindow,
			opps*0.5,
			oppsScale,
		).Write(float64(skill.HistCorrectEnter())))

		out.SetMetric("hist_missed_enter", data.NewMetric[float64](
			"hist_missed_enter",
			data.UnitCount,
			data.TimescaleRollingWindow,
			opps*0.5,
			oppsScale,
		).Write(float64(skill.HistMissedEnter())))

		out.SetMetric("hist_false_enter", data.NewMetric[float64](
			"hist_false_enter",
			data.UnitCount,
			data.TimescaleRollingWindow,
			opps*0.5,
			oppsScale,
		).Write(float64(skill.HistFalseEnter())))

		out.SetMetric("hist_mean_return", data.NewMetric[float64](
			"hist_mean_return",
			data.UnitPercent,
			data.TimescaleRollingWindow,
			0.0,
			0.01,
		).Write(skill.HistMeanReturn()))

		out.SetMetric("hist_lower_bound", data.NewMetric[float64](
			"hist_lower_bound",
			data.UnitPercent,
			data.TimescaleRollingWindow,
			0.0,
			0.01,
		).Write(skill.HistLowerBound()))

		fwdPreds := float64(skill.FwdPredictions())
		fwdScale := math.Max(fwdPreds, 1.0)

		out.SetMetric("fwd_enter_predictions", data.NewMetric[float64](
			"fwd_enter_predictions",
			data.UnitCount,
			data.TimescaleSession,
			fwdPreds,
			fwdScale,
		).Write(fwdPreds))

		out.SetMetric("fwd_paper_trades", data.NewMetric[float64](
			"fwd_paper_trades",
			data.UnitCount,
			data.TimescaleSession,
			fwdPreds,
			fwdScale,
		).Write(float64(skill.FwdPaperTrades())))

		out.SetMetric("fwd_paper_mean_return", data.NewMetric[float64](
			"fwd_paper_mean_return",
			data.UnitPercent,
			data.TimescaleRollingWindow,
			0.0,
			0.01,
		).Write(skill.FwdMeanReturn()))

		out.SetMetric("fwd_paper_lower_bound", data.NewMetric[float64](
			"fwd_paper_lower_bound",
			data.UnitPercent,
			data.TimescaleRollingWindow,
			0.0,
			0.01,
		).Write(skill.FwdLowerBound()))

		upCount, downCount, chopCount, flatCount, unsupCount := skill.Fragments()
		totalFrags := float64(upCount + downCount + chopCount + flatCount + unsupCount)
		fragScale := math.Max(totalFrags, 1.0)
		fragCenter := totalFrags / 5.0

		out.SetMetric("fragments_up", data.NewMetric[float64](
			"fragments_up",
			data.UnitCount,
			data.TimescaleSession,
			fragCenter,
			fragScale,
		).Write(float64(upCount)))

		out.SetMetric("fragments_down", data.NewMetric[float64](
			"fragments_down",
			data.UnitCount,
			data.TimescaleSession,
			fragCenter,
			fragScale,
		).Write(float64(downCount)))

		out.SetMetric("fragments_chop", data.NewMetric[float64](
			"fragments_chop",
			data.UnitCount,
			data.TimescaleSession,
			fragCenter,
			fragScale,
		).Write(float64(chopCount)))

		out.SetMetric("fragments_flat", data.NewMetric[float64](
			"fragments_flat",
			data.UnitCount,
			data.TimescaleSession,
			fragCenter,
			fragScale,
		).Write(float64(flatCount)))

		out.SetMetric("fragments_unsupported", data.NewMetric[float64](
			"fragments_unsupported",
			data.UnitCount,
			data.TimescaleSession,
			fragCenter,
			fragScale,
		).Write(float64(unsupCount)))

		samples := skill.RecentSamples()

		if len(samples) > 0 {
			sampleStrings := make([]string, 0, len(samples))

			for _, sampleValue := range samples {
				sampleStrings = append(sampleStrings, strconv.FormatFloat(sampleValue, 'f', 6, 64))
			}

			out.SetMetadata("edge_samples", strings.Join(sampleStrings, ","))
		}
	}

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

/*
Publish creates a new measurement, populates canonical learning metrics,
and pushes the publication into the UI tee off-ramp.
*/
func (reporter *Reporter) Publish(snapshot ReportSnapshot, skill *Skill) {
	if reporter == nil || reporter.tee == nil || reporter.arena == nil {
		return
	}

	out := reporter.newMeasurement(snapshot.Source)
	reporter.Populate(out, snapshot, skill)

	publication := data.NewPublication(out, nil)
	reporter.tee.Push(publication)
}

/*
PublishExcursion streams the price curve for a detected trajectory and publishes
the resolved evaluation snapshot upon reaching Mark C.
*/
func (reporter *Reporter) PublishExcursion(
	symbol string,
	precursor []*data.Measurement[float64],
	holding []*data.Measurement[float64],
	precursorTokens [][]byte,
	pnl float64,
	skill *Skill,
) {
	if reporter == nil || reporter.tee == nil || reporter.arena == nil {
		return
	}

	if len(precursor) == 0 || len(holding) == 0 {
		return
	}

	markA := precursor[0].SeqIdx
	markB := precursor[len(precursor)-1].SeqIdx
	markC := holding[len(holding)-1].SeqIdx

	for _, measurement := range precursor {
		quote, hasQuote := quotePrice(measurement)

		if hasQuote {
			seqSpan := math.Max(float64(markC-markA), 1.0)
			seqCenter := float64(markB)

			point := reporter.newMeasurement("training")
			point.Label = symbol
			point.SeqIdx = measurement.SeqIdx
			point.At = measurement.At

			point.SetMetric("price", data.NewMetric[float64](
				"price",
				data.UnitPrice,
				data.TimescaleTick,
				quote,
				math.Max(quote*0.001, 1e-6),
			).Write(quote))

			point.SetMetric("stage_code", data.NewMetric[float64](
				"stage_code",
				data.UnitCount,
				data.TimescaleSession,
				0.0,
				1.0,
			).Write(float64(StageHistoricalValidation)))

			point.SetMetric("mark_a", data.NewMetric[float64](
				"mark_a",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markA)))

			point.SetMetric("mark_b", data.NewMetric[float64](
				"mark_b",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markB)))

			point.SetMetric("mark_c", data.NewMetric[float64](
				"mark_c",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markC)))

			point.SetMetric("agent_entry", data.NewMetric[float64](
				"agent_entry",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markB)))

			point.SetMetric("agent_exit", data.NewMetric[float64](
				"agent_exit",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markC)))

			point.SetMetadata("excursion_start", strconv.FormatInt(markA, 10))
			point.SetMetadata("excursion_ignition", strconv.FormatInt(markB, 10))
			point.SetMetadata("excursion_exit", strconv.FormatInt(markC, 10))
			reporter.tee.Push(data.NewPublication(point, nil))
		}
	}

	for index, measurement := range holding {
		if index == len(holding)-1 {
			continue
		}

		quote, hasQuote := quotePrice(measurement)

		if hasQuote {
			seqSpan := math.Max(float64(markC-markA), 1.0)
			seqCenter := float64(markB)

			point := reporter.newMeasurement("training")
			point.Label = symbol
			point.SeqIdx = measurement.SeqIdx
			point.At = measurement.At

			point.SetMetric("price", data.NewMetric[float64](
				"price",
				data.UnitPrice,
				data.TimescaleTick,
				quote,
				math.Max(quote*0.001, 1e-6),
			).Write(quote))

			point.SetMetric("stage_code", data.NewMetric[float64](
				"stage_code",
				data.UnitCount,
				data.TimescaleSession,
				0.0,
				1.0,
			).Write(float64(StageHistoricalValidation)))

			point.SetMetric("mark_a", data.NewMetric[float64](
				"mark_a",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markA)))

			point.SetMetric("mark_b", data.NewMetric[float64](
				"mark_b",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markB)))

			point.SetMetric("mark_c", data.NewMetric[float64](
				"mark_c",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markC)))

			point.SetMetric("agent_entry", data.NewMetric[float64](
				"agent_entry",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markB)))

			point.SetMetric("agent_exit", data.NewMetric[float64](
				"agent_exit",
				data.UnitCount,
				data.TimescaleEvent,
				seqCenter,
				seqSpan,
			).Write(float64(markC)))

			point.SetMetadata("excursion_start", strconv.FormatInt(markA, 10))
			point.SetMetadata("excursion_ignition", strconv.FormatInt(markB, 10))
			point.SetMetadata("excursion_exit", strconv.FormatInt(markC, 10))
			reporter.tee.Push(data.NewPublication(point, nil))
		}
	}

	exitMeasurement := holding[len(holding)-1]
	exitPrice, _ := quotePrice(exitMeasurement)

	blockerMessage := ""

	if skill != nil {
		blockerMessage = skill.HistBlocker()
	}

	reporter.Publish(ReportSnapshot{
		Source:       "training",
		Symbol:       symbol,
		SeqIdx:       markC,
		At:           exitMeasurement.At,
		Stage:        StageHistoricalValidation,
		Blocker:      blockerMessage,
		Action:       1,
		RegionTokens: precursorTokens,
		MarkA:        markA,
		MarkB:        markB,
		MarkC:        markC,
		Price:        exitPrice,
		ExcursionMag: pnl,
		Direction:    "up",
		Clears:       pnl > 0,
	}, skill)
}

