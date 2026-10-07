package algo_test

import (
	"math"
	"math/rand"
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func prices(at []int64, values []float64) [][2]float64 {
	out := make([][2]float64, len(values))

	for index, value := range values {
		out[index] = [2]float64{float64(at[index]), value}
	}

	return out
}

/*
returns decodes a [][2]float64{at, price} path into flat log returns
{value, from, to, ...} and their energy.
*/
func returns(path [][2]float64) ([]float64, float64) {
	flat := make([]float64, 0, max(0, len(path)-1)*3)
	energy := 0.0

	for index := 1; index < len(path); index++ {
		value := math.Log(path[index][1]) - math.Log(path[index-1][1])
		flat = append(flat, value, path[index-1][0], path[index][0])
		energy += value * value
	}

	return flat, energy
}

func pathQuery(left, right [][2]float64, lag float64) [3][]float64 {
	leftReturns, leftEnergy := returns(left)
	rightReturns, rightEnergy := returns(right)

	return [3][]float64{leftReturns, rightReturns, {leftEnergy, rightEnergy, lag}}
}

func TestHayashiYoshidaNext(t *testing.T) {
	Convey("One left increment overlapping two right increments is counted twice", t, func() {
		query := pathQuery(
			prices([]int64{0, 2}, []float64{1, math.Exp(1)}),
			prices([]int64{0, 1, 2}, []float64{1, math.Exp(1), math.Exp(2)}),
			0,
		)
		node := algo.NewHayashiYoshida()

		for range 3 {
			out := tests.CollectSeq[[6]float64](node.Next(data.NewValue(query)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][1], ShouldEqual, 2)
			So(out[0][3], ShouldEqual, 1)
			So(out[0][4], ShouldEqual, 2)
			So(out[0][2], ShouldEqual, 2)
			So(out[0][0], ShouldAlmostEqual, 1.0)
		}
	})
}

func TestHayashiEmptyAndTouch(t *testing.T) {
	Convey("Touching intervals contribute no overlap, and empty paths stay undefined", t, func() {
		node := algo.NewHayashiYoshida()
		fields := tests.CollectSeq[[6]float64](node.Next(data.NewValue(pathQuery(
			prices([]int64{0, 1}, []float64{1, 2}),
			prices([]int64{1, 2}, []float64{1, 2}),
			0,
		))))
		So(fields[0][2], ShouldEqual, 0)
		So(fields[0][0], ShouldEqual, 0)
		So(fields[0][5], ShouldEqual, 0)

		node = algo.NewHayashiYoshida()
		empty := tests.CollectSeq[[6]float64](node.Next(data.NewValue(pathQuery(nil, nil, 0))))
		So(math.IsNaN(empty[0][0]), ShouldBeTrue)
		So(empty[0][5], ShouldEqual, 0)
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
			overlapLeftEnergy, overlapRightEnergy := 0.0, 0.0

			for index := 1; index < len(lp); index++ {
				increment := math.Log(lp[index]) - math.Log(lp[index-1])
				leftEnergy += increment * increment

				for other := 1; other < len(rp); other++ {
					if lt[index-1] < rt[other] && rt[other-1] < lt[index] {
						rightIncrement := math.Log(rp[other]) - math.Log(rp[other-1])
						covariance += increment * rightIncrement
						overlapLeftEnergy += increment * increment
						overlapRightEnergy += rightIncrement * rightIncrement
						support++
					}
				}
			}

			for other := 1; other < len(rp); other++ {
				increment := math.Log(rp[other]) - math.Log(rp[other-1])
				rightEnergy += increment * increment
			}

			node := algo.NewHayashiYoshida()
			out := tests.CollectSeq[[6]float64](node.Next(data.NewValue(pathQuery(prices(lt, lp), prices(rt, rp), 0))))
			So(node.Error(), ShouldBeNil)
			So(out[0][1], ShouldEqual, covariance)
			So(out[0][2], ShouldEqual, support)
			So(out[0][0], ShouldAlmostEqual, covariance/math.Sqrt(overlapLeftEnergy*overlapRightEnergy))
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

	input := pathQuery(prices(times, values), prices(shifted, values), 0)
	graph := algo.NewHayashiYoshida()
	b.ReportAllocs()

	for b.Loop() {
		count := 0

		for range graph.Next(data.NewValue(input)) {
			count++
		}

		if count != 1 || graph.Error() != nil {
			b.Fatal("expected one covariance record", graph.Error())
		}
	}
}

func TestHayashiYoshidaEstimate(t *testing.T) {
	Convey("Prepared multi-leg paths preserve exact lagged overlap economics", t, func() {
		// Alternating three-observation legs at epoch-scale nanoseconds with
		// irregular event gaps, the shape of the market opportunity tape.
		gaps := []time.Duration{170 * time.Millisecond, 230 * time.Millisecond, 310 * time.Millisecond}
		start := time.Unix(1700000000, 0)
		count := 8*3 + 3
		times, shifted, values := make([]int64, count), make([]int64, count), make([]float64, count)
		eventTime := start

		for index := range count {
			eventTime = eventTime.Add(gaps[index%len(gaps)])
			times[index] = eventTime.UnixNano()
			shifted[index] = times[index] + int64(43*time.Millisecond)
			values[index] = 100

			if index >= 3 {
				sign := 1.0

				if (index-3)/3%2 != 0 {
					sign = -1
				}

				values[index] = values[index-3] * math.Exp(0.02*sign)
			}
		}

		left, right := prices(times, values), prices(shifted, values)
		leftReturns, leftEnergy := returns(left)
		rightReturns, rightEnergy := returns(right)
		original := slices.Clone(leftReturns)
		estimator := algo.NewHayashiYoshida()

		estimate := func(lag float64) ([6]float64, error) {
			out := tests.CollectSeq[[6]float64](estimator.Next(data.NewValue(
				[3][]float64{leftReturns, rightReturns, {leftEnergy, rightEnergy, lag}},
			)))

			if len(out) == 0 {
				return [6]float64{}, estimator.Error()
			}

			return out[0], estimator.Error()
		}

		for _, lag := range []float64{-float64(time.Second), 0, float64(43 * time.Millisecond), float64(time.Second)} {
			covariance, support := 0.0, 0.0
			overlapLeftEnergy, overlapRightEnergy := 0.0, 0.0

			for leftIndex := 0; leftIndex < len(leftReturns); leftIndex += 3 {
				for rightIndex := 0; rightIndex < len(rightReturns); rightIndex += 3 {
					if leftReturns[leftIndex+1]+lag < rightReturns[rightIndex+2] &&
						rightReturns[rightIndex+1] < leftReturns[leftIndex+2]+lag {
						covariance += leftReturns[leftIndex] * rightReturns[rightIndex]
						overlapLeftEnergy += leftReturns[leftIndex] * leftReturns[leftIndex]
						overlapRightEnergy += rightReturns[rightIndex] * rightReturns[rightIndex]
						support++
					}
				}
			}

			fields, err := estimate(lag)
			So(err, ShouldBeNil)
			So(fields[2], ShouldEqual, support)
			So(fields[1], ShouldAlmostEqual, covariance)
			So(fields[0], ShouldAlmostEqual, covariance/math.Sqrt(leftEnergy*rightEnergy))
			So(leftReturns, ShouldResemble, original)
		}

		Convey("Later evaluations do not overwrite an earlier result", func() {
			first, err := estimate(0)
			So(err, ShouldBeNil)
			covariance := first[1]
			_, err = estimate(float64(time.Hour))
			So(err, ShouldBeNil)
			So(first[1], ShouldEqual, covariance)
		})

		Convey("Unrepresentable timestamp offsets fail explicitly", func() {
			_, err := estimate(math.MaxInt64)
			So(err, ShouldNotBeNil)
		})
	})
}
