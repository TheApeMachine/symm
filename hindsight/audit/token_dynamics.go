package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
)

/*
FrameData records empirical state information for one market tick frame,
including multi-signal region brightness, active token, and price return.
*/
type FrameData struct {
	Tick        int64     `json:"tick"`
	Timestamp   int64     `json:"timestamp"`
	ActiveToken string    `json:"active_token"`
	Brightness  []float64 `json:"brightness"`
	Price       float64   `json:"price"`
	Return      float64   `json:"return"`
}

/*
DynamicsStage audits token dynamics and state transition entropy across continuous market tape.
It builds an empirical transition matrix between states and calculates the transition entropy H:
H = - sum_i P(i) sum_j P(j|i) log2 P(j|i)
Lower entropy signifies structured, predictable market state sequences.
Excessive entropy indicates random noise, while extreme token concentration indicates a dead/frozen state.
*/
type DynamicsStage struct {
	*runtime.System
	ch           chan *data.Value
	violations   []*Violation
	grid         *nmstore.Grid
	tokens       []string
	transitions  map[string]map[string]int64
	tokenCounts  map[string]int64
	frames       []*FrameData
	initialPrice float64
	lastPrice    float64
	entropy      float64
	maxEntropy   float64
}

func NewDynamicsStage(ctx context.Context) *DynamicsStage {
	return &DynamicsStage{
		System:      runtime.NewSystem(ctx, "audit:dynamics"),
		ch:          make(chan *data.Value, 64),
		violations:  make([]*Violation, 0),
		grid:        nmstore.NewGrid(ctx),
		tokens:      make([]string, 0),
		transitions: make(map[string]map[string]int64),
		tokenCounts: make(map[string]int64),
		frames:      make([]*FrameData, 0),
	}
}

func (stage *DynamicsStage) Channel() chan *data.Value {
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

		if measurement.Role != "runtime:join" {
			continue
		}

		if measurement.Role == "error" {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"measurement.error",
				"error measurement encountered",
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
recordToken extracts the active state token from the measurement
and accumulates the sequential transition count from the prior token.
*/
func (stage *DynamicsStage) recordToken(measurement *data.Value) {
	if len(measurement.Layers) == 0 {
		return
	}

	regions := stage.grid.Observe(measurement)

	if len(regions) == 0 || regions[0].Brightness <= 0 {
		return
	}

	currentToken := fmt.Sprintf("R%02d", regions[0].Key)
	brightness := make([]float64, nmstore.RegionCount)

	for _, reg := range regions {
		if int(reg.Key) < len(brightness) {
			brightness[reg.Key] = reg.Brightness
		}
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

	if measurement.Role == "runtime:join" {
		var bidPrice, askPrice, tradePrice float64
		metrics := data.Metrics(measurement)

		if val, ok := metrics["best_price:bid"]; ok {
			bidPrice = val
		} else if val, ok := metrics["bid"]; ok {
			bidPrice = val
		}

		if val, ok := metrics["best_price:ask"]; ok {
			askPrice = val
		} else if val, ok := metrics["ask"]; ok {
			askPrice = val
		}

		if val, ok := metrics["price"]; ok {
			tradePrice = val
		}

		var price float64

		if bidPrice > 0 && askPrice > 0 {
			price = (bidPrice + askPrice) / 2.0
		}

		if price <= 0 && bidPrice > 0 {
			price = bidPrice
		}

		if price <= 0 && askPrice > 0 {
			price = askPrice
		}

		if price <= 0 && tradePrice > 0 {
			price = tradePrice
		}

		if price <= 0 && stage.lastPrice > 0 {
			price = stage.lastPrice
		}

		if price > 0 {
			stage.lastPrice = price

			if stage.initialPrice <= 0 {
				stage.initialPrice = price
			}
		}

		var netReturn float64

		if stage.initialPrice > 0 && price > 0 {
			netReturn = (price - stage.initialPrice) / stage.initialPrice * 100.0
		}

		stage.frames = append(stage.frames, &FrameData{
			Tick:        int64(measurement.Tick),
			Timestamp:   int64(measurement.Timestamp),
			ActiveToken: currentToken,
			Brightness:  brightness,
			Price:       price,
			Return:      netReturn,
		})
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
		"frames":          stage.frames,
	}
}
