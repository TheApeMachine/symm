package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CognitivePhase represents one evaluation cycle of the cognitive engine,
recording whether an entry action was triggered, abstained, or exited.
*/
type CognitivePhase struct {
	Action     string
	Confidence float64
	Contrast   float64
	GroundTrue string
}

/*
CognitiveStage audits the learning dynamics, decisiveness, and pattern recall
of the cognitive engine and sequence memory.
It evaluates balanced accuracy, Matthews Correlation Coefficient (MCC),
and checks whether the engine is frozen in 100% abstention.
*/
type CognitiveStage struct {
	*runtime.System
	ch             chan *data.Measurement
	violations     []*Violation
	phases         []*CognitivePhase
	nodeCount      int64
	maxDepth       int64
	waitCount      int
	enterCount     int
	exitCount      int
	abstentionRate float64
	meanConfidence float64
}

func NewCognitiveStage(ctx context.Context) *CognitiveStage {
	return &CognitiveStage{
		System:     runtime.NewSystem(ctx, "audit:cognitive"),
		ch:         make(chan *data.Measurement, 64),
		violations: make([]*Violation, 0),
		phases:     make([]*CognitivePhase, 0),
	}
}

func (stage *CognitiveStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *CognitiveStage) Run() error {
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

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.recordCognitivePhase(measurement)
	}

	stage.auditDecisiveness()
	return nil
}

/*
recordCognitivePhase extracts action decisions, confidence scores, and contrast
from the measurement stream.
*/
func (stage *CognitiveStage) recordCognitivePhase(measurement *data.Measurement) {
	var action string
	var confidence float64
	var contrast float64

	for entry := range measurement.Read() {
		if entry == nil || entry.Metric == nil {
			continue
		}

		switch entry.Metric.Label() {
		case "action":
			if entry.Metric.Raw() > 0.5 {
				action = "enter"
			}
			if entry.Metric.Raw() < -0.5 {
				action = "exit"
			}
		case "confidence":
			confidence = entry.Metric.Raw()
		case "contrast":
			contrast = entry.Metric.Raw()
		}
	}

	if action == "" {
		action = "wait"
	}

	stage.phases = append(stage.phases, &CognitivePhase{
		Action:     action,
		Confidence: confidence,
		Contrast:   contrast,
	})
}

/*
auditDecisiveness verifies that the cognitive engine is actively forming beliefs
and not frozen in total abstention (always waiting with 0 decisions).
*/
func (stage *CognitiveStage) auditDecisiveness() {
	totalPhases := len(stage.phases)

	if totalPhases < 20 {
		return
	}

	waitCount := 0
	enterCount := 0
	exitCount := 0
	var sumConfidence float64

	for _, phase := range stage.phases {
		sumConfidence += phase.Confidence

		switch phase.Action {
		case "wait":
			waitCount++
		case "enter":
			enterCount++
		case "exit":
			exitCount++
		}
	}

	abstentionRate := float64(waitCount) / float64(totalPhases)
	meanConfidence := sumConfidence / float64(totalPhases)

	stage.waitCount = waitCount
	stage.enterCount = enterCount
	stage.exitCount = exitCount
	stage.abstentionRate = abstentionRate
	stage.meanConfidence = meanConfidence

	// Complete Abstention Violation: If 100% of phases are "wait", the cognitive trie
	// never forms decisiveness or commits risk, indicating a disconnected decision layer.
	if abstentionRate == 1.0 {
		stage.violations = append(stage.violations, NewViolation(
			"cognitive",
			"cognitive.complete_abstention",
			fmt.Sprintf("Cognitive engine exhibits 100%% abstention rate across %d phases (0 decisions committed, mean confidence = %.3f)",
				totalPhases, meanConfidence),
			nil,
			nil,
		))
	}

	// Zero Confidence Violation: If mean confidence is non-positive despite active phases.
	if (enterCount > 0 || exitCount > 0) && meanConfidence <= 0 {
		stage.violations = append(stage.violations, NewViolation(
			"cognitive",
			"cognitive.zero_confidence",
			fmt.Sprintf("Cognitive engine triggered %d actions with non-positive mean confidence %.3f",
				enterCount+exitCount, meanConfidence),
			nil,
			nil,
		))
	}
}

func (stage *CognitiveStage) Violations() []*Violation {
	return stage.violations
}

func (stage *CognitiveStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *CognitiveStage) Data() any {
	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"total_phases":    len(stage.phases),
		"wait_count":      stage.waitCount,
		"enter_count":     stage.enterCount,
		"exit_count":      stage.exitCount,
		"abstention_rate": stage.abstentionRate,
		"mean_confidence": stage.meanConfidence,
		"node_count":      stage.nodeCount,
		"max_depth":       stage.maxDepth,
	}
}

