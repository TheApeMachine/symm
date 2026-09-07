package algo

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestLagProfileSupportAndUnits(t *testing.T) {
	times := []int64{0, 1e9, 2e9, 3e9, 4e9, 5e9}
	left := path(times, []float64{1, 2, 1.5, 3, 2.2, 4})
	right := path([]int64{1e9, 2e9, 3e9, 4e9, 5e9, 6e9}, []float64{1, 2, 1.5, 3, 2.2, 4})
	node := correlation.NewLagProfile(NewHayashiYoshida(), transport.NewIO(core.From(1e9)), transport.NewIO(core.From(2.0)))
	profile := tests.Drain(t, node, observation(left, right))
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
func TestProfileCurvatureSeconds(t *testing.T) {
	points := []core.Primitive{}
	for _, p := range [][2]float64{{-1, 0.1}, {0, 0.9}, {1, 0.3}} {
		points = append(points, core.From(map[string]core.Primitive{"x": core.From(p[0]), "y": core.From(p[1])}))
	}
	curvature := equation.NewCurvature()
	out := tests.Drain(t, curvature, transport.NewIO(points...))
	if curvature.Error() != nil {
		t.Fatal(curvature.Error())
	}
	tests.EqualNumber(t, out[0], 1.4)
	prominence := equation.NewProminence()
	out = tests.Drain(t, prominence, transport.NewIO(points...))
	tests.EqualNumber(t, out[0], .7)
	_ = math.Pi
}
