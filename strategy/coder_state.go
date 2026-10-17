package strategy

import (
	"fmt"
	"io"
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

/*
Coder state (data.CoderState): what the chord gate learned per symbol, the
solver's coder and the gate's warm-up and surprise moments, so learning
continues across trainings and from train into make run instead of starting
cold. Train continues the newest state trained under the same gate settings
and saves its own under the policy epoch it publishes; make run continues
the state of the policy epoch it trades, so its gate reads exactly as train
would have read the same ticks next.
*/

/*
stateSource is the read side a coder state is loaded through: train's
store.Client or make run's read-only store.Reader.
*/
type stateSource struct {
	keys     func(prefixes ...string) ([]string, error)
	download func(keys []string) iter.Seq2[*data.Value, error]
}

/*
momentsVector lays moments out as data.CoderStateKeys, with the number of
pending predictions the coder dropped.
*/
func momentsVector(moments surpriseMoments, dropped int) []float64 {
	warm := 0.0

	if moments.settle.warm {
		warm = 1
	}

	return []float64{
		float64(dropped), float64(moments.settle.steps), moments.settle.fast, moments.settle.slow, warm,
		float64(moments.count), moments.mean, moments.m2,
	}
}

/*
momentsOf reads a stored state's gate moments, at the rates of settings.
*/
func momentsOf(state data.CoderState, settings data.PolicyGate) (surpriseMoments, error) {
	fields := state.Fields

	for _, key := range []string{"settle_steps", "surprise_count", "pending_dropped"} {
		if value := fields[key]; value < 0 || value != math.Trunc(value) {
			return surpriseMoments{}, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[gate] %s's coder state %s is %g, not a count", state.Symbol, key, value,
			), nil)
		}
	}

	if warm := fields["warm"]; warm != 0 && warm != 1 {
		return surpriseMoments{}, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"[gate] %s's coder state warm is %g, not 0 or 1", state.Symbol, warm,
		), nil)
	}

	return surpriseMoments{
		settle: settling{
			fastRate: settings.Warmup.FastRate, slowRate: settings.Warmup.SlowRate,
			steps: int(fields["settle_steps"]), fast: fields["settle_fast"], slow: fields["settle_slow"], warm: fields["warm"] == 1,
		},
		count: int(fields["surprise_count"]), mean: fields["surprise_mean"], m2: fields["surprise_m2"],
	}, nil
}

/*
seed loads states into the gate's (fresh) solver and keeps their moments
for the symbols' tapes (newSymbolTape). A state trained under other gate
settings, or that does not fit the solver, is an error.
*/
func (gate *chordGate) seed(states []data.CoderState) error {
	if gate.solver == nil {
		return errnie.Err(errnie.Validation, "[gate] coder states for a gate that is off", nil)
	}

	gate.carried = make(map[string]surpriseMoments, len(states))

	for _, state := range states {
		if !state.Gate.Same(gate.settings) {
			return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[gate] %s's coder state was trained under %s, this gate is %s", state.Symbol, state.Gate, gate.settings,
			), nil)
		}

		moments, err := momentsOf(state, gate.settings)

		if err != nil {
			return err
		}

		if err := gate.solver.LoadSymbol(state.Symbol, state.Blob); err != nil {
			return err
		}

		gate.carried[state.Symbol] = moments
	}

	return nil
}

/*
save writes one coder state per symbol the gate's solver holds into w,
under settings' policy epoch at lastTick, each symbol's moments read from
tapes (or, for one carried in but not stepped, as it was carried). Every
symbol's pending predictions are dropped, never resolved across the gap;
the count is stored and logged.
*/
func (gate *chordGate) save(w io.Writer, settings data.PolicyGate, lastTick uint64, tapes map[string]*symbolTape) error {
	if gate.solver == nil {
		return nil
	}

	symbols, dropped := 0, 0

	for _, symbol := range gate.solver.Symbols() {
		moments, carried := gate.carried[symbol]

		if tape, ok := tapes[symbol]; ok {
			moments = tape.surprise
		} else if !carried {
			return errnie.Err(errnie.Internal, "[gate] "+symbol+" has a coder but neither a tape nor carried moments", nil)
		}

		blob, pending, err := gate.solver.SaveSymbol(symbol)

		if err != nil {
			return err
		}

		value := data.NewValue(data.CoderOrigin, data.CoderStateRole, symbol)
		value.Epoch, value.Tick = settings.Epoch, lastTick
		value.FormatPrefix()

		if err := data.AppendCoderState(value, momentsVector(moments, pending), blob, settings); err != nil {
			return err
		}

		if _, err := io.Copy(w, value); err != nil {
			return errnie.Err(errnie.IO, "[gate] writing "+value.Prefix, err)
		}

		symbols++
		dropped += pending
	}

	errnie.Info(fmt.Sprintf(
		"[gate] saved the coder state of %d symbols under policy epoch %d; dropped %d pending predictions (never resolved across the gap)",
		symbols, settings.Epoch, dropped,
	))

	return nil
}

