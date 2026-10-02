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

func (s StageCode) String() string {
	switch s {
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

func (reporter *Reporter) Publish(snapshot ReportSnapshot, skill *Skill) {
	if reporter == nil || reporter.tee == nil || reporter.arena == nil {
		return
	}

	step := reporter.steps.Add(1)

	if snapshot.Action != 0 {
		reporter.decisions.Add(1)
	}

	out := reporter.arena.NewMeasurement(snapshot.Source)
	out.Label = snapshot.Symbol
	out.SeqIdx = snapshot.SeqIdx

	if !snapshot.At.IsZero() {
		out.At = snapshot.At
	} else {
		out.At = time.Now()
	}

	// Core learning performance metrics
	out.WriteMetric("steps", float64(step))
	out.WriteMetric("decisions", float64(reporter.decisions.Load()))

	if skill != nil {
		resolved := skill.Resolved()
		out.WriteMetric("resolved", float64(resolved))
		out.WriteMetric("edge_sample_count", float64(resolved))
		out.WriteMetric("win_rate", skill.WinRate())
		out.WriteMetric("accuracy", skill.WinRate())
		out.WriteMetric("edge", skill.Edge())
	}

	out.WriteMetric("confidence", snapshot.Confidence)
	out.WriteMetric("contrast", snapshot.Contrast)
	out.WriteMetric("stage_code", float64(snapshot.Stage))

	trading := 0.0
	if snapshot.Trading {
		trading = 1.0
	}
	out.WriteMetric("trading", trading)

	out.WriteMetric("action", float64(snapshot.Action))
	out.WriteMetric("frozen_prediction", float64(snapshot.Action))
	out.WriteMetric("mark_a", float64(snapshot.MarkA))
	out.WriteMetric("mark_b", float64(snapshot.MarkB))
	out.WriteMetric("mark_c", float64(snapshot.MarkC))
	out.WriteMetric("precursor_length", float64(len(snapshot.Tokens)))
	out.WriteMetric("price", snapshot.Price)
	out.WriteMetric("excursion_mag", snapshot.ExcursionMag)

	// Stage & blocker provenance
	out.SetProvenance("stage", snapshot.Stage.String())
	out.SetProvenance("stage_blocker", snapshot.Blocker)

	// Excursion markers & classification
	if len(snapshot.Tokens) > 0 {
		var parts []string
		for _, b := range snapshot.Tokens {
			if b != 0 {
				parts = append(parts, strconv.Itoa(int(b)))
			}
		}
		if len(parts) > 0 {
			tokenStr := strings.Join(parts, ",")
			out.SetProvenance("precursor_tokens", tokenStr)
			out.SetMetadata("precursor_tokens", tokenStr)
		}
	}

	out.SetMetadata("excursion_start", strconv.FormatInt(snapshot.MarkA, 10))
	out.SetMetadata("excursion_ignition", strconv.FormatInt(snapshot.MarkB, 10))
	out.SetMetadata("excursion_exit", strconv.FormatInt(snapshot.MarkC, 10))

	if snapshot.Direction != "" {
		out.SetMetadata("excursion_direction", snapshot.Direction)
	}

	out.SetMetadata("excursion_clears", fmt.Sprintf("%t", snapshot.Clears))

	if snapshot.Event != "" {
		out.SetMetadata("excursion_event", snapshot.Event)
	}

	pub := data.NewPublication(out, nil)
	reporter.tee.Push(pub)
}
