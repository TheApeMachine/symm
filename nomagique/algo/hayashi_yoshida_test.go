package algo

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/tests/market"
	"math"
	"math/rand"
	"slices"
	"testing"
	"time"
)

func path(at []int64, prices []float64) []core.Primitive {
	out := []core.Primitive{}
	for i, x := range prices {
		out = append(out, core.From(map[string]core.Primitive{"at": core.From(at[i]), "value": core.From(x)}))
	}
	return out
}
func observation(left, right []core.Primitive) core.Primitive {
	return transport.NewIO(core.From(map[string]core.Primitive{"left": core.From(left), "right": core.From(right)}))
}
func TestHayashiYoshida(t *testing.T) {
	// Deliberately asynchronous: one left increment overlaps two right increments.
	left := path([]int64{0, 2}, []float64{1, math.Exp(1)})
	right := path([]int64{0, 1, 2}, []float64{1, math.Exp(1), math.Exp(2)})
	node := NewHayashiYoshida()
	for range 3 {
		out := tests.Drain(t, node, observation(left, right))
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
	left := path([]int64{0, 1}, []float64{1, 2})
	right := path([]int64{1, 2}, []float64{1, 2})
	node := NewHayashiYoshida()
	fields := tests.Drain(t, node, observation(left, right))[0].(map[string]core.Primitive)
	tests.EqualNumber(t, core.To[float64](fields["support"]), 0)
	tests.EqualNumber(t, core.To[float64](fields["correlation"]), 0)
	node = NewHayashiYoshida()
	fields = tests.Drain(t, node, observation(nil, nil))[0].(map[string]core.Primitive)
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
		node := NewHayashiYoshida()
		out := tests.Drain(t, node, observation(path(lt, lp), path(rt, rp)))[0].(map[string]core.Primitive)
		if node.Error() != nil {
			t.Fatal(node.Error())
		}
		tests.EqualNumber(t, core.To[float64](out["covariance"]), covariance)
		tests.EqualNumber(t, core.To[float64](out["support"]), support)
		tests.EqualNumber(t, core.To[float64](out["correlation"]), covariance/math.Sqrt(lv*rv))
	}
}

func BenchmarkNewHayashiYoshida(b *testing.B) {
	// Two 128-observation asynchronous price paths with alternating returns.
	times, shifted := make([]int64, 128), make([]int64, 128)
	prices := make([]float64, 128)
	for index := range times {
		times[index] = int64(index * 2)
		shifted[index] = times[index] + 1
		prices[index] = 100 * math.Exp(0.01*math.Sin(float64(index)))
	}
	input := observation(path(times, prices), path(shifted, prices))
	graph := NewHayashiYoshida()
	b.ReportAllocs()
	for b.Loop() {
		if graph.Next(input) == nil || graph.Next(input) != nil {
			b.Fatal("expected one covariance record")
		}
		if err := graph.Error(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestHayashiYoshidaEstimate(t *testing.T) {
	Convey("Prepared multi-leg paths preserve exact lagged overlap economics", t, func() {
		tape := market.NewOpportunityTape("BTC/USD", time.Unix(1700000000, 0), 8)
		times, shifted, prices := make([]int64, len(tape.Steps)), make([]int64, len(tape.Steps)), make([]float64, len(tape.Steps))
		for index, step := range tape.Steps {
			times[index], prices[index] = step.EventTime.UnixNano(), step.ExecutableBid
			shifted[index] = times[index] + int64(43*time.Millisecond)
		}
		var left, right equation.LogReturns
		So(left.Load(path(times, prices)), ShouldBeNil)
		So(right.Load(path(shifted, prices)), ShouldBeNil)
		original := slices.Clone(left.Intervals)
		estimator := NewHayashiYoshida()
		for _, lag := range []int64{-int64(time.Second), 0, int64(43 * time.Millisecond), int64(time.Second)} {
			covariance, support := 0.0, 0.0
			for _, leftReturn := range left.Intervals {
				for _, rightReturn := range right.Intervals {
					if leftReturn.From+lag < rightReturn.To && rightReturn.From < leftReturn.To+lag {
						covariance += leftReturn.Value * rightReturn.Value
						support++
					}
				}
			}
			fields, err := estimator.Estimate(&left, &right, lag)
			So(err, ShouldBeNil)
			So(core.To[float64](fields["support"]), ShouldEqual, support)
			So(core.To[float64](fields["covariance"]), ShouldAlmostEqual, covariance)
			So(core.To[float64](fields["correlation"]), ShouldAlmostEqual, covariance/math.Sqrt(left.Energy*right.Energy))
			So(left.Intervals, ShouldResemble, original)
		}

		Convey("Later evaluations do not overwrite an earlier result", func() {
			first, err := estimator.Estimate(&left, &right, 0)
			So(err, ShouldBeNil)
			covariance := core.To[float64](first["covariance"])
			_, err = estimator.Estimate(&left, &right, int64(time.Hour))
			So(err, ShouldBeNil)
			So(core.To[float64](first["covariance"]), ShouldEqual, covariance)
		})

		Convey("Unrepresentable timestamp offsets fail explicitly", func() {
			_, err := estimator.Estimate(&left, &right, math.MaxInt64)
			So(err, ShouldNotBeNil)
		})
	})
}
