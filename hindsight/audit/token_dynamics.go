package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DynamicsStage audits token dynamics and state transition entropy across continuous market tape.
It builds an empirical transition matrix between states and calculates the transition entropy H:
H = - sum_i P(i) sum_j P(j|i) log2 P(j|i)
Lower entropy signifies structured, predictable market state sequences.
Excessive entropy indicates random noise, while extreme token concentration indicates a dead/frozen state.
*/
type DynamicsStage struct {
	*runtime.System
	ch          chan *data.Measurement
	violations  []*Violation
	tokens      []string
	transitions map[string]map[string]int64
	tokenCounts map[string]int64
	entropy     float64
	maxEntropy  float64
}

func NewDynamicsStage(ctx context.Context) *DynamicsStage {
	return &DynamicsStage{
		System:      runtime.NewSystem(ctx, "audit:dynamics"),
		ch:          make(chan *data.Measurement, 64),
		violations:  make([]*Violation, 0),
		tokens:      make([]string, 0),
		transitions: make(map[string]map[string]int64),
		tokenCounts: make(map[string]int64),
	}
}

func (stage *DynamicsStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *DynamicsStage) Run() error {
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

		stage.recordToken(measurement)
	}

	stage.auditDynamics()
	return nil
}

func (stage *DynamicsStage) Violations() []*Violation {
	return stage.violations
}

/*
recordToken extracts the active state token or dominant metric from the measurement
and accumulates the sequential transition count from the prior token.
*/
func (stage *DynamicsStage) recordToken(measurement *data.Measurement) {
	var currentToken string

	for entry := range measurement.Read() {
		if entry != nil && entry.Metric != nil {
			currentToken = entry.Metric.Label()
			break
		}
	}

	if currentToken == "" {
		return
	}

	stage.tokens = append(stage.tokens, currentToken)
	stage.tokenCounts[currentToken]++

	if len(stage.tokens) > 1 {
		priorToken := stage.tokens[len(stage.tokens)-2]
		row, exists := stage.transitions[priorToken]

		if !exists {
			row = make(map[string]int64)
			stage.transitions[priorToken] = row
		}

		row[currentToken]++
	}
}

/*
auditDynamics evaluates state transition entropy and checks for absorbing state collapses.
*/
func (stage *DynamicsStage) auditDynamics() {
	totalTokens := len(stage.tokens)

	if totalTokens < 20 {
		return
	}

	// 1. Check for Absorbing State Collapse: If a single token dominates > 80%
	// of all observations, the system is frozen in an absorbing basin.
	for token, count := range stage.tokenCounts {
		share := float64(count) / float64(totalTokens)

		if share > 0.80 {
			stage.violations = append(stage.violations, NewViolation(
				token,
				"dynamics.absorbing_token_collapse",
				fmt.Sprintf("Token %s dominates %.1f%% of all emissions (> 80%% threshold)",
					token, share*100),
				nil,
				nil,
			))
		}
	}

	// 2. Compute Transition Entropy H(S):
	// H = - sum_i P(i) * [ sum_j P(j|i) log2 P(j|i) ]
	stage.entropy = stage.computeTransitionEntropy()
	entropy := stage.entropy

	// Compute empirical maximum possible entropy for the number of observed states
	distinctStates := len(stage.tokenCounts)

	if distinctStates > 1 {
		stage.maxEntropy = math.Log2(float64(distinctStates))
		maxEntropy := stage.maxEntropy

		// If real transition entropy equals max possible entropy within machine precision,
		// state transitions carry 0 predictive structure (pure random walk / noise).
		if math.Abs(entropy-maxEntropy) < 0.01 {
			stage.violations = append(stage.violations, NewViolation(
				"dynamics",
				"dynamics.pure_noise_entropy",
				fmt.Sprintf("Transition entropy H = %.3f bits equals maximum random entropy %.3f bits (zero predictive structure)",
					entropy, maxEntropy),
				nil,
				nil,
			))
		}
	}
}

func (stage *DynamicsStage) computeTransitionEntropy() float64 {
	totalTransitions := float64(len(stage.tokens) - 1)

	if totalTransitions <= 0 {
		return 0
	}

	keys := make([]string, 0, len(stage.tokenCounts))

	for key := range stage.tokenCounts {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	var entropy float64

	for _, stateI := range keys {
		row, hasRow := stage.transitions[stateI]

		if !hasRow {
			continue
		}

		var rowTotal int64

		for _, count := range row {
			rowTotal += count
		}

		if rowTotal <= 0 {
			continue
		}

		probI := float64(stage.tokenCounts[stateI]) / float64(len(stage.tokens))
		var rowEntropy float64

		for _, count := range row {
			if count > 0 {
				probJGivenI := float64(count) / float64(rowTotal)
				rowEntropy -= probJGivenI * math.Log2(probJGivenI)
			}
		}

		entropy += probI * rowEntropy
	}

	return entropy
}

func (stage *DynamicsStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *DynamicsStage) Data() any {
	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"entropy":         stage.entropy,
		"max_entropy":     stage.maxEntropy,
		"total_tokens":    len(stage.tokens),
		"distinct_tokens": len(stage.tokenCounts),
		"token_counts":    stage.tokenCounts,
		"transitions":     stage.transitions,
	}
}

