package probability

import (
	"math"
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGeomean(t *testing.T) {
	mapping := types.NewMap[string, types.Value[float64]]()
	mapping.Put("sample/0", types.NewValue(4.0))
	mapping.Put("sample/1", types.NewValue(9.0))
	input := types.NewInput(types.NewValue(mapping))
	output := nomagique.Number(input, NewGeomean(input))
	if output.Error() != "" {
		t.Fatal(output.Error())
	}
	result, found := output.Project().Read().Get("result")
	if !found || math.Abs(result.Read()-6) > 1e-12 {
		t.Fatalf("geomean=%v; want 6", result.Read())
	}
}

func TestGeomeanRejectsInvalidEvidence(t *testing.T) {
	mapping := types.NewMap[string, types.Value[float64]]()
	mapping.Put("sample/0", types.NewValue(math.Inf(1)))
	input := types.NewInput(types.NewValue(mapping))
	if output := nomagique.Number(input, NewGeomean(input)); output.Error() == "" {
		t.Fatal("non-finite evidence should fail")
	}
}

func TestGeomeanUsesOnlyInitialAndNext(t *testing.T) {
	structure := reflect.TypeOf(Geomean{})
	if structure.NumField() != 2 || structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
		t.Fatal("Geomean must contain only initial and next")
	}
}
