package matrix_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
	"reflect"
	"testing"
)

func TestTransposeNext(t *testing.T) {
	node := matrix.NewTranspose[string]()
	original := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
	want := [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}}
	for range 3 {
		out := tests.Drain(t, node, tests.Values(original))
		tests.Sound(t, node)
		if !reflect.DeepEqual(out[0], want) {
			t.Fatal(out)
		}
		if !reflect.DeepEqual(node.Read(), want) {
			t.Fatal("Read lost result at terminal nil")
		}
		out[0].([][]string)[0][0] = "changed"
		if original[0][0] != "a" {
			t.Fatal("transpose mutated source")
		}
	}
	// Multiple matrix values in a fold yield the last transpose, not a truncated first matrix.
	out := tests.Drain(t, node, tests.Values(original, [][]string{{"last"}}))
	tests.Sound(t, node)
	if !reflect.DeepEqual(out[0], [][]string{{"last"}}) {
		t.Fatal(out)
	}
	out = tests.Drain(t, node, nil)
	tests.Sound(t, node)
	if !reflect.DeepEqual(out[0], [][]string{}) {
		t.Fatal(out)
	}
	bad := matrix.NewTranspose[float64]()
	tests.Drain(t, bad, tests.Values([][]float64{{1, 2}, {3}}))
	if !errors.Is(bad.Error(), core.ErrShape) {
		t.Fatal("ragged rows accepted")
	}
	wrong := matrix.NewTranspose[float64]()
	tests.Drain(t, wrong, tests.Values("wrong"))
	if !errors.Is(wrong.Error(), core.ErrWrongType) {
		t.Fatal("wrong input type lost")
	}
}
