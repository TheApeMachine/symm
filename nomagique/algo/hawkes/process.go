package hawkes

import (
	"sort"

	"github.com/theapemachine/errnie"
)

/*
Process owns symbol-local fit state for one serial event processor. Keeping
the mutable estimators with their single owner preserves event ordering without
adding a concurrent map that cannot protect estimator internals.
*/
type Process struct {
	symbols map[string]*symbol
}

/*
NewProcess returns a numerical Hawkes excitation process with isolated state
for every symbol it observes.
*/
func NewProcess() *Process {
	return &Process{symbols: make(map[string]*symbol)}
}

/*
Measure estimates the current empirical arrival rate or Hawkes state from one
chronological marked stream.
*/
func (process *Process) Measure(input Input) (Out, bool, error) {
	if input.Symbol == "" || len(input.Stream.BuyTimes())+len(input.Stream.SellTimes()) == 0 {
		return Out{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"excitation: invalid arrival stream",
			nil,
		))
	}

	_, latest, _ := input.Stream.Bounds()

	if input.Horizon.IsZero() || input.Horizon.Before(latest) {
		return Out{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"excitation: horizon must include the latest arrival",
			nil,
		))
	}

	if input.ObservedFrom.IsZero() {
		input.ObservedFrom = input.Stream.ObservationOrigin()
	}

	input.Stream = input.Stream.WithObservationOrigin(input.ObservedFrom)

	outcome, ready := process.symbol(input.Symbol).measure(
		input.Stream,
		input.Horizon,
	)

	return outcome, ready, nil
}

func (process *Process) symbol(symbolName string) *symbol {
	model, ok := process.symbols[symbolName]

	if ok {
		return model
	}

	model = newSymbol()
	process.symbols[symbolName] = model

	return model
}

/*
Symbols returns every symbol that has produced at least one excitation outcome.
*/
func (process *Process) Symbols() []string {
	if process == nil || len(process.symbols) == 0 {
		return nil
	}

	symbols := make([]string, 0, len(process.symbols))

	for symbolName := range process.symbols {
		symbols = append(symbols, symbolName)
	}

	sort.Strings(symbols)

	return symbols
}

/*
Outcome returns the latest measured Hawkes outcome for one symbol.
*/
func (process *Process) Outcome(symbolName string) (Out, bool) {
	if process == nil || symbolName == "" {
		return Out{}, false
	}

	model, ok := process.symbols[symbolName]

	if !ok || !model.lastReady {
		return Out{}, false
	}

	return model.lastOutcome, true
}

/*
AwaitFit waits for an in-flight asynchronous parameter fit for one symbol.
It returns whether a completed epoch is pending publication on the next Measure.
*/
func (process *Process) AwaitFit(symbolName string) bool {
	if process == nil || symbolName == "" {
		return false
	}

	model, ok := process.symbols[symbolName]

	if !ok {
		return false
	}

	return model.awaitFit()
}
