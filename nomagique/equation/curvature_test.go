package equation_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNewCurvature(t *testing.T) {
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
}
