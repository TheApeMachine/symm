package probability_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestDistributionNext(t *testing.T) {
	for _, values := range [][]float64{{1, 1, 1, 1}, {-1000, 1000, 0}, {8}, {0, 5, 1}} {
		members := []core.Primitive{}
		for _, v := range values {
			members = append(members, core.From(v))
		}
		out := tests.Drain(t, probability.NewDistribution(), transport.NewIO(members...))
		if len(out) != 1 {
			t.Fatalf("outputs=%v", out)
		}
		record := out[0].(map[string]core.Primitive)
		p := core.To[[]float64](record["probabilities"])
		sum := 0.0
		for _, v := range p {
			sum += v
		}
		tests.EqualNumber(t, sum, 1)
		max := values[0]
		winner := 0
		total := 0.0
		for i, v := range values {
			if v > max {
				max = v
				winner = i
			}
		}
		for _, v := range values {
			total += math.Exp(v - max)
		}
		tests.EqualNumber(t, core.To[float64](record["winner"]), float64(winner))
		tests.EqualNumber(t, core.To[float64](record["confidence"]), 1/total)
	}
}
func TestCalibratorNext(t *testing.T) {
	calibrator := probability.NewCalibrator(collection.NewTail[float64](transport.NewIO(core.From(3))))
	samples := []float64{4, 2, 3, 1, 5}
	history := []float64{}
	for _, x := range samples {
		out := tests.Drain(t, calibrator, transport.NewIO(core.From(x)))
		record := out[0].(map[string]core.Primitive)
		want := 0.0
		for _, v := range history {
			if v > x {
				want++
			}
		}
		if len(history) > 0 {
			want /= float64(len(history))
		}
		tests.EqualNumber(t, core.To[float64](record["value"]), want)
		tests.EqualNumber(t, core.To[float64](record["prior_count"]), float64(len(history)))
		if core.To[bool](record["ready"]) != (len(history) > 0) {
			t.Fatal("readiness")
		}
		history = append(history, x)
		if len(history) > 3 {
			history = history[1:]
		}
	}
}
func TestEntropyNext(t *testing.T) {
	for _, tc := range []struct {
		x    []float64
		want float64
	}{{[]float64{1}, 0}, {[]float64{.5, .5}, math.Log(2)}, {[]float64{0, 1}, 0}} {
		values := []core.Primitive{}
		for _, v := range tc.x {
			values = append(values, core.From(v))
		}
		out := tests.Drain(t, equation.NewEntropy(), transport.NewIO(values...))
		tests.EqualNumber(t, out[0], tc.want)
	}
}

func TestDistributionUndefinedInput(t *testing.T) {
	for _, members := range [][]core.Primitive{nil, {core.From(math.NaN())}, {core.From(math.Inf(1))}} {
		node := probability.NewDistribution()
		core.Yield(transport.NewIO(core.From(0)), transport.NewApply(node, transport.NewIO(members...)), func(n int, p core.Primitive) int { return n })
		if node.Error() == nil {
			t.Fatal("undefined distribution presented as valid")
		}
	}
}
