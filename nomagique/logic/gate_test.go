package logic

import (
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGate(t *testing.T) {
	for _, test := range []struct{ condition, want float64 }{{0, 0}, {1, 3}} {
		mapping := types.NewMap[string, types.Value[float64]]()
		mapping.Put("condition", types.NewValue(test.condition))
		mapping.Put("value", types.NewValue(3.0))
		input := types.NewInput(types.NewValue(mapping))
		output := nomagique.Number(input, NewGate(input))
		if output.Error() != "" {
			t.Fatal(output.Error())
		}
		result, _ := output.Project().Read().Get("result")
		if result.Read() != test.want {
			t.Fatalf("condition=%v result=%v; want %v", test.condition, result.Read(), test.want)
		}
	}
}

func TestGateUsesOnlyInitialAndNext(t *testing.T) {
	structure := reflect.TypeOf(Gate{})
	if structure.NumField() != 2 || structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
		t.Fatal("Gate must contain only initial and next")
	}
}
