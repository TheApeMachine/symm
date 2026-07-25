package logic

import (
	"reflect"

	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
stateReplay reads the optional manifold replay marker without requiring a fixed
State field layout at compile time.
*/
func stateReplay(state manifold.State) bool {
	value := reflect.ValueOf(state)
	field := value.FieldByName("Replay")
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	return field.Bool()
}

/*
withStateReplay returns a copy of state with the optional replay marker set when
that field exists on this manifold.State build.
*/
func withStateReplay(state manifold.State, replay bool) manifold.State {
	value := reflect.ValueOf(&state).Elem()
	field := value.FieldByName("Replay")
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
		field.SetBool(replay)
	}
	return state
}

/*
isReplayCognitionState reports replay for cognition, preferring the manifold
marker and falling back to same-timestamp cached cognition when unavailable.
*/
func (analyzer *Analyzer) isReplayCognitionState(
	thesis *types.Thesis,
	state manifold.State,
) bool {
	if stateReplay(state) {
		return true
	}
	if thesis == nil {
		return false
	}
	value, found := thesis.Cognition.Load(state.Symbol)
	if !found {
		return false
	}
	reading, ok := value.(types.Cognition)
	if !ok {
		return false
	}
	return reading.At.Equal(state.At)
}
