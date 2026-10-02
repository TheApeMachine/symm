package strategy

import (
	"fmt"
	"strconv"
	"strings"
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

	out.WriteMetric("steps", float64(stepCount))
	out.WriteMetric("decisions", float64(reporter.decisions.Load()))

	if skill != nil {
		resolvedCount := skill.Resolved()
		out.WriteMetric("resolved", float64(resolvedCount))
		out.WriteMetric("edge_sample_count", float64(resolvedCount))
		out.WriteMetric("win_rate", skill.WinRate())
		out.WriteMetric("accuracy", skill.WinRate())
		out.WriteMetric("edge", skill.Edge())

		out.WriteMetric("hist_opportunities", float64(skill.HistOpportunities()))
		out.WriteMetric("hist_correct_enter", float64(skill.HistCorrectEnter()))
		out.WriteMetric("hist_missed_enter", float64(skill.HistMissedEnter()))
		out.WriteMetric("hist_false_enter", float64(skill.HistFalseEnter()))
		out.WriteMetric("hist_mean_return", skill.HistMeanReturn())
		out.WriteMetric("hist_lower_bound", skill.HistLowerBound())

		out.WriteMetric("fwd_enter_predictions", float64(skill.FwdPredictions()))
		out.WriteMetric("fwd_paper_trades", float64(skill.FwdPaperTrades()))
		out.WriteMetric("fwd_paper_mean_return", skill.FwdMeanReturn())
		out.WriteMetric("fwd_paper_lower_bound", skill.FwdLowerBound())

		upCount, downCount, chopCount, flatCount, unsupCount := skill.Fragments()
		out.WriteMetric("fragments_up", float64(upCount))
		out.WriteMetric("fragments_down", float64(downCount))
		out.WriteMetric("fragments_chop", float64(chopCount))
		out.WriteMetric("fragments_flat", float64(flatCount))
		out.WriteMetric("fragments_unsupported", float64(unsupCount))

		samples := skill.RecentSamples()

		if len(samples) > 0 {
			sampleStrings := make([]string, 0, len(samples))

			for _, sampleValue := range samples {
				sampleStrings = append(sampleStrings, strconv.FormatFloat(sampleValue, 'f', 6, 64))
			}

			out.SetMetadata("edge_samples", strings.Join(sampleStrings, ","))
		}
	}

	out.WriteMetric("confidence", snapshot.Confidence)
	out.WriteMetric("contrast", snapshot.Contrast)
	out.WriteMetric("stage_code", float64(snapshot.Stage))

	tradingValue := 0.0

	if snapshot.Trading {
		tradingValue = 1.0
	}

	out.WriteMetric("trading", tradingValue)
	out.WriteMetric("action", float64(snapshot.Action))
	out.WriteMetric("frozen_prediction", float64(snapshot.Action))

	if snapshot.Clears {
		out.WriteMetric("delayed_target", 1.0)
	}

	if !snapshot.Clears && snapshot.Action != 0 {
		out.WriteMetric("delayed_target", float64(snapshot.Action))
	}

	if snapshot.Direction == "up" {
		out.WriteMetric("excursion_type", 1.0)
		out.SetProvenance("excursion_direction", "up")
		out.SetMetadata("excursion_direction", "up")
	}

	if snapshot.Direction == "down" {
		out.WriteMetric("excursion_type", 2.0)
		out.SetProvenance("excursion_direction", "down")
		out.SetMetadata("excursion_direction", "down")
	}

	out.WriteMetric("mark_a", float64(snapshot.MarkA))
	out.WriteMetric("mark_b", float64(snapshot.MarkB))
	out.WriteMetric("mark_c", float64(snapshot.MarkC))

	tokenLength := len(snapshot.RegionTokens)

	if tokenLength == 0 {
		tokenLength = len(snapshot.Tokens)
	}

	out.WriteMetric("precursor_length", float64(tokenLength))

	if snapshot.Price > 0 {
		out.WriteMetric("price", snapshot.Price)
	}

	out.WriteMetric("excursion_mag", snapshot.ExcursionMag)

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
				tokenParts = append(tokenParts, fmt.Sprintf("0x%x", tokenBytes))
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

	out := reporter.arena.NewMeasurement(snapshot.Source)
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
			point := reporter.arena.NewMeasurement("training")
			point.Label = symbol
			point.SeqIdx = measurement.SeqIdx
			point.At = measurement.At
			point.WriteMetric("price", quote)
			point.WriteMetric("stage_code", float64(StageHistoricalValidation))
			point.SetMetadata("excursion_start", strconv.FormatInt(markA, 10))
			reporter.tee.Push(data.NewPublication(point, nil))
		}
	}

	for index, measurement := range holding {
		if index == len(holding)-1 {
			continue
		}

		quote, hasQuote := quotePrice(measurement)

		if hasQuote {
			point := reporter.arena.NewMeasurement("training")
			point.Label = symbol
			point.SeqIdx = measurement.SeqIdx
			point.At = measurement.At
			point.WriteMetric("price", quote)
			point.WriteMetric("stage_code", float64(StageHistoricalValidation))
			point.SetMetadata("excursion_start", strconv.FormatInt(markA, 10))
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

