package temporal

import (
	"math"
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestDuration(t *testing.T) {
	mapping := types.NewMap[string, types.Value[float64]]()
	mapping.Put("current_sec", types.NewValue(101.0))
	mapping.Put("current_nsec", types.NewValue(100_000_000.0))
	mapping.Put("previous_sec", types.NewValue(100.0))
	mapping.Put("previous_nsec", types.NewValue(900_000_000.0))
	input := types.NewInput(types.NewValue(mapping))
	output := nomagique.Number(input, NewDuration(input))
	if output.Error() != "" {
		t.Fatal(output.Error())
	}
	delta, _ := output.Project().Read().Get("delta")
	if math.Abs(delta.Read()-0.2) > 1e-12 {
		t.Fatalf("delta=%v; want 0.2", delta.Read())
	}
}

func TestDurationUsesOnlyInitialAndNext(t *testing.T) {
	structure := reflect.TypeOf(Duration{})
	if structure.NumField() != 2 || structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
		t.Fatal("Duration must contain only initial and next")
	}
}
