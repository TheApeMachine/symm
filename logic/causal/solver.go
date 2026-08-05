package causal

import (
	"sync"

	"github.com/theapemachine/datura"
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
	history  map[string][][]float64
	config   algorithm.PearlConfig
	ui       chan []byte
}

type causalResult struct {
	symbol string
	output map[string]any
	err    error
}

/*
NewSolver creates a typed causal solver wired to audit recording.
Default layout (4-column row):
  - Col 0: Control 1 (Resonance System Energy)
  - Col 1: Control 2 (Resonance Surprise / Anomaly)
  - Col 2: Treatment (Resonance Task Prediction / Expected Return)
  - Col 3: Target (Realized Price Return)
*/
func NewSolver(ui chan []byte, recorder *audit.Recorder, opts ...Option) *Solver {
	defaultConfig := algorithm.PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	solver := &Solver{
		recorder: recorder,
		pearls:   make(map[string]*algorithm.Pearl),
		regimes:  make(map[string]*causal.Regime),
		history:  make(map[string][][]float64),
		config:   defaultConfig,
		ui:       ui,
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update extracts aligned causal rows from Thesis, evaluates regime
switching and Pearl's causal ladder (Association, Do-Intervention,
Abductive Counterfactuals), and enriches thesis.Causal.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	inputs := solver.buildCausalInputs(thesis)
	results := make([]causalResult, len(inputs))
	groups := make([]types.SymbolRows[causalInput], len(inputs))

	for index, input := range inputs {
		groups[index] = types.SymbolRows[causalInput]{Symbol: input.symbol, Rows: []causalInput{input}}
	}

	err := types.RunSymbolGroupsParallel(groups, func(index int, rows []causalInput) error {
		results[index] = solver.measure(rows[0])
		return nil
	})

	if err != nil {
		return err
	}

	solver.store(thesis, results)
	thesis.Readiness.Causal = true

	solver.publish(thesis)
	return nil
}

func (solver *Solver) measure(input causalInput) causalResult {
	solver.appendHistory(input.symbol, input.row)

	regimeOut, err := solver.getRegime(input.symbol).Measure(causal.RegimeInput{
		Rows:      [][]float64{input.row},
		Contagion: input.contagion,
	})
	inverted := err == nil && regimeOut.RawInverted > 0
	output, ready, err := solver.getPearl(input.symbol).Measure(algorithm.PearlInput{
		Key: input.symbol, Row: input.row, Inverted: inverted,
		Contagion: input.contagion, Condition: input.condition, Intervention: input.intervention,
	})

	if err != nil {
		return causalResult{
			symbol: input.symbol,
			err: errnie.Err(
				errnie.UnprocessableContent,
				"causal: pearl evaluation failed",
				err,
			),
		}
	}

	if !ready {
		return causalResult{symbol: input.symbol}
	}

	return causalResult{symbol: input.symbol, output: output.Outputs()}
}

func (solver *Solver) store(thesis *types.Thesis, results []causalResult) bool {
	resolved := false

	for _, result := range results {
		if result.err != nil {
			errnie.Error(result.err)

			continue
		}

		if result.output == nil {
			continue
		}

		result.output["historyRows"] = solver.historyRows(result.symbol)
		thesis.Causal.Store(result.symbol, result.output)
		resolved = true

		if solver.recorder != nil {
			result.output["symbol"] = result.symbol
			result.output["stage"] = "causal"
			errnie.Error(audit.Record(solver.recorder, "predictive", result.output))
		}
	}

	return resolved
}

func (solver *Solver) appendHistory(symbol string, row []float64) {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	rows := append(solver.history[symbol], append([]float64(nil), row...))
	rowWidth := len(row)
	capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2

	if len(rows) > capacity {
		rows = rows[len(rows)-capacity:]
	}

	solver.history[symbol] = rows
}

func (solver *Solver) historyRows(symbol string) [][]float64 {
	solver.mu.RLock()
	defer solver.mu.RUnlock()

	stored := solver.history[symbol]
	rows := make([][]float64, len(stored))

	for index, row := range stored {
		rows[index] = append([]float64(nil), row...)
	}

	return rows
}

/*
publish emits one causal wire frame per symbol observed on this tick.
*/
func (solver *Solver) publish(thesis *types.Thesis) {
	if solver.ui == nil || thesis == nil {
		return
	}

	rows := make([]map[string]any, 0)

	thesis.Causal.Range(func(key, value any) bool {
		causalMap, ok := value.(map[string]any)
		symbol, symbolOK := key.(string)

		if !ok || !symbolOK || symbol == "" {
			return true
		}

		if _, present := causalMap["symbol"]; !present {
			causalMap["symbol"] = symbol
		}

		rows = append(rows, causalMap)
		return true
	})

	if len(rows) > 0 {
		select {
		case solver.ui <- datura.NewMap("causal", rows).MarshalAndFree():
		default:
		}
	}
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

	solver.history = nil
	return nil
}
