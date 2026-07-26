package logic

import (
	"reflect"

	"github.com/theapemachine/symm/logic/manifold"
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
