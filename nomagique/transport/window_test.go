package transport

import (
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestWindowRetainsBoundedRing(t *testing.T) {
	mapping := types.NewMap[string, types.Value[float64]]()
	mapping.Put("capacity", types.NewValue(3.0))

	for _, sample := range []float64{1, 2, 3, 4} {
		mapping.Put("sample", types.NewValue(sample))
		input := types.NewInput(types.NewValue(mapping))
		output := nomagique.Number(input, NewWindow(input))
		if output.Error() != "" {
			t.Fatal(output.Error())
		}
		mapping = output.Project().Read()
	}

	count, _ := mapping.Get("count")
	head, _ := mapping.Get("head")
	if count.Read() != 3 || head.Read() != 1 {
		t.Fatalf("count=%v head=%v; want 3 and 1", count.Read(), head.Read())
	}
	want := map[string]float64{"sample/0": 4, "sample/1": 2, "sample/2": 3}
	for key, expected := range want {
		value, found := mapping.Get(key)
		if !found || value.Read() != expected {
			t.Fatalf("%s=%v; want %v", key, value.Read(), expected)
		}
	}
}

func TestWindowUsesOnlyInitialAndNext(t *testing.T) {
	structure := reflect.TypeOf(Window{})
	if structure.NumField() != 2 || structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
		t.Fatal("Window must contain only initial and next")
	}
}
