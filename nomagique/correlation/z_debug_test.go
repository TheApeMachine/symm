package correlation_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDebugDependence(t *testing.T) {
	cases := []struct {
		name   string
		lt, rt []int64
		lp, rp []float64
	}{
		{"a", []int64{0, 2e9}, []int64{0, 1e9, 2e9}, []float64{1, math.E}, []float64{1, math.E, math.Exp(2)}},
		{"b", []int64{0, 1e9, 2e9}, []int64{0, 1e9, 2e9}, []float64{1, 2, 3}, []float64{1, .5, 1.0 / 3}},
		{"c", []int64{0, 1e9}, []int64{2e9, 3e9}, []float64{1, 2}, []float64{1, 2}},
		{"nil", nil, nil, nil, nil},
		{"single", []int64{1}, []int64{1}, []float64{1}, []float64{2}},
		{"large", []int64{1700000000000000000, 1700000000000000007},
			[]int64{1700000000000000003, 1700000000000000010}, []float64{1, 2}, []float64{1, 3}},
	}
	node := correlation.NewDependence(algo.NewHayashiYoshida())
	for _, c := range cases {
		pair := [2][][2]float64{prices(c.lt, c.lp), prices(c.rt, c.rp)}
		out := tests.CollectSeq[[13]float64](node.Next(tests.SliceToSeq([][2][][2]float64{pair})))
		fmt.Printf("%s err=%v n=%d\n", c.name, node.Error(), len(out))
	}
}
