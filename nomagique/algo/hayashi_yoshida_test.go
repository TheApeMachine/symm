package algo_test

import (
	"math"
	"math/rand"
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/tests/market"
)

func prices(at []int64, values []float64) []equation.Price {
	out := make([]equation.Price, len(values))

	for index, value := range values {
		out[index] = equation.Price{At: at[index], Value: value}
	}

	return out
}

func pair(left, right []equation.Price) equation.LagProfileInput {
	return equation.LagProfileInput{Left: left, Right: right}
}

func TestHayashiYoshidaNext(t *testing.T) {
	Convey("One left increment overlapping two right increments is counted twice", t, func() {
		left := prices([]int64{0, 2}, []float64{1, math.Exp(1)})
		right := prices([]int64{0, 1, 2}, []float64{1, math.Exp(1), math.Exp(2)})
		node := algo.NewHayashiYoshida()

		for range 3 {
			out := tests.CollectSeq(node.Next(transport.Values(pair(left, right))))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0].Covariance, ShouldEqual, 2)
			So(out[0].LeftEnergy, ShouldEqual, 1)
			So(out[0].RightEnergy, ShouldEqual, 2)
			So(out[0].Support, ShouldEqual, 2)
			So(out[0].Correlation, ShouldAlmostEqual, math.Sqrt2)
		}
	})
}

func TestHayashiEmptyAndTouch(t *testing.T) {
	Convey("Touching intervals contribute no overlap, and empty paths stay undefined", t, func() {
		node := algo.NewHayashiYoshida()
		fields := tests.CollectSeq(node.Next(transport.Values(pair(
			prices([]int64{0, 1}, []float64{1, 2}),
			prices([]int64{1, 2}, []float64{1, 2}),
		))))
		So(fields[0].Support, ShouldEqual, 0)
		So(fields[0].Correlation, ShouldEqual, 0)

		node = algo.NewHayashiYoshida()
		empty := tests.CollectSeq(node.Next(transport.Values(pair(nil, nil))))
		So(math.IsNaN(empty[0].Correlation), ShouldBeTrue)
	})
}

func TestHayashiReference(t *testing.T) {
	Convey("Random asynchronous paths match the interval-sum oracle", t, func() {
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

			covariance, support, leftEnergy, rightEnergy := 0.0, 0.0, 0.0, 0.0

			for index := 1; index < len(lp); index++ {
				increment := math.Log(lp[index]) - math.Log(lp[index-1])
				leftEnergy += increment * increment

				for other := 1; other < len(rp); other++ {
					if lt[index-1] < rt[other] && rt[other-1] < lt[index] {
						covariance += increment * (math.Log(rp[other]) - math.Log(rp[other-1]))
						support++
					}
				}
			}

			for other := 1; other < len(rp); other++ {
				increment := math.Log(rp[other]) - math.Log(rp[other-1])
				rightEnergy += increment * increment
			}

			node := algo.NewHayashiYoshida()
			out := tests.CollectSeq(node.Next(transport.Values(pair(prices(lt, lp), prices(rt, rp)))))
			So(node.Error(), ShouldBeNil)
			So(out[0].Covariance, ShouldEqual, covariance)
			So(out[0].Support, ShouldEqual, support)
			So(out[0].Correlation, ShouldAlmostEqual, covariance/math.Sqrt(leftEnergy*rightEnergy))
		}
	})
}

func BenchmarkNewHayashiYoshida(b *testing.B) {
	times, shifted := make([]int64, 128), make([]int64, 128)
	values := make([]float64, 128)

	for index := range times {
		times[index] = int64(index * 2)
		shifted[index] = times[index] + 1
		values[index] = 100 * math.Exp(0.01*math.Sin(float64(index)))
	}

	input := pair(prices(times, values), prices(shifted, values))
	graph := algo.NewHayashiYoshida()
	b.ReportAllocs()

	for b.Loop() {
		count := 0

		for range graph.Next(transport.Values(input)) {
			count++
		}

		if count != 1 || graph.Error() != nil {
			b.Fatal("expected one covariance record", graph.Error())
		}
	}
}

func TestHayashiYoshidaEstimate(t *testing.T) {
	Convey("Prepared multi-leg paths preserve exact lagged overlap economics", t, func() {
		tape := market.NewOpportunityTape("BTC/USD", time.Unix(1700000000, 0), 8)
		times, shifted, values := make([]int64, len(tape.Steps)), make([]int64, len(tape.Steps)), make([]float64, len(tape.Steps))

		for index, step := range tape.Steps {
			times[index], values[index] = step.EventTime.UnixNano(), step.ExecutableBid
			shifted[index] = times[index] + int64(43*time.Millisecond)
		}

		var left, right equation.LogReturns
		So(left.Load(prices(times, values)), ShouldBeNil)
		So(right.Load(prices(shifted, values)), ShouldBeNil)
		original := slices.Clone(left.Intervals)
		estimator := algo.NewHayashiYoshida()

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
			So(fields.Support, ShouldEqual, support)
			So(fields.Covariance, ShouldAlmostEqual, covariance)
			So(fields.Correlation, ShouldAlmostEqual, covariance/math.Sqrt(left.Energy*right.Energy))
			So(left.Intervals, ShouldResemble, original)
		}

		Convey("Later evaluations do not overwrite an earlier result", func() {
			first, err := estimator.Estimate(&left, &right, 0)
			So(err, ShouldBeNil)
			covariance := first.Covariance
			_, err = estimator.Estimate(&left, &right, int64(time.Hour))
			So(err, ShouldBeNil)
			So(first.Covariance, ShouldEqual, covariance)
		})

		Convey("Unrepresentable timestamp offsets fail explicitly", func() {
			_, err := estimator.Estimate(&left, &right, math.MaxInt64)
			So(err, ShouldNotBeNil)
		})
	})
}
