package logic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestGate(t *testing.T) {
	for _, testCase := range []struct {
		condition float64
		want      float64
	}{
		{condition: 0, want: 0},
		{condition: 1, want: 3},
	} {
		input := nomagique.Frame{}
		input.Put(SymbolCondition, testCase.condition)
		input.Put(SymbolValue, 3)
		_, output, err := Gate(nomagique.Frame{}, input)

		if err != nil {
			t.Fatal(err)
		}

		if got := output.MustGet(SymbolResult); got != testCase.want {
			t.Fatalf("condition=%v result=%v; want %v", testCase.condition, got, testCase.want)
		}
	}
}
