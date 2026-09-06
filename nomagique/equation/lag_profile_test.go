package equation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNewLagProfile(t *testing.T) {
	times := []int64{0, 1e9, 2e9, 3e9, 4e9, 5e9}
	left := tests.Path(times, []float64{1, 2, 1.5, 3, 2.2, 4})
	right := tests.Path([]int64{1e9, 2e9, 3e9, 4e9, 5e9, 6e9}, []float64{1, 2, 1.5, 3, 2.2, 4})
	node := equation.NewLagProfile(algo.NewHayashiYoshida(), transport.NewIO(core.From(1e9)), transport.NewIO(core.From(2.0)))
	profile := tests.Drain(t, node, tests.Observation(left, right))
	if node.Error() != nil {
		t.Fatal(node.Error())
	}
	if len(profile) != 5 {
		t.Fatal(len(profile))
	}
	points := []core.Primitive{}
	for _, v := range profile {
		m := v.(map[string]core.Primitive)
		if _, ok := m["support"]; !ok {
			t.Fatal("candidate support discarded")
		}
		points = append(points, core.From(m))
	}
	peak := tests.Drain(t, equation.NewPeak(), transport.NewIO(points...))[0].(map[string]core.Primitive)
	point := core.To[map[string]core.Primitive](peak["point"])
	tests.EqualNumber(t, core.To[float64](point["x"]), 1)
	tests.EqualNumber(t, core.To[float64](point["y"]), 1)
}
