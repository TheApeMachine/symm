package calculus

import (
	"math"
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestScalarPrimitiveShapes(t *testing.T) {
	for _, value := range []any{
		Sum{}, Difference{}, Positive{}, Product{}, LogRatio{},
		Squash{}, Inverse{}, Ratio{},
	} {
		structure := reflect.TypeOf(value)
		if structure.NumField() != 2 || structure.Field(0).Name != "initial" ||
			structure.Field(1).Name != "next" {
			t.Fatalf("%s must contain only initial and next", structure.Name())
		}
	}
}

func TestScalarPrimitives(t *testing.T) {
	tests := []struct {
		name      string
		params    scalarMap
		primitive func(types.Input[scalarMap]) types.IO[scalarMap]
		want      float64
	}{
		{"sum", scalarParams("left", 3, "right", 4), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewSum(in) }, 7},
		{"difference", scalarParams("left", 3, "right", 4), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewDifference(in) }, -1},
		{"positive", scalarParams("value", -3), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewPositive(in) }, 0},
		{"product", scalarParams("left", 3, "right", 4), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewProduct(in) }, 12},
		{"log ratio", scalarParams("current", 110, "previous", 100), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewLogRatio(in) }, math.Log(1.1)},
		{"squash", scalarParams("value", 2, "scale", 2), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewSquash(in) }, 0.5},
		{"inverse", scalarParams("value", 2, "scale", 2), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewInverse(in) }, 0.5},
		{"ratio", scalarParams("value", 2, "baseline", 4, "ready", 1), func(in types.Input[scalarMap]) types.IO[scalarMap] { return NewRatio(in) }, 0.5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := types.NewInput(types.NewValue(test.params))
			output := nomagique.Number(input, test.primitive(input))
			if output.Error() != "" {
				t.Fatal(output.Error())
			}
			result, found := output.Project().Read().Get("result")
			if !found {
				t.Fatal("result is missing")
			}
			if math.Abs(result.Read()-test.want) > 1e-12 {
				t.Fatalf("result=%v; want %v", result.Read(), test.want)
			}
		})
	}
}

func TestEvidenceBoundaryPrimitives(t *testing.T) {
	inverse := scalarParams("value", 0, "scale", 0)
	input := types.NewInput(types.NewValue(inverse))
	out := nomagique.Number(input, NewInverse(input))
	if got, _ := out.Project().Read().Get("result"); got.Read() != 1 {
		t.Fatalf("inverse absence=%v; want 1", got.Read())
	}

	ratio := scalarParams("value", 2, "baseline", 4, "ready", 0)
	input = types.NewInput(types.NewValue(ratio))
	out = nomagique.Number(input, NewRatio(input))
	if got, _ := out.Project().Read().Get("result"); got.Read() != 0 {
		t.Fatalf("unready ratio=%v; want 0", got.Read())
	}

	squash := scalarParams("value", 2, "scale", 0)
	input = types.NewInput(types.NewValue(squash))
	out = nomagique.Number(input, NewSquash(input))
	if got, _ := out.Project().Read().Get("result"); got.Read() != 0 {
		t.Fatalf("unscaled squash=%v; want 0", got.Read())
	}
}

func scalarParams(values ...any) scalarMap {
	mapping := types.NewMap[string, types.Value[float64]]()
	for index := 0; index+1 < len(values); index += 2 {
		key := values[index].(string)
		var value float64
		switch number := values[index+1].(type) {
		case int:
			value = float64(number)
		case float64:
			value = number
		default:
			panic("scalarParams requires numeric values")
		}
		mapping.Put(key, types.NewValue(value))
	}
	return mapping
}