/*
coderStateKeys lists every stored coder state key by policy epoch.
*/
func coderStateKeys(source stateSource) (map[uint64][]string, error) {
	keys, err := source.keys(data.CoderStatesPrefix())

	if err != nil {
		return nil, errnie.Err(errnie.IO, "[gate] listing "+data.CoderStatesPrefix(), err)
	}

	byEpoch := make(map[uint64][]string)

	for _, key := range keys {
		epoch, err := data.CoderStateEpoch(key)

		if err != nil {
			return nil, err
		}

		byEpoch[epoch] = append(byEpoch[epoch], key)
	}

	return byEpoch, nil
}

/*
readCoderStates reads the states under keys, every one of policy epoch.
*/
func readCoderStates(source stateSource, epoch uint64, keys []string) ([]data.CoderState, error) {
	states := make([]data.CoderState, 0, len(keys))

	for value, err := range source.download(keys) {
		if err != nil {
			return nil, err
		}

		state, err := data.ReadCoderState(value)

		if err != nil {
			return nil, err
		}

		if state.Gate.Epoch != epoch || value.Epoch != epoch {
			return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[gate] coder state %s is tagged policy epoch %d under policy epoch %d", value.Prefix, state.Gate.Epoch, epoch,
			), nil)
		}

		states = append(states, state)
	}

	if len(states) != len(keys) {
		return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"[gate] policy epoch %d lists %d coder states and reads %d", epoch, len(keys), len(states),
		), nil)
	}

	return states, nil
}

/*
newestCoderStates are the coder states of the newest policy epoch trained
under settings (train continues them): skipped counts the newer epochs
trained under other settings. None is a cold start (nil, 0); an unreadable
state is an error.
*/
func newestCoderStates(source stateSource, settings data.PolicyGate) (states []data.CoderState, epoch uint64, skipped int, err error) {
	byEpoch, err := coderStateKeys(source)

	if err != nil {
		return nil, 0, 0, err
	}

	epochs := slices.Sorted(func(yield func(uint64) bool) {
		for epoch := range byEpoch {
			if !yield(epoch) {
				return
			}
		}
	})

	for _, epoch := range slices.Backward(epochs) {
		keys := byEpoch[epoch]
		first, err := readCoderStates(source, epoch, keys[:1])

		if err != nil {
			return nil, 0, 0, err
		}

		if !first[0].Gate.Same(settings) {
			skipped++
			continue
		}

		states, err := readCoderStates(source, epoch, keys)
		return states, epoch, skipped, err
	}

	return nil, 0, skipped, nil
}

/*
policyCoderStates are the coder states of policy epoch trained under gate
(make run continues them). None is (nil, nil): the epoch was trained before
coder states were saved.
*/
func policyCoderStates(source stateSource, gate data.PolicyGate) ([]data.CoderState, error) {
	byEpoch, err := coderStateKeys(source)

	if err != nil {
		return nil, err
	}

	keys := byEpoch[gate.Epoch]

	if len(keys) == 0 {
		return nil, nil
	}

	states, err := readCoderStates(source, gate.Epoch, keys)

	if err != nil {
		return nil, err
	}

	for _, state := range states {
		if !state.Gate.Same(gate) || state.Gate.TickEpoch != gate.TickEpoch {
			return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[gate] %s's coder state of policy epoch %d was trained under %s, its policies under %s",
				state.Symbol, gate.Epoch, state.Gate, gate,
			), nil)
		}
	}

	return states, nil
}
