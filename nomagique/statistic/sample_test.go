package statistic

import (
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestMedianAndMaximum(t *testing.T) {
	mapping := sampleValues(7, 1, 5, 3)
	input := types.NewInput(types.NewValue(mapping))
	median := nomagique.Number(input, NewMedian(input))
	if median.Error() != "" {
		t.Fatal(median.Error())
	}
	if got, _ := median.Project().Read().Get("result"); got.Read() != 4 {
		t.Fatalf("median=%v; want 4", got.Read())
	}

	input = types.NewInput(types.NewValue(mapping))
	maximum := nomagique.Number(input, NewMaximum(input))
	if maximum.Error() != "" {
		t.Fatal(maximum.Error())
	}
	if got, _ := maximum.Project().Read().Get("result"); got.Read() != 7 {
		t.Fatalf("maximum=%v; want 7", got.Read())
	}
}

func TestMedianEmptyIsProvisional(t *testing.T) {
	mapping := types.NewMap[string, types.Value[float64]]()
	input := types.NewInput(types.NewValue(mapping))
	output := nomagique.Number(input, NewMedian(input))
	if output.Error() != "" {
		t.Fatal(output.Error())
	}
	ready, _ := output.Project().Read().Get("ready")
	if ready.Read() != 0 {
		t.Fatalf("ready=%v; want 0", ready.Read())
	}
}

func TestSampleStatisticsUseOnlyInitialAndNext(t *testing.T) {
	for _, value := range []any{Median{}, Maximum{}} {
		structure := reflect.TypeOf(value)
		if structure.NumField() != 2 || structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
			t.Fatalf("%s must contain only initial and next", structure.Name())
		}
	}
}

func sampleValues(values ...float64) types.Map[string, types.Value[float64]] {
	mapping := types.NewMap[string, types.Value[float64]]()
	for index, value := range values {
		mapping.Put("sample/"+string(rune('0'+index)), types.NewValue(value))
	}
	return mapping
}
