package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CognitivePhase represents one evaluation cycle of the cognitive engine,
recording whether an entry action was triggered, abstained, or exited.
*/
type CognitivePhase struct {
	Action      string  `json:"action"`
	Confidence  float64 `json:"confidence"`
	Contrast    float64 `json:"contrast"`
	GroundTrue  string  `json:"ground_true"`
	Path        string  `json:"path"`
	Gain        float64 `json:"gain"`
	HoldSeconds float64 `json:"hold_seconds"`
	Visits      int     `json:"visits"`
}

/*
CognitiveStage audits the learning dynamics, decisiveness, and action distribution
of the cognitive engine and sequence memory from published policy decisions.
It verifies that the engine commits risk (not frozen in 100% abstention)
and that decision confidence is strictly positive.
*/
type CognitiveStage struct {
	*runtime.System
	ch             chan *data.Value
	violations     []*Violation
	phases         []*CognitivePhase
	totalChecked   int
	nodeCount      int64
	maxDepth       int64
	waitCount      int
	noopCount      int
	enterCount     int
	exitCount      int
	abstentionRate float64
	meanConfidence float64
	gate           *data.PolicyGate
	policyEpoch    uint64
}

func NewCognitiveStage(ctx context.Context) *CognitiveStage {
	return &CognitiveStage{
		System:     runtime.NewSystem(ctx, "audit:cognitive"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		phases:     make([]*CognitivePhase, 0),
	}
}

func (stage *CognitiveStage) Channel() chan *data.Value {
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

		stage.totalChecked++

		stage.recordCognitivePhase(measurement)
	}

	stage.auditDecisiveness()
	return nil
}

/*
recordCognitivePhase extracts action decisions, confidence scores, and contrast
from the measurement stream.
*/
func (stage *CognitiveStage) recordCognitivePhase(measurement *data.Value) {
	if measurement.Role == "runtime:join" {
		return
	}

	// policy/current is judged only when no policy epoch is selected
	// (policyEpoch == 0) and may omit the gate. A selected epoch expects
	// one on every policy, tagged or not.
	if measurement.Origin == data.PolicyOrigin && (stage.policyEpoch != 0 || hasGateLayer(measurement)) {
		stage.recordGate(measurement)
	}

	var action string
	var confidence float64
	var contrast float64
	var path string
	var gain float64
	var holdSeconds float64
	var visits int
	hasDecision := false

	metrics := data.Metrics(measurement)

	if actVal, ok := metrics["action"]; ok {
		hasDecision = true
		if actVal > 0.5 {
			action = "enter"
		} else if actVal < -0.5 {
			action = "exit"
		} else {
			action = "wait"
		}
	} else if measurement.Role == "enter" || measurement.Role == "exit" || measurement.Role == "wait" || measurement.Role == "noop" {
		action = measurement.Role
		hasDecision = true
	}

	if confVal, ok := metrics["confidence"]; ok {
		confidence = confVal
		hasDecision = true
	}
	if contVal, ok := metrics["contrast"]; ok {
		contrast = contVal
	}
	if gainVal, ok := metrics["gain"]; ok {
		gain = gainVal
	}
	if holdVal, ok := metrics["hold_seconds"]; ok {
		holdSeconds = holdVal
	}
	if visitsVal, ok := metrics["visits"]; ok {
		visits = int(visitsVal)
	}

	if measurement.Origin == data.PolicyOrigin {
		if read, err := data.PolicyPath(measurement); err == nil {
			path = read
		}
	}

	if !hasDecision {
		return
	}

	if action == "" {
		action = "noop"
	}

	stage.phases = append(stage.phases, &CognitivePhase{
		Action:      action,
		Confidence:  confidence,
		Contrast:    contrast,
		Path:        path,
		Gain:        gain,
		HoldSeconds: holdSeconds,
		Visits:      visits,
	})
}

/*
recordGate keeps the gate the judged policies were trained under; a policy
without one, or one disagreeing with the others, is a violation.
*/
func (stage *CognitiveStage) recordGate(policy *data.Value) {
	gate, err := data.ReadPolicyGate(policy)

	switch {
	case err != nil:
		stage.violations = append(stage.violations, NewViolation("all", "cognitive.gate", err.Error(), policy, nil))
	case stage.gate == nil:
		stage.gate = &gate
	case gate != *stage.gate:
		stage.violations = append(stage.violations, NewViolation(
			"all", "cognitive.gate", fmt.Sprintf("%s carries %s, other policies %s", policy.Prefix, gate, stage.gate), policy, nil,
		))
	}
}

/*
auditDecisiveness verifies that the cognitive engine is actively forming beliefs
and not frozen in total abstention (always waiting with 0 decisions).
*/
func (stage *CognitiveStage) auditDecisiveness() {
	totalPhases := len(stage.phases)

	if stage.totalChecked > 0 && totalPhases == 0 {
		stage.violations = append(stage.violations, NewViolation(
			"cognitive",
			"cognitive.no_decisions",
			"No cognitive decisions witnessed in stream",
			nil,
			nil,
		))
		return
	}

	if totalPhases == 0 {
		stage.violations = append(stage.violations, insufficient("cognitive", "no published policies for the epoch: run train on it"))
		return
	}

	waitCount := 0
	noopCount := 0
	enterCount := 0
	exitCount := 0
	var sumConfidence float64

	for _, phase := range stage.phases {
		sumConfidence += phase.Confidence

		switch phase.Action {
		case "wait":
			waitCount++
		case "noop":
			noopCount++
		case "enter":
			enterCount++
		case "exit":
			exitCount++
		}
	}

	abstentionRate := float64(waitCount+noopCount) / float64(totalPhases)
	meanConfidence := sumConfidence / float64(totalPhases)

	stage.waitCount = waitCount
	stage.noopCount = noopCount
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
		"passed":          len(stage.violations) == 0 && len(stage.phases) > 0,
		"total_phases":    len(stage.phases),
		"wait_count":      stage.waitCount,
		"noop_count":      stage.noopCount,
		"enter_count":     stage.enterCount,
		"exit_count":      stage.exitCount,
		"abstention_rate": stage.abstentionRate,
		"mean_confidence": stage.meanConfidence,
		"node_count":      stage.nodeCount,
		"max_depth":       stage.maxDepth,
		"phases":          stage.phases,
		"gate":            gateData(stage.gate),
	}
}

/*
gateData is the gate tag a policy stage reports: which policy epoch it
judged, trained on which ticks, under which gate (nil when none was judged).
*/
func hasGateLayer(value *data.Value) bool {
	if value == nil {
		return false
	}

	needle := "/" + data.PolicyGateLayer + "/"

	for _, index := range value.Index {
		if strings.Contains(index, needle) {
			return true
		}
	}

	return false
}

func gateData(gate *data.PolicyGate) any {
	if gate == nil {
		return nil
	}

	return map[string]any{
		"policy_epoch":  fmt.Sprint(gate.Epoch),
		"tick_epoch":    fmt.Sprint(gate.TickEpoch),
		"enabled":       gate.Enabled,
		"surprise_z":    gate.SurpriseZ,
		"warmup":        gate.Warmup.String(),
		"learning_rate": gate.LearningRate,
	}
}
