package transport_test

import (
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCollectionMovementByComposition(t *testing.T) {
	for _, item := range []struct {
		name string
		node core.Primitive
		want float64
	}{
		{"squares", transport.NewPipe(transport.NewSpread[float64](),
			transport.NewMap(calculus.NewSquare(transport.NewIO(core.From(0.0)))),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))), 210},
		{"count", transport.NewPipe(transport.NewSpread[float64](), equation.NewCount()), 4},
		{"mean", transport.NewPipe(transport.NewSpread[float64](), equation.NewMean()), 7},
	} {
		t.Run(item.name, func(t *testing.T) {
			out := tests.Drain(t, item.node, tests.Values([]float64{4, 7, 9, 8}))
			tests.Sound(t, item.node)
			if len(out) != 1 {
				t.Fatal(out)
			}
			tests.EqualNumber(t, out[0], item.want)
		})
	}
}

func TestPairingAndSelectionByComposition(t *testing.T) {
	// The same generic window serves pairing; no consumer inspects its producer.
	for _, run := range [][]float64{{1, 2, 3, 4}, {1}, {}} {
		window := transport.NewPipe(transport.NewSpread[float64](), transport.NewWindow(2, 1))
		out := tests.Drain(t, window, tests.Values(run))
		tests.Sound(t, window)
		want := max(0, len(run)-1)
		if len(out) != want {
			t.Fatalf("got %d pairs, want %d", len(out), want)
		}
		for i, pair := range out {
			members := pair.([]core.Primitive)
			if len(members) != 2 {
				t.Fatal(members)
			}
			tests.EqualNumber(t, core.To[float64](members[0]), run[i])
			tests.EqualNumber(t, core.To[float64](members[1]), run[i+1])
		}
	}
	for _, position := range []float64{0, 1} {
		selectMember := transport.NewPipe(transport.NewWindow(2, 1),
			transport.NewMap(collection.NewAt[core.Primitive](transport.NewIO(core.From(position)))))
		out := tests.Drain(t, selectMember, tests.Values(3.0, 7.0, 11.0))
		tests.Sound(t, selectMember)
		wanted := []any{3.0, 7.0}
		if position == 1 {
			wanted = []any{7.0, 11.0}
		}
		if !reflect.DeepEqual(out, wanted) {
			t.Fatal(out, wanted)
		}
	}
}

func TestSourceBindingAndExplicitReplay(t *testing.T) {
	// Independent IOs, rather than caller-identity introspection, own replay.
	values := []core.Primitive{core.From([]float64{1, 2, 3})}
	first := transport.NewApply(transport.NewSpread[float64](), transport.NewIO(values...))
	second := transport.NewApply(transport.NewSpread[float64](), transport.NewIO(values...))
	if core.To[float64](first.Next(nil)) != 1 {
		t.Fatal("first source")
	}
	out := tests.Drain(t, second, nil)
	if !reflect.DeepEqual(out, []any{1.0, 2.0, 3.0}) {
		t.Fatal(out)
	}
	out = tests.Drain(t, first, nil)
	if !reflect.DeepEqual(out, []any{2.0, 3.0}) {
		t.Fatal(out)
	}
}

func TestPairProductUsesExistingMultiplication(t *testing.T) {
	node := transport.NewPipe(transport.NewSpread[float64](),
		arithmetic.NewMultiply[float64](store.NewConstant(core.From(1.0))))
	out := tests.Drain(t, node, tests.Values([]float64{3, 7}))
	tests.Sound(t, node)
	tests.EqualNumber(t, out[0], 21)
}
