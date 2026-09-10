package correlation_test

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

func prices(at []int64, values []float64) []equation.Price {
	out := make([]equation.Price, len(values))

	for index, value := range values {
		out[index] = equation.Price{At: at[index], Value: value}
	}

	return out
}

func TestDependenceNext(t *testing.T) {
	Convey("Path diagnostics match the independent interval-sum oracle", t, func() {
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

		node := correlation.NewDependence(algo.NewHayashiYoshida())

		for _, test := range cases {
			covariance, support, leftEnergy, rightEnergy := 0.0, 0.0, 0.0, 0.0
			leftRates, rightRates := []float64{}, []float64{}

			for index := 1; index < len(test.lp); index++ {
				increment := math.Log(test.lp[index]) - math.Log(test.lp[index-1])
				leftEnergy += increment * increment
				leftRates = append(leftRates, increment*increment/(float64(test.lt[index]-test.lt[index-1])*1e-9))

				for other := 1; other < len(test.rp); other++ {
					if test.lt[index-1] < test.rt[other] && test.rt[other-1] < test.lt[index] {
						covariance += increment * (math.Log(test.rp[other]) - math.Log(test.rp[other-1]))
						support++
					}
				}
			}

			for other := 1; other < len(test.rp); other++ {
				increment := math.Log(test.rp[other]) - math.Log(test.rp[other-1])
				rightEnergy += increment * increment
				rightRates = append(rightRates, increment*increment/(float64(test.rt[other]-test.rt[other-1])*1e-9))
			}

			shared := 0.0

			if len(test.lt) > 1 && len(test.rt) > 1 {
				shared = float64(max(int64(0), min(test.lt[len(test.lt)-1], test.rt[len(test.rt)-1])-max(test.lt[0], test.rt[0]))) * 1e-9
			}

			density := 0.0

			if shared > 0 {
				density = support / shared
			}

			got, err := transport.Evaluate(node, transport.Values(equation.LagProfileInput{
				Left:  prices(test.lt, test.lp),
				Right: prices(test.rt, test.rp),
			}))
			So(err, ShouldBeNil)
			So(got.Covariance, ShouldAlmostEqual, covariance)
			So(got.Support, ShouldEqual, support)
			So(got.LeftEnergy, ShouldAlmostEqual, leftEnergy)
			So(got.RightEnergy, ShouldAlmostEqual, rightEnergy)
			So(got.SharedTime, ShouldAlmostEqual, shared)
			So(got.OverlapDensity, ShouldAlmostEqual, density)
			So(got.LeftReturns, ShouldEqual, float64(max(0, len(test.lp)-1)))
			So(got.RightReturns, ShouldEqual, float64(max(0, len(test.rp)-1)))
			So(got.Defined, ShouldEqual, support > 0 && leftEnergy > 0 && rightEnergy > 0)
			sameFloat(got.Correlation, covariance/math.Sqrt(leftEnergy*rightEnergy))
			sameFloat(got.LeftEnergyRate, medianRate(leftRates))
			sameFloat(got.RightEnergyRate, medianRate(rightRates))
		}
	})
}

func sameFloat(actual, expected float64) {
	if math.IsNaN(expected) {
		So(math.IsNaN(actual), ShouldBeTrue)
		return
	}

	So(actual, ShouldAlmostEqual, expected, 1e-12*math.Abs(expected)+1e-18)
}

func medianRate(rates []float64) float64 {
	if len(rates) == 0 {
		return math.NaN()
	}

	sort.Float64s(rates)
	count := len(rates)
	return (rates[(count-1)/2] + rates[count/2]) / 2
}
