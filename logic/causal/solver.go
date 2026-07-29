package causal

import (
	"math"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
Option configures the Causal solver.
*/
type Option func(*Solver)

/*
WithPearlConfig customizes Pearl's causal ladder configuration.
*/
func WithPearlConfig(config algorithm.PearlConfig) Option {
	return func(s *Solver) {
		s.config = config
	}
}

/*
Solver evaluates Judea Pearl's Causal Ladder over live market measurements,
physics fluid metrics, and predictive coding resonance predictions.
*/
type Solver struct {
	recorder *audit.Recorder
	mu       sync.RWMutex
	pearls   map[string]*algorithm.Pearl
	regimes  map[string]*causal.Regime
	config   algorithm.PearlConfig
}

/*
NewSolver creates a typed causal solver wired to audit recording.
Default layout (4-column row):
  - Col 0: Control 1 (Resonance System Energy)
  - Col 1: Control 2 (Resonance Surprise / Anomaly)
  - Col 2: Treatment (Resonance Task Prediction / Expected Return)
  - Col 3: Target (Realized Price Return)
*/
func NewSolver(recorder *audit.Recorder, opts ...Option) *Solver {
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		MinHistory:              12,
		History:                 64,
		KernelBandwidth:         0.35,
		CategoryIndexes:         []float64{1, 2, 3, 4},
		NonlinearCounterfactual: true,
	}

	solver := &Solver{
		recorder: recorder,
		pearls:   make(map[string]*algorithm.Pearl),
		regimes:  make(map[string]*causal.Regime),
		config:   defaultConfig,
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update extracts aligned causal rows from Thesis, evaluates regime switching and Pearl's
causal ladder (Association, Do-Intervention, Abductive Counterfactuals), and enriches thesis.Causal.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	symbols := solver.extractSymbols(thesis)
	if len(symbols) == 0 {
		symbols = []string{"default"}
	}

	for _, symbol := range symbols {
		row, intervention, contagion, condition, ok := solver.buildCausalRow(thesis, symbol)
		if !ok {
			continue
		}

		pearl := solver.getPearl(symbol)
		regime := solver.getRegime(symbol)

		// 1. Evaluate Causal Regime (Normal vs Inverted Market)
		regimeOut, err := regime.Measure(causal.RegimeInput{
			Rows:      [][]float64{row},
			Contagion: contagion,
		})
		inverted := false
		if err == nil && regimeOut.RawInverted > 0 {
			inverted = true
		}

		// 2. Evaluate Pearl's Causal Ladder (Association, Intervention, Counterfactuals)
		output, ready, err := pearl.Measure(algorithm.PearlInput{
			Key:          symbol,
			Row:          row,
			Inverted:     inverted,
			Contagion:    contagion,
			Condition:    condition,
			Intervention: intervention,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"causal: pearl evaluation failed",
				err,
			))
			continue
		}

		if !ready {
			continue // Waiting for MinHistory rows
		}

		// 3. Enrich thesis.Causal with Pearl outputs map
		thesis.Causal.Store(symbol, output.Outputs())

		// 4. Audit Record
		if solver.recorder != nil {
			auditData := output.Outputs()
			auditData["symbol"] = symbol
			solver.recorder.Write(auditData)
		}
	}

	return nil
}

/*
buildCausalRow constructs an aligned 4-column causal observation vector [C1, C2, T, Y]
from Thesis measurements and predictive coding outputs.
*/
func (solver *Solver) buildCausalRow(
	thesis *types.Thesis,
	symbol string,
) (row []float64, intervention float64, contagion float64, condition float64, ok bool) {
	// Extract Predictive Coding Resonance outputs
	var energy, surprise, taskPred float64

	if resRaw, found := thesis.Resonance.Load("resonance"); found {
		if resMap, isMap := resRaw.(map[string]any); isMap {
			if v, ok := resMap["energy"].(float64); ok {
				energy = v
			}
			if v, ok := resMap["surprise"].(float64); ok {
				surprise = v
			}
			if v, ok := resMap["taskPrediction"].([]float64); ok && len(v) > 0 {
				taskPred = v[0]
			} else if v, ok := resMap["forwardCurve"].([]float64); ok && len(v) > 0 {
				taskPred = v[0]
			}
		}
	}

	// Extract Realized Return / Target from Measurement Metrics (.Raw)
	var realizedReturn float64
	measurementCount := 0

	thesis.EachMeasurement(func(m *types.Measurement) bool {
		if m != nil && (m.Symbol == symbol || symbol == "default") {
			for _, metric := range m.Metrics {
				realizedReturn += metric.Raw
				measurementCount++
			}
		}
		return true
	})

	if measurementCount > 0 {
		realizedReturn /= float64(measurementCount)
	}

	// Validate numeric values
	if math.IsNaN(energy) || math.IsInf(energy, 0) {
		energy = 0.0
	}
	if math.IsNaN(surprise) || math.IsInf(surprise, 0) {
		surprise = 0.0
	}
	if math.IsNaN(taskPred) || math.IsInf(taskPred, 0) {
		taskPred = 0.0
	}

	// Construct 4-column Row: [Control1 (Energy), Control2 (Surprise), Treatment (TaskPred), Target (Return)]
	row = []float64{
		energy,         // Col 0: Control 1
		surprise,       // Col 1: Control 2
		taskPred,       // Col 2: Treatment
		realizedReturn, // Col 3: Target
	}

	intervention = taskPred // Desired interventional level
	contagion = surprise    // Surprise magnitude acts as contagion proxy
	condition = energy      // Energy acts as pair condition proxy

	return row, intervention, contagion, condition, true
}

/*
extractSymbols finds all distinct symbols in Thesis measurements.
*/
func (solver *Solver) extractSymbols(thesis *types.Thesis) []string {
	seen := make(map[string]struct{})
	thesis.EachMeasurement(func(m *types.Measurement) bool {
		if m != nil && m.Symbol != "" {
			seen[m.Symbol] = struct{}{}
		}
		return true
	})

	symbols := make([]string, 0, len(seen))
	for s := range seen {
		symbols = append(symbols, s)
	}
	return symbols
}

/*
getPearl lazily gets or creates a Pearl causal evaluator per symbol.
*/
func (solver *Solver) getPearl(symbol string) *algorithm.Pearl {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	p, ok := solver.pearls[symbol]
	if !ok {
		p = algorithm.NewPearl(solver.config)
		solver.pearls[symbol] = p
	}
	return p
}

/*
getRegime lazily gets or creates a Regime selector per symbol.
*/
func (solver *Solver) getRegime(symbol string) *causal.Regime {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	r, ok := solver.regimes[symbol]
	if !ok {
		r = causal.NewRegime(causal.RegimeConfig{
			Target:         solver.config.Target,
			MinHistory:     solver.config.MinHistory,
			ContagionBreak: 0.8,
			ConditionLeft:  0,
			ConditionRight: 1,
		})
		solver.regimes[symbol] = r
	}
	return r
}

/*
Close cleans up the solver instance.
*/
func (solver *Solver) Close() error {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	solver.pearls = nil
	solver.regimes = nil
	return nil
}
