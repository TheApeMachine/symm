package algo_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"math/rand"
	"testing"
)

func TestHayashiYoshida(t *testing.T) {
	// Deliberately asynchronous: one left increment overlaps two right increments.
	left := tests.Path([]int64{0, 2}, []float64{1, math.Exp(1)})
	right := tests.Path([]int64{0, 1, 2}, []float64{1, math.Exp(1), math.Exp(2)})
	node := algo.NewHayashiYoshida()
	for range 3 {
		out := tests.Drain(t, node, tests.Observation(left, right))
		if node.Error() != nil {
			t.Fatal(node.Error())
		}
		if len(out) != 1 {
			t.Fatal(out)
		}
		fields := out[0].(map[string]core.Primitive)
		tests.EqualNumber(t, core.To[float64](fields["covariance"]), 2)
		tests.EqualNumber(t, core.To[float64](fields["left_energy"]), 1)
		tests.EqualNumber(t, core.To[float64](fields["right_energy"]), 2)
		tests.EqualNumber(t, core.To[float64](fields["support"]), 2)
		tests.EqualNumber(t, core.To[float64](fields["correlation"]), math.Sqrt2)
	}
}
func TestHayashiEmptyAndTouch(t *testing.T) {
	left := tests.Path([]int64{0, 1}, []float64{1, 2})
	right := tests.Path([]int64{1, 2}, []float64{1, 2})
	node := algo.NewHayashiYoshida()
	fields := tests.Drain(t, node, tests.Observation(left, right))[0].(map[string]core.Primitive)
	tests.EqualNumber(t, core.To[float64](fields["support"]), 0)
	tests.EqualNumber(t, core.To[float64](fields["correlation"]), 0)
	node = algo.NewHayashiYoshida()
	fields = tests.Drain(t, node, tests.Observation(nil, nil))[0].(map[string]core.Primitive)
	tests.EqualNumber(t, core.To[float64](fields["correlation"]), math.NaN())
}
func TestHayashiReference(t *testing.T) {
	random := rand.New(rand.NewSource(71))
	for range 30 {
		lt, rt := []int64{0}, []int64{0}
		lp, rp := []float64{1}, []float64{1}
		for range 7 {
			lt = append(lt, lt[len(lt)-1]+int64(random.Intn(4)+1))
			rt = append(rt, rt[len(rt)-1]+int64(random.Intn(4)+1))
			lp = append(lp, lp[len(lp)-1]*math.Exp(random.NormFloat64()*0.1))
			rp = append(rp, rp[len(rp)-1]*math.Exp(random.NormFloat64()*0.1))
		}
		covariance, support, lv, rv := 0.0, 0.0, 0.0, 0.0
		for i := 1; i < len(lp); i++ {
			a := math.Log(lp[i]) - math.Log(lp[i-1])
			lv += a * a
			for j := 1; j < len(rp); j++ {
				if lt[i-1] < rt[j] && rt[j-1] < lt[i] {
					covariance += a * (math.Log(rp[j]) - math.Log(rp[j-1]))
					support++
				}
			}
		}
		for j := 1; j < len(rp); j++ {
			b := math.Log(rp[j]) - math.Log(rp[j-1])
			rv += b * b
		}
		node := algo.NewHayashiYoshida()
		out := tests.Drain(t, node, tests.Observation(tests.Path(lt, lp), tests.Path(rt, rp)))[0].(map[string]core.Primitive)
		if node.Error() != nil {
			t.Fatal(node.Error())
		}
		tests.EqualNumber(t, core.To[float64](out["covariance"]), covariance)
		tests.EqualNumber(t, core.To[float64](out["support"]), support)
		tests.EqualNumber(t, core.To[float64](out["correlation"]), covariance/math.Sqrt(lv*rv))
	}
}
