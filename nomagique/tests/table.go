package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

// Reference folds live once in the harness, independently of production nodes.
var references = map[string]func(float64, float64) float64{
	"add":      func(a, b float64) float64 { return a + b },
	"subtract": func(a, b float64) float64 { return a - b },
	"multiply": func(a, b float64) float64 { return a * b },
	"divide":   func(a, b float64) float64 { return a / b },
	"absolute": func(_, x float64) float64 { return math.Abs(x) },
	"atanh":    func(_, x float64) float64 { return math.Atanh(x) },
	"erfc":     func(_, x float64) float64 { return math.Erfc(x) },
	"exp":      func(_, x float64) float64 { return math.Exp(x) },
	"log":      func(_, x float64) float64 { return math.Log(x) },
	"maximum":  math.Max, "minimum": math.Min,
	"negate":     func(_, x float64) float64 { return -x },
	"reciprocal": func(_, x float64) float64 { return 1 / x },
	"floor":      func(_, x float64) float64 { return math.Floor(x) },
	"sqrt":       func(_, x float64) float64 { return math.Sqrt(x) },
	"square":     func(_, x float64) float64 { return x * x },
	"tanh":       func(_, x float64) float64 { return math.Tanh(x) },
	"sign": func(_, x float64) float64 {
		if math.IsNaN(x) || x == 0 {
			return x
		}
		return math.Copysign(1, x)
	},
}

type Case struct {
	Name      string
	Operation core.Primitive
	Seed      float64
}

// Check exercises independent delivery runs, multi-yield input, empty input,
// nil input and every IEEE exceptional value. Defined saturation is compared to
// the reference operation, never exempted from testing.
func Check(t *testing.T, example Case) {
	t.Helper()
	fold, found := references[example.Name]
	if !found {
		t.Fatalf("unknown reference %q", example.Name)
	}
	runs := [][]float64{{2}, {3}, {1, 2, 3}, {}, {0}, {-1}, {math.NaN()}, {math.Inf(1)}, {math.Inf(-1)}}
	for _, run := range runs {
		wanted := example.Seed
		values := []core.Primitive{}
		for _, value := range run {
			values = append(values, core.From(value))
			wanted = fold(wanted, value)
		}
		got := Drain(t, example.Operation, transport.NewIO(values...))
		if len(got) != 1 {
			t.Fatalf("%s yielded %d values", example.Name, len(got))
		}
		EqualNumber(t, got[0], wanted)
		EqualNumber(t, example.Operation.Read(), wanted)
		if err := example.Operation.Error(); err != nil {
			t.Fatal(err)
		}
	}
	nilRun := Drain(t, example.Operation, nil)
	if len(nilRun) != 1 {
		t.Fatalf("nil input: %d results", len(nilRun))
	}
	EqualNumber(t, nilRun[0], example.Seed)
}

// EqualNumber is the single numerical assertion boundary.
func EqualNumber(t *testing.T, actual any, expected float64) {
	t.Helper()
	value, ok := actual.(float64)
	if !ok {
		t.Fatalf("got %T, wanted float64", actual)
	}
	if math.IsNaN(expected) {
		if !math.IsNaN(value) {
			t.Fatalf("got %v, wanted NaN", value)
		}
		return
	}
	if value == expected {
		return
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || math.IsInf(expected, 0) || math.Abs(value-expected) > 1e-12*math.Max(1, math.Abs(expected)) {
		t.Fatalf("got %.17g, wanted %.17g", value, expected)
	}
}
