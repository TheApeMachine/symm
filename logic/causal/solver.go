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
	pearls   *sync.Map
	regimes  *sync.Map
	history  *sync.Map
	config   algorithm.PearlConfig
	ui       chan []byte
}

type causalResult struct {
	symbol string
	output map[string]any
	err    error
}

/*
NewSolver creates a typed causal solver.
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
		pearls:   &sync.Map{},
		regimes:  &sync.Map{},
		history:  &sync.Map{},
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
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "causal: parallel evaluation failed", err,
		))
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
		Key:          input.symbol,
		Row:          input.row,
		Inverted:     inverted,
		Contagion:    input.contagion,
		Condition:    input.condition,
		Intervention: input.intervention,
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

	return causalResult{
		symbol: input.symbol, output: output.Outputs(),
	}
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
	}

	return resolved
}

func (solver *Solver) appendHistory(symbol string, row []float64) {
	stored, _ := solver.history.LoadOrStore(symbol, [][]float64{})
	rows := stored.([][]float64)
	rows = append(rows, append([]float64(nil), row...))
	rowWidth := len(row)
	capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2

	if len(rows) > capacity {
		rows = rows[len(rows)-capacity:]
	}

	solver.history.Store(symbol, rows)
}

func (solver *Solver) historyRows(symbol string) [][]float64 {
	stored, _ := solver.history.LoadOrStore(symbol, [][]float64{})
	rows := stored.([][]float64)
	copied := make([][]float64, len(rows))

	for index, row := range rows {
		copied[index] = append([]float64(nil), row...)
	}

	return copied
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
	p, ok := solver.pearls.Load(symbol)

	if !ok {
		p = algorithm.NewPearl(solver.config)
		solver.pearls.Store(symbol, p)
	}

	return p.(*algorithm.Pearl)
}

/*
getRegime lazily gets or creates a Regime selector per symbol.
*/
func (solver *Solver) getRegime(symbol string) *causal.Regime {
	r, ok := solver.regimes.Load(symbol)

	if !ok {
		r = causal.NewRegime(causal.RegimeConfig{
			Target:         solver.config.Target,
			ConditionLeft:  0,
			ConditionRight: 1,
		})

		solver.regimes.Store(symbol, r)
	}

	return r.(*causal.Regime)
}

/*
Close cleans up the solver instance.
*/
func (solver *Solver) Close() error {
	solver.history = nil
	return nil
}
