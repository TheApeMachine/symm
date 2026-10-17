package audit

import (
	"context"
	"fmt"
	"maps"
	"math"
	"math/rand"
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
Lower entropy than a block-shuffled copy of the same tokens is structure.
Entropy that matches the shuffled copy is chance. One token taking over the tape is a frozen state.
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
	tokens       map[string][]string
	entropy      float64
	maxEntropy   float64
	nullEntropy  float64
	nullPValue   float64
	blockSize    int
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
		tokens:       make(map[string][]string),
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
	regions, err := stage.grid.Observe(measurement)

	if err != nil {
		stage.err = err
		return
	}

	if len(regions) == 0 || regions[0].Brightness <= 0 {
		return
	}
	currentToken := regionToken(regions[0].Key)
	brightness := make([]float64, nmstore.RegionCount)

	for _, reg := range regions {
		brightness[reg.Key] = reg.Brightness
	}

	stage.totalTokens++
	stage.tokenCounts[currentToken]++
	stage.tokens[symbol] = append(stage.tokens[symbol], currentToken)

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

	// 2. Transition entropy against a block-shuffled copy of the same tokens.
	// The block length is the median dwell, so the copy keeps local persistence
	// and only the order of those blocks is chance.
	stage.entropy = transitionEntropy(stage.transitions, stage.tokenCounts, stage.totalTokens)
	distinctStates := len(stage.tokenCounts)

	if distinctStates > 1 {
		stage.maxEntropy = math.Log2(float64(distinctStates))
	}

	stage.blockSize = dwellBlockSize(stage.tokens)
	nullEntropies := blockNullEntropies(stage.tokens, stage.blockSize, cfg.Permutations)
	stage.nullPValue = lowerTailP(stage.entropy, nullEntropies)

	for _, value := range nullEntropies {
		stage.nullEntropy += value
	}

	if len(nullEntropies) > 0 {
		stage.nullEntropy /= float64(len(nullEntropies))
	}

	if len(nullEntropies) > 0 && stage.nullPValue > cfg.Significance {
		stage.violations = append(stage.violations, NewViolation(
			"dynamics",
			"dynamics.no_structure_vs_null",
			fmt.Sprintf("Token order matches a shuffled copy of the same tokens: entropy %.3f bits, shuffled mean %.3f bits (p = %.3f)",
				stage.entropy, stage.nullEntropy, stage.nullPValue),
			nil,
			nil,
		))
	}
}

func transitionEntropy(transitions map[string]map[string]int64, tokenCounts map[string]int64, total int) float64 {
	if total == 0 {
		return 0
	}

	keys := make([]string, 0, len(tokenCounts))

	for key := range tokenCounts {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	var entropy float64

	for _, stateI := range keys {
		row, hasRow := transitions[stateI]

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

		probI := float64(tokenCounts[stateI]) / float64(total)
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

/*
dwellBlockSize is the median run of the same token inside one symbol. The
shuffled copy keeps runs of this length.
*/
func dwellBlockSize(tokensBySymbol map[string][]string) int {
	runs := make([]int, 0)

	for _, tokens := range tokensBySymbol {
		if len(tokens) == 0 {
			continue
		}

		run := 1

		for index := 1; index < len(tokens); index++ {
			if tokens[index] == tokens[index-1] {
				run++
				continue
			}

			runs = append(runs, run)
			run = 1
		}

		runs = append(runs, run)
	}

	if len(runs) == 0 {
		return 1
	}

	slices.Sort(runs)
	median := runs[len(runs)/2]

	if median < 1 {
		return 1
	}

	return median
}

/*
blockNullEntropies is the transition entropy of each block-shuffle of the
observed tokens. Seed 1791 makes a rerun reproduce.
*/
func blockNullEntropies(tokensBySymbol map[string][]string, blockSize, iterations int) []float64 {
	if iterations <= 0 || len(tokensBySymbol) == 0 {
		return nil
	}

	if blockSize < 1 {
		blockSize = 1
	}

	symbols := slices.Sorted(maps.Keys(tokensBySymbol))
	blocksBySymbol := make(map[string][][]string, len(symbols))
	total := 0

	for _, symbol := range symbols {
		tokens := tokensBySymbol[symbol]
		total += len(tokens)

		if len(tokens) == 0 {
			continue
		}

		blocks := make([][]string, 0, (len(tokens)+blockSize-1)/blockSize)

		for start := 0; start < len(tokens); start += blockSize {
			end := min(start+blockSize, len(tokens))
			blocks = append(blocks, append([]string(nil), tokens[start:end]...))
		}

		blocksBySymbol[symbol] = blocks
	}

	if total < 2 {
		return nil
	}

	rng := rand.New(rand.NewSource(1791))
	entropies := make([]float64, 0, iterations)

	for range iterations {
		shuffled := make(map[string][]string, len(blocksBySymbol))

		for _, symbol := range symbols {
			blocks := blocksBySymbol[symbol]

			if len(blocks) == 0 {
				continue
			}

			order := append([][]string(nil), blocks...)
			rng.Shuffle(len(order), func(i, j int) {
				order[i], order[j] = order[j], order[i]
			})

			flat := make([]string, 0, len(tokensBySymbol[symbol]))

			for _, block := range order {
				flat = append(flat, block...)
			}

			shuffled[symbol] = flat
		}

		entropies = append(entropies, sequenceEntropy(shuffled))
	}

	return entropies
}

func sequenceEntropy(tokensBySymbol map[string][]string) float64 {
	transitions := make(map[string]map[string]int64)
	counts := make(map[string]int64)
	var total int

	for _, tokens := range tokensBySymbol {
		var prior string
		var hasPrior bool

		for _, token := range tokens {
			total++
			counts[token]++

			if hasPrior {
				row := transitions[prior]

				if row == nil {
					row = make(map[string]int64)
					transitions[prior] = row
				}

				row[token]++
			}

			prior = token
			hasPrior = true
		}
	}

	return transitionEntropy(transitions, counts, total)
}

/*
lowerTailP is (1 + shuffles at or below observed) / (shuffles + 1). A small
value means the real entropy is lower than the shuffled copies.
*/
func lowerTailP(observed float64, null []float64) float64 {
	if len(null) == 0 {
		return 1
	}

	below := 0

	for _, value := range null {
		if value <= observed {
			below++
		}
	}

	return float64(1+below) / float64(len(null)+1)
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
		"passed":                  len(stage.violations) == 0,
		"entropy":                 stage.entropy,
		"max_entropy":             stage.maxEntropy,
		"null_transition_entropy": stage.nullEntropy,
		"entropy_reduction":       stage.nullEntropy - stage.entropy,
		"null_p_value":            stage.nullPValue,
		"null_block_size":         stage.blockSize,
		"total_tokens":            stage.totalTokens,
		"distinct_tokens":         len(stage.tokenCounts),
		"token_counts":            stage.tokenCounts,
		"transitions":             stage.transitions,
		"frames":                  stage.frames,
	}
}
