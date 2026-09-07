package store_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"reflect"
	"testing"
)

func TestPairComposition(t *testing.T) {
	graph := transport.NewPipe(transport.NewSpread[float64](), transport.NewWindow(2, 1))
	for _, test := range []struct {
		values []float64
		want   [][2]float64
	}{{[]float64{1, 2, 3, 4}, [][2]float64{{1, 2}, {2, 3}, {3, 4}}}, {[]float64{1, 2, 3}, [][2]float64{{1, 2}, {2, 3}}}, {[]float64{1}, nil}, {nil, nil}} {
		input := transport.NewIO(core.From(test.values))
		var actual [][2]float64
		for out := graph.Next(input); out != nil; out = graph.Next(input) {
			pair := core.To[[]core.Primitive](out)
			if len(pair) != 2 {
				t.Fatal("invalid pair")
			}
			actual = append(actual, [2]float64{core.To[float64](pair[0]), core.To[float64](pair[1])})
		}
		if graph.Error() != nil || !reflect.DeepEqual(actual, test.want) {
			t.Fatalf("got %v, %v; want %v", actual, graph.Error(), test.want)
		}
	}
	// First/Second are ordinary indexed projections, with no alternate pair type.
	for index, want := range []float64{3, 7} {
		node := collection.NewAt[core.Primitive](store.NewConstant(core.From(float64(index))))
		value, err := transport.Evaluate[float64](node, core.From([]core.Primitive{core.From(3.0), core.From(7.0)}))
		if err != nil || value != want {
			t.Fatalf("member %v %v", value, err)
		}
	}
}
