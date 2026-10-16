package audit

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
FrameData records one symbol's tick frame: region brightness, the active
token, and the touch midpoint with its return since the symbol's first
midpoint. Price and Return are absent (null) when the tick recorded no
two-sided touch.
*/
type FrameData struct {
	Symbol      string    `json:"symbol"`
	Tick        int64     `json:"tick"`
	Timestamp   int64     `json:"timestamp"`
	ActiveToken string    `json:"active_token"`
	Brightness  []float64 `json:"brightness"`
	Price       *float64  `json:"price"`
	Return      *float64  `json:"return"`
}

/*
DynamicsStage audits token dynamics and state transition entropy across continuous market tape.
Each symbol is its own token sequence; transitions never cross symbols. It
builds the empirical transition matrix and the transition entropy H:
H = - sum_i P(i) sum_j P(j|i) log2 P(j|i)
Lower entropy signifies structured, predictable market state sequences.
Excessive entropy indicates random noise, while extreme token concentration indicates a dead/frozen state.
*/
type DynamicsStage struct {
	*runtime.System
	ch           chan *data.Value
	violations   []*Violation
	grid         *nmstore.Grid
	lastToken    map[string]string
	initialPrice map[string]float64
	totalTokens  int
	transitions  map[string]map[string]int64
	tokenCounts  map[string]int64
	frames       []*FrameData
	entropy      float64
	maxEntropy   float64
	err          error
}

func NewDynamicsStage(ctx context.Context) *DynamicsStage {
	return &DynamicsStage{
		System:       runtime.NewSystem(ctx, "audit:dynamics"),
		ch:           make(chan *data.Value, 64),
		violations:   make([]*Violation, 0),
		grid:         nmstore.NewGrid(ctx),
		lastToken:    make(map[string]string),
		initialPrice: make(map[string]float64),
		transitions:  make(map[string]map[string]int64),
		tokenCounts:  make(map[string]int64),
		frames:       make([]*FrameData, 0),
	}
}

func (stage *DynamicsStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *DynamicsStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		if stage.err != nil {
			continue
		}

		if measurement == nil {
			stage.err = errnie.Err(errnie.Validation, stage.Name()+": nil measurement on the tape", nil)
			continue
		}

		if measurement.Role != "runtime:join" {
			continue
		}

		stage.recordToken(measurement)
	}

	if stage.err != nil {
		return stage.err
	}

	stage.auditDynamics()
	return nil
}

func (stage *DynamicsStage) Violations() []*Violation {
	return stage.violations
}

/*
recordToken extracts the active state token of the symbol's frame and
accumulates the transition from that symbol's prior token.
*/
func (stage *DynamicsStage) recordToken(measurement *data.Value) {
	symbol := measurement.Scope
	frame, err := stage.grid.Observe(measurement, symbol)

	if err != nil {
		stage.err = err
		return
	}

	if len(frame.Regions) == 0 || frame.Regions[0].Brightness <= 0 {
		return
	}

	regions := frame.Regions
	currentToken := regionToken(regions[0].Key)
	brightness := make([]float64, nmstore.RegionCount)

	for _, reg := range regions {
		brightness[reg.Key] = reg.Brightness
	}

	stage.totalTokens++
	stage.tokenCounts[currentToken]++

	if priorToken, ok := stage.lastToken[symbol]; ok {
		row, exists := stage.transitions[priorToken]

		if !exists {
			row = make(map[string]int64)
			stage.transitions[priorToken] = row
		}

		row[currentToken]++
	}

	stage.lastToken[symbol] = currentToken

	record := &FrameData{
		Symbol:      symbol,
		Tick:        int64(measurement.Tick),
		Timestamp:   int64(measurement.Timestamp),
		ActiveToken: currentToken,
		Brightness:  brightness,
	}

	if bid, ask, ok := Touch(measurement); ok {
		price := (bid + ask) / 2.0

		if _, seen := stage.initialPrice[symbol]; !seen {
			stage.initialPrice[symbol] = price
		}

		netReturn := (price - stage.initialPrice[symbol]) / stage.initialPrice[symbol] * 100.0
		record.Price, record.Return = &price, &netReturn
	}

	stage.frames = append(stage.frames, record)
}

/*
auditDynamics evaluates state transition entropy and checks for absorbing state collapses.
*/
func (stage *DynamicsStage) auditDynamics() {
	cfg := system.Cfg.Audit

	if stage.totalTokens < cfg.MinTokens {
		stage.violations = append(stage.violations, insufficient("dynamics", fmt.Sprintf(
			"%d tokens; need %d", stage.totalTokens, cfg.MinTokens,
		)))
		return
	}

	// 1. Absorbing state collapse: one token dominating the emissions.
	for _, token := range slices.Sorted(maps.Keys(stage.tokenCounts)) {
		share := float64(stage.tokenCounts[token]) / float64(stage.totalTokens)

		if share > cfg.DominanceShare {
			stage.violations = append(stage.violations, NewViolation(
				token,
				"dynamics.absorbing_token_collapse",
				fmt.Sprintf("Token %s dominates %.1f%% of all emissions (> %.0f%% threshold)",
					token, share*100, cfg.DominanceShare*100),
				nil,
				nil,
			))
		}
	}

	// 2. Transition entropy against the maximum for the observed states.
	stage.entropy = stage.computeTransitionEntropy()
	distinctStates := len(stage.tokenCounts)

	if distinctStates > 1 {
		stage.maxEntropy = math.Log2(float64(distinctStates))

		if math.Abs(stage.entropy-stage.maxEntropy) < cfg.EntropyTolerance {
			stage.violations = append(stage.violations, NewViolation(
				"dynamics",
				"dynamics.pure_noise_entropy",
				fmt.Sprintf("Transition entropy H = %.3f bits equals maximum random entropy %.3f bits (zero predictive structure)",
					stage.entropy, stage.maxEntropy),
				nil,
				nil,
			))
		}
	}
}

func (stage *DynamicsStage) computeTransitionEntropy() float64 {
	if stage.totalTokens == 0 {
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

		probI := float64(stage.tokenCounts[stateI]) / float64(stage.totalTokens)
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
		"total_tokens":    stage.totalTokens,
		"distinct_tokens": len(stage.tokenCounts),
		"token_counts":    stage.tokenCounts,
		"transitions":     stage.transitions,
		"frames":          stage.frames,
	}
}
