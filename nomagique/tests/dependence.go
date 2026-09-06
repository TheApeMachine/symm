package tests

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
)

// CheckDependence retains an independent interval-sum oracle at the boundary.
// The extrema are integer timestamps; comparison occurs before conversion.
func CheckDependence(t *testing.T, node core.Primitive) {
	t.Helper()
	cases := []struct {
		lt, rt []int64
		lp, rp []float64
	}{
		{[]int64{0, 2e9}, []int64{0, 1e9, 2e9}, []float64{1, math.E}, []float64{1, math.E, math.Exp(2)}},
		{[]int64{0, 1e9, 2e9}, []int64{0, 1e9, 2e9}, []float64{1, 2, 3}, []float64{1, .5, 1.0 / 3}},
		{[]int64{0, 1e9}, []int64{2e9, 3e9}, []float64{1, 2}, []float64{1, 2}},
		{nil, nil, nil, nil},
		{[]int64{1}, []int64{1}, []float64{1}, []float64{2}},
		{[]int64{1700000000000000000, 1700000000000000007},
			[]int64{1700000000000000003, 1700000000000000010}, []float64{1, 2}, []float64{1, 3}},
	}
	random := rand.New(rand.NewSource(1701))
	for range 25 {
		lt, rt := []int64{0}, []int64{0}
		lp, rp := []float64{1}, []float64{1}
		for range 7 {
			lt = append(lt, lt[len(lt)-1]+int64(random.Intn(4)+1)*1e9)
			rt = append(rt, rt[len(rt)-1]+int64(random.Intn(4)+1)*1e9)
			lp = append(lp, lp[len(lp)-1]*math.Exp(random.NormFloat64()*.1))
			rp = append(rp, rp[len(rp)-1]*math.Exp(random.NormFloat64()*.1))
		}
		cases = append(cases, struct {
			lt, rt []int64
			lp, rp []float64
		}{lt, rt, lp, rp})
	}
	for index, c := range cases {
		covariance, support, le, re := 0.0, 0.0, 0.0, 0.0
		lrates, rrates := []float64{}, []float64{}
		for i := 1; i < len(c.lp); i++ {
			a := math.Log(c.lp[i]) - math.Log(c.lp[i-1])
			le += a * a
			lrates = append(lrates, a*a/(float64(c.lt[i]-c.lt[i-1])*1e-9))
			for j := 1; j < len(c.rp); j++ {
				if c.lt[i-1] < c.rt[j] && c.rt[j-1] < c.lt[i] {
					covariance += a * (math.Log(c.rp[j]) - math.Log(c.rp[j-1]))
					support++
				}
			}
		}
		for j := 1; j < len(c.rp); j++ {
			a := math.Log(c.rp[j]) - math.Log(c.rp[j-1])
			re += a * a
			rrates = append(rrates, a*a/(float64(c.rt[j]-c.rt[j-1])*1e-9))
		}
		shared := 0.0
		if len(c.lt) > 1 && len(c.rt) > 1 {
			shared = float64(max(int64(0), min(c.lt[len(c.lt)-1], c.rt[len(c.rt)-1])-max(c.lt[0], c.rt[0]))) * 1e-9
		}
		density := 0.0
		if shared > 0 {
			density = support / shared
		}
		got := Drain(t, node, Observation(Path(c.lt, c.lp), Path(c.rt, c.rp)))
		Sound(t, node)
		if len(got) != 1 {
			t.Fatalf("case %d yielded %d records", index, len(got))
		}
		fields := Fields(t, got[0])
		for name, want := range map[string]float64{
			"covariance": covariance, "support": support, "left_energy": le, "right_energy": re,
			"correlation": covariance / math.Sqrt(le*re), "shared_time": shared, "overlap_density": density,
			"left_returns": float64(max(0, len(c.lp)-1)), "right_returns": float64(max(0, len(c.rp)-1)),
		} {
			EqualNumber(t, Number(t, fields, name), want)
		}
		for name, rates := range map[string][]float64{"left_energy_rate": lrates, "right_energy_rate": rrates} {
			want := math.NaN()
			sort.Float64s(rates)
			if len(rates) > 0 {
				want = (rates[(len(rates)-1)/2] + rates[len(rates)/2]) / 2
			}
			EqualNumber(t, Number(t, fields, name), want)
		}
		if core.To[bool](fields["defined"]) != (support > 0 && le > 0 && re > 0) {
			t.Fatal("wrong definedness", index)
		}
	}
}
